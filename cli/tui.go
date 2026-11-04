package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"deepseek-coder/internal/clipboard"
	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/jobs"
	"deepseek-coder/internal/markdown"
	"deepseek-coder/internal/shell"
)

// Styles.
var (
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	styleUser   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleAI     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleSystem = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleInfo   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
)

const tuiMaxSuggestions = 8

type tuiMode int

const (
	modeChat tuiMode = iota
	modeConfirmTool
	modeQuestion
	modeModelSelect
	modeSecret
)

type chatEntry struct {
	role    string // user, assistant, system, error, info
	content string
}

// renderedEntry caches the rendered form of one chat entry. Without it every
// streamed chunk would re-render the whole history, which gets expensive in a
// long conversation once Markdown rendering is involved.
type renderedEntry struct {
	src string
	out string
}

// Messages exchanged with the tea program.
type streamChunkMsg struct{ text string }
type streamDoneMsg struct {
	content string
	err     error
}

// questionRequestMsg asks the UI to present the model's question.
type questionRequestMsg struct {
	question string
	options  []string
	multi    bool
	ch       chan []string
}

type toolRequestMsg struct {
	tc deepseek.ToolCall
	ch chan bool
}
type infoMsg struct{ text string }
type modelListMsg struct{ models []deepseek.Model }
type setModelMsg struct {
	id     string
	warn   bool
	window int // context window of the new model, 0 when unknown
}
type reasoningMsg struct{ text string }

// copiedMsg reports the outcome of a clipboard copy.
type copiedMsg struct {
	chars int
	err   error
}

// savedMsg reports the outcome of saving the transcript to a file.
type savedMsg struct {
	path string
	err  error
}

// runDoneMsg carries the result of a /run command.
type runDoneMsg struct {
	command string
	res     shell.Result
	err     error
}

// usageMsg reports the token accounting of one API round.
type usageMsg struct{ usage deepseek.Usage }

// balanceMsg carries the result of a /balance query.
type balanceMsg struct {
	text string
	err  error
}

// jobDoneMsg announces a finished background job.
type jobDoneMsg struct{ result jobs.Result }

// compactMsg carries the result of a /compact request.
type compactMsg struct {
	before, after int
	err           error
}

type tuiModel struct {
	app    *app
	client *deepseek.Client
	model  string

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	help     help.Model

	entries []chatEntry

	width  int
	height int

	mode       tuiMode
	streaming  bool
	pendingMsg string

	permCh chan bool

	// reading is true while a reader command is outstanding on chatCh.
	reading     bool
	pendingTool string
	modelList   []deepseek.Model
	secret      strings.Builder
	suggestions []string

	// Question state, used while mode == modeQuestion.
	questionCh     chan []string
	questionOpts   []string
	questionCursor int
	questionPicked map[int]bool

	chatCh chan tea.Msg // messages from the chat goroutine

	streamStart time.Time
	reasoning   string // live reasoning content of thinking models

	renderCache []renderedEntry // per-entry cache, see renderEntries
	renderWidth int             // width the cache was built for
}

// slowResponseMsg fires when the model has not produced any content for a
// while, so the user knows the request is still alive.
type slowResponseMsg struct{ elapsed time.Duration }

// replTUI runs the interactive chat as a full-screen TUI.
func (a *app) replTUI(client *deepseek.Client, model string) error {
	m := newTUIModel(a, client, model)
	// Deliberately no tea.WithMouseCellMotion(): capturing the mouse would make
	// the terminal hand drags to the app, and the user could no longer select
	// text with the cursor. Scrolling still works because terminals translate
	// the wheel into arrow keys, which the viewport consumes (see viewportMsg).
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newTUIModel(a *app, client *deepseek.Client, model string) tuiModel {
	if a.jobs == nil {
		a.jobs = jobs.New() // tests build an app without one
	}
	ta := textarea.New()
	ta.Placeholder = a.msg.TUIInputPlaceholder
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter is handled by us
	ta.Focus()                                // the textarea must be focused to accept input

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	sp.Spinner = spinner.Dot

	m := tuiModel{
		app:      a,
		client:   client,
		model:    model,
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		help:     help.New(),
	}
	m = m.addEntry("info", a.msg.Welcome)
	m = m.addEntry("info", a.msg.TUIHints)
	m.viewport.SetContent(m.renderEntries())
	return m
}

// waitJob blocks on the job channel and turns a completion into a message.
// It is re-armed after every receipt, so completions are pushed, never polled.
func (m tuiModel) waitJob() tea.Cmd {
	return func() tea.Msg {
		result := <-m.app.jobs.Done()
		return jobDoneMsg{result: result}
	}
}
func (m tuiModel) Init() tea.Cmd {
	// The job listener runs for the whole session: a background job that ends
	// while the user is typing still reaches the conversation.
	return tea.Batch(m.spinner.Tick, textarea.Blink, m.waitJob())
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages from the chat goroutine go through chatMsg, which is where the
	// reader is re-armed. Doing it here instead of in every case is what keeps
	// one forgotten handler from hanging the whole turn.
	switch msg.(type) {
	case chatMsg:
		wrapped := msg.(chatMsg)
		m.reading = false
		next, cmd := m.Update(wrapped.inner)
		return next, cmd
	case chatClosedMsg:
		m.reading = false
		m.chatCh = nil
		return m, nil
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.handleResize()

	case tea.KeyMsg:
		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		cmds = append(cmds, cmd)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case streamChunkMsg:
		m = m.appendAssistant(msg.text)
		m = m.refreshViewport()

	case streamDoneMsg:
		m.streaming = false
		m.pendingMsg = ""
		if msg.err != nil {
			m = m.addEntry("error", msg.err.Error())
		} else if m.app.session != nil {
			// Close the durable turn so a later resume does not report it as
			// interrupted.
			if err := m.app.session.AppendTurnEnd("completed"); err != nil {
				m = m.addEntry("error", m.app.msg.SaveFailed+" "+err.Error())
			}
		}
		m = m.refreshViewport()

	case questionRequestMsg:
		m.questionCh = msg.ch
		m.questionOpts = msg.options
		m.questionCursor = 0
		m.questionPicked = map[int]bool{}
		m.mode = modeQuestion
		m.pendingMsg = msg.question
		m.textarea.Reset()
		m = m.refreshViewport()

	case toolRequestMsg:
		m.permCh = msg.ch
		m.pendingTool = msg.tc.Function.Name
		m.mode = modeConfirmTool
		m.pendingMsg = m.app.msg.ToolRequest(msg.tc.Function.Name, toolDetail(msg.tc))
		if msg.tc.Function.Name == shellToolName {
			m.pendingMsg += " " + m.app.msg.ShellAlwaysWarning
		}
		m.textarea.Reset()
		m.textarea.Placeholder = m.app.msg.ToolPrompt
		m = m.refreshViewport()

	case infoMsg:
		m = m.addEntry("info", msg.text)
		m = m.refreshViewport()

	case reasoningMsg:
		m.reasoning += msg.text
		if len(m.reasoning) > 120 {
			m.reasoning = m.reasoning[:120]
		}

	case modelListMsg:
		m.modelList = msg.models
		m.mode = modeModelSelect
		m.pendingMsg = m.app.msg.ModelSelectHint
		var b strings.Builder
		b.WriteString(m.app.msg.SelectModel + "\n")
		for i, md := range msg.models {
			marker := " "
			if md.ID == m.model {
				marker = "*"
			}
			name := md.Name
			if name == "" {
				name = "-"
			}
			fmt.Fprintf(&b, "  %d) %s %s %s\n", i+1, marker, md.ID, name)
		}
		m = m.addEntry("info", b.String())
		m.textarea.Reset()
		m.textarea.Placeholder = m.app.msg.ModelSelectPlaceholder
		m = m.refreshViewport()

	case setModelMsg:
		if msg.warn {
			m = m.addEntry("info", m.app.msg.ModelNotListed(msg.id))
		}
		if msg.window > 0 {
			m.app.modelWindow = msg.window
		}
		m = m.setModelNow(msg.id)
		m = m.refreshViewport()

	case slowResponseMsg:
		if m.streaming {
			m = m.addEntry("info", m.app.msg.SlowResponse(msg.elapsed))
			m = m.refreshViewport()
		}

	case copiedMsg:
		if msg.err != nil {
			m = m.addEntry("error", m.app.msg.CopyFailed+" "+msg.err.Error())
		} else {
			m = m.addEntry("info", m.app.msg.Copied(msg.chars))
		}
		m = m.refreshViewport()

	case savedMsg:
		if msg.err != nil {
			m = m.addEntry("error", m.app.msg.SaveTranscriptFailed+" "+msg.err.Error())
		} else {
			m = m.addEntry("info", m.app.msg.TranscriptSaved(msg.path))
		}
		m = m.refreshViewportFollow()

	case runDoneMsg:
		m = m.addEntry("info", m.app.renderRunResult(msg.command, msg.res, msg.err))
		m = m.refreshViewportFollow()

	case usageMsg:
		// Accumulated here, on the UI goroutine, so the header counter is not
		// written from the chat goroutine.
		m.app.addUsage(msg.usage)

	case balanceMsg:
		if msg.err != nil {
			m = m.addEntry("error", m.app.msg.BalanceFailed+" "+msg.err.Error())
		} else {
			m = m.addEntry("info", msg.text)
		}
		m = m.refreshViewportFollow()

	case jobDoneMsg:
		note := jobNotification(msg.result)
		if m.app.session != nil {
			_ = m.app.session.AppendUser(note)
		}
		m = m.addEntry("info", note)
		m = m.refreshViewportFollow()
		// The whole point of a job is that work continues: if nothing else is
		// happening, let the model react to the result right away.
		if !m.streaming && strings.TrimSpace(m.textarea.Value()) == "" {
			m.streaming = true
			m.pendingMsg = m.app.msg.TUIThinking
			m.streamStart = time.Now()
			var chatCmd tea.Cmd
			m, chatCmd = m.startChat()
			return m, tea.Batch(chatCmd, m.waitJob())
		}
		return m, m.waitJob()

	case compactMsg:
		if msg.err != nil {
			m = m.addEntry("error", m.app.msg.CompactionFailed+" "+msg.err.Error())
		} else {
			m = m.addEntry("info", m.app.msg.CompactionDone(msg.before, msg.after))
		}
		m = m.refreshViewportFollow()
	}

	// The textarea handles typing (except in secret mode, where we read the
	// key ourselves to keep it hidden).
	if m.mode != modeSecret {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Refresh suggestions after typing.
	if m.mode == modeChat && !m.streaming {
		m.suggestions = m.computeSuggestions()
	}

	var vpCmd tea.Cmd
	if vpMsg, ok := viewportMsg(msg, m.textarea.Value() == ""); ok {
		m.viewport, vpCmd = m.viewport.Update(vpMsg)
		cmds = append(cmds, vpCmd)
	}

	// Safety net: whatever path we came from, if a turn is running there must
	// be exactly one reader on the chat channel. This is what makes a forgotten
	// re-arm impossible instead of merely unlikely.
	if m.chatCh != nil && m.streaming && !m.reading {
		m.reading = true
		cmds = append(cmds, m.readChat())
	}
	return m, tea.Batch(cmds...)
}

// viewportMsg reports whether msg should be delivered to the chat viewport.
//
// Arrow keys are only forwarded while the input is empty: terminals that do
// not capture the mouse translate the wheel into arrow keys, and that is how
// the wheel scrolls the chat. With text in the input the arrows belong to the
// textarea, so typing is never disturbed.
func viewportMsg(msg tea.Msg, inputEmpty bool) (tea.Msg, bool) {
	switch m := msg.(type) {
	case tea.MouseMsg:
		return m, true
	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyPgUp, tea.KeyPgDown:
			return m, true
		case tea.KeyUp, tea.KeyDown:
			return m, inputEmpty
		}
	}
	return nil, false
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyCtrlY:
		// Copy the last reply straight to the clipboard: in a full-screen
		// TUI the terminal has no scrollback, so dragging to select a long
		// answer is painful.
		return m, m.copyLastReply()
	case tea.KeyCtrlJ:
		// Ctrl+J inserts a newline. Alt+Enter used to do this, but Windows
		// Terminal binds Alt+Enter to fullscreen, and Shift+Enter cannot be
		// used: the terminal delivers it as a plain Enter (Bubble Tea maps
		// VK_RETURN to KeyEnter without looking at the Shift state), so it
		// would send the message instead. Ctrl+J arrives as its own key on
		// every platform.
		if m.mode == modeChat {
			m.textarea.InsertString("\n")
		}
		return m, nil
	case tea.KeyEnter:
		if msg.Alt {
			// Kept as a secondary binding for terminals that do deliver it.
			if m.mode == modeChat {
				m.textarea.InsertString("\n")
			}
			return m, nil
		}
		switch m.mode {
		case modeChat:
			return m.sendMessage()
		case modeConfirmTool:
			return m.answerTool()
		case modeModelSelect:
			return m.pickModelFromInput()
		case modeSecret:
			return m.finishLogin()
		}
	case tea.KeyTab:
		if m.mode == modeChat {
			m = m.complete()
		}
	case tea.KeyRunes:
		if m.mode == modeSecret {
			m.secret.WriteString(string(msg.Runes))
		}
	case tea.KeyBackspace:
		if m.mode == modeSecret {
			s := m.secret.String()
			if len(s) > 0 {
				m.secret.Reset()
				m.secret.WriteString(s[:len(s)-1])
			}
		}
	}
	return m, nil
}

// sendMessage sends the current textarea content as a user message.
func (m tuiModel) sendMessage() (tuiModel, tea.Cmd) {
	msg := strings.TrimSpace(m.textarea.Value())
	if msg == "" || m.streaming {
		return m, nil
	}
	m.textarea.Reset()
	m.suggestions = nil

	if strings.HasPrefix(msg, "/") {
		return m.handleSlash(msg)
	}

	// Attachments: the transcript, the clipboard and the history keep the
	// message with a one-line note; only the API receives the file contents.
	return m.sendResolved(m.app.resolveUserMessage(msg))
}

// sendResolved sends a message that is already split into what the user sees
// and what the model receives.
//
// Skills use it: dumping a skill's instructions into the transcript would show
// the user a wall of text they never typed.
func (m tuiModel) sendResolved(um userMessage) (tuiModel, tea.Cmd) {
	if m.streaming {
		return m, nil
	}

	m = m.addEntry("user", um.Display)
	m = m.refreshViewportFollow() // a new message always scrolls into view

	// The log is the record: the request is rebuilt from it, so a resumed
	// session and a live one take exactly the same path.
	if m.app.session != nil {
		if err := m.app.session.AppendUser(um.Payload); err != nil {
			m = m.addEntry("error", m.app.msg.SaveFailed+" "+err.Error())
			return m, nil
		}
	}

	m.streaming = true
	m.pendingMsg = m.app.msg.TUIThinking
	m.streamStart = time.Now()
	m.reasoning = ""
	var chatCmd tea.Cmd
	m, chatCmd = m.startChat()
	return m, tea.Batch(chatCmd, tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return slowResponseMsg{elapsed: 30 * time.Second}
	}))
}

// startChat runs the chat loop in a goroutine that writes messages to a
// channel. readChat forwards them to the tea program. It returns the model
// with chatCh set so subsequent readChat re-issues use the same channel.
func (m tuiModel) startChat() (tuiModel, tea.Cmd) {
	ch := make(chan tea.Msg, 256)
	m.chatCh = ch
	go func() {
		defer close(ch)
		ctx := context.Background()
		var buf strings.Builder
		last := time.Now()
		onChunk := func(d string) {
			buf.WriteString(d)
			if time.Since(last) > 40*time.Millisecond {
				ch <- streamChunkMsg{buf.String()}
				buf.Reset()
				last = time.Now()
			}
		}
		flush := func() {
			if buf.Len() > 0 {
				ch <- streamChunkMsg{buf.String()}
				buf.Reset()
			}
		}
		cb := chatCallbacks{
			onChunk: onChunk,
			onReasoning: func(d string) {
				ch <- reasoningMsg{d}
			},
			onStatus: func(s string) {
				ch <- infoMsg{s}
			},
			onUsage: func(u deepseek.Usage) {
				// Accounted on the UI goroutine to keep the model race-free.
				ch <- usageMsg{u}
			},
			askPerm: func(tc deepseek.ToolCall) (bool, error) {
				permCh := make(chan bool)
				ch <- toolRequestMsg{tc: tc, ch: permCh}
				return <-permCh, nil
			},
		}
		msgs := m.app.conversationMessages()
		content, _, err := m.app.chatTurn(ctx, m.client, m.model, msgs, cb)
		flush()
		ch <- streamDoneMsg{content: content, err: err}
	}()
	m.reading = true
	return m, m.readChat()
}

// chatMsg wraps a message that came from the chat goroutine.
//
// It exists so Update can tell that the reader is free again: the re-arm is
// centralised in one place instead of trusting every handler to remember it.
type chatMsg struct{ inner tea.Msg }

// chatClosedMsg reports that the chat goroutine closed its channel.
type chatClosedMsg struct{}

// readChat returns a command that forwards one message from the chat channel.
func (m tuiModel) readChat() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.chatCh
		if !ok {
			return chatClosedMsg{}
		}
		return chatMsg{inner: msg}
	}
}

// renderQuestion draws the question and its options.
func (m tuiModel) renderQuestion() string {
	var b strings.Builder
	b.WriteString(styleInfo.Render("? " + m.pendingMsg))
	for i, opt := range m.questionOpts {
		cursor := "  "
		if i == m.questionCursor {
			cursor = "▸ "
		}
		fmt.Fprintf(&b, "\n%s %d) %s", cursor, i+1, opt)
	}
	b.WriteString("\n" + styleSystem.Render(m.app.msg.QuestionHelp))
	return b.String()
}

// answerQuestion resolves a pending structured question.
//
// Digits pick directly, arrows move, Enter confirms. With multi, Space toggles
// and Enter sends everything selected.
func (m tuiModel) answerQuestion(key string) (tuiModel, tea.Cmd) {
	finish := func(chosen []string) (tuiModel, tea.Cmd) {
		if m.questionCh != nil {
			m.questionCh <- chosen
			m.questionCh = nil
		}
		m.mode = modeChat
		m.pendingMsg = ""
		m.questionOpts = nil
		m.questionPicked = nil
		return m.refreshViewport(), nil
	}

	if key == "esc" || key == "ctrl+c" {
		return finish(nil)
	}
	if key == "up" || key == "k" {
		if m.questionCursor > 0 {
			m.questionCursor--
		}
		return m, nil
	}
	if key == "down" || key == "j" {
		if m.questionCursor < len(m.questionOpts)-1 {
			m.questionCursor++
		}
		return m, nil
	}
	// A digit answers immediately: it is the fastest path.
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		idx := int(key[0] - '1')
		if idx < len(m.questionOpts) {
			return finish([]string{m.questionOpts[idx]})
		}
		return m, nil
	}
	if key == " " && m.questionPicked != nil && len(m.questionPicked) >= 0 {
		// Toggle only makes sense when several answers are allowed; the UI
		// cannot tell, so it toggles and Enter decides.
		m.questionPicked[m.questionCursor] = !m.questionPicked[m.questionCursor]
		return m, nil
	}
	if key == "enter" {
		var chosen []string
		for i, opt := range m.questionOpts {
			if m.questionPicked[i] {
				chosen = append(chosen, opt)
			}
		}
		if len(chosen) == 0 {
			chosen = []string{m.questionOpts[m.questionCursor]}
		}
		return finish(chosen)
	}
	return m, nil
}

// answerTool resolves a pending tool permission prompt.
func (m tuiModel) answerTool() (tuiModel, tea.Cmd) {
	answer := strings.ToLower(strings.TrimSpace(m.textarea.Value()))
	m.textarea.Reset()
	m.textarea.Placeholder = m.app.msg.TUIInputPlaceholder
	m.mode = modeChat
	m.pendingMsg = ""
	allowed := false
	switch answer {
	case "y", "yes":
		allowed = true
	case "a", "auto":
		// Session policy, not a per-call grant.
		m.app.approvalAuto = true
		allowed = true
		m = m.addEntry("info", m.app.msg.ApprovalAutoWarning)
	case "d", "deny":
		m.app.perm.deny[m.pendingTool] = true
	}
	if m.permCh != nil {
		m.permCh <- allowed
		m.permCh = nil
	}
	m = m.refreshViewport()
	return m, nil
}

func (m tuiModel) handleSlash(msg string) (tuiModel, tea.Cmd) {
	switch {
	case msg == "/exit":
		return m, tea.Quit
	case msg == "/help" || msg == "/help all":
		return m.addEntry("info", m.app.helpCatalog()), nil
	case strings.HasPrefix(msg, "/help "):
		topic := cmdArg(msg)
		text, ok := m.app.helpTopic(topic)
		if !ok {
			return m.addEntry("error", m.app.msg.HelpUnknownTopic(topic)), nil
		}
		return m.addEntry("info", text), nil
	case msg == "/shell":
		return m.addEntry("info", m.app.shellString()), nil
	case msg == "/run" || strings.HasPrefix(msg, "/run "):
		cmdline := cmdArg(msg)
		if strings.TrimSpace(cmdline) == "" {
			return m.addEntry("error", m.app.msg.RunUsage), nil
		}
		return m, m.runShell(cmdline)
	case msg == "/tokens":
		return m.addEntry("info", m.app.tokensString()), nil
	case msg == "/plan":
		m.app.planMode = !m.app.planMode
		return m.addEntry("info", m.app.planString()), nil
	case msg == "/jobs":
		return m.addEntry("info", m.app.jobsString()), nil
	case msg == "/todos":
		return m.addEntry("info", m.app.todosString()), nil
	case msg == "/sandbox":
		return m.addEntry("info", m.app.sandboxString()), nil
	case strings.HasPrefix(msg, "/sandbox "):
		if err := m.app.setSandbox(strings.TrimSpace(msg[len("/sandbox "):])); err != nil {
			return m.addEntry("error", err.Error()), nil
		}
		return m.addEntry("info", m.app.sandboxString()), nil
	case msg == "/balance":
		return m, m.fetchBalance()
	case msg == "/compact":
		return m, m.compactHistory()
	case msg == "/list-models":
		return m, m.listModels()
	case msg == "/models" || msg == "/model":
		return m, m.showModelSelect()
	case strings.HasPrefix(msg, "/models ") || strings.HasPrefix(msg, "/model "):
		id := cmdArg(msg)
		if id == "reset" {
			m.app.cfg.Model = ""
			_ = m.app.cfg.Save()
			return m, m.showModelSelect()
		}
		if id == "" {
			return m.addEntry("error", m.app.msg.ModelUsage), nil
		}
		return m, m.setModel(id)
	case msg == "/approvals":
		return m.addEntry("info", m.app.approvalsString()), nil
	case strings.HasPrefix(msg, "/approvals "):
		if err := m.app.setApprovals(cmdArg(msg)); err != nil {
			return m.addEntry("error", err.Error()), nil
		}
		return m.addEntry("info", m.app.approvalsString()), nil
	case msg == "/login":
		m.mode = modeSecret
		m.pendingMsg = m.app.msg.EnterAPIKey
		m.secret.Reset()
		return m, nil
	case msg == "/thinking" || strings.HasPrefix(msg, "/thinking "):
		return m.setThinking(cmdArg(msg))
	case msg == "/effort" || strings.HasPrefix(msg, "/effort "):
		return m.setEffort(cmdArg(msg))
	case msg == "/context":
		return m.addEntry("info", m.app.contextString()), nil
	case msg == "/mcp":
		return m.addEntry("info", m.app.mcpStatusString()), nil
	case msg == "/copy all" || msg == "/copy todo":
		return m, m.copyConversation()
	case msg == "/copy":
		return m, m.copyLastReply()
	case msg == "/save" || strings.HasPrefix(msg, "/save "):
		return m, m.saveTranscript(cmdArg(msg))
	default:
		if ref, ok := m.app.skillOrToolRef(msg); ok {
			// The user sees the reference and what they typed; the skill body
			// travels only in the payload.
			m.textarea.Reset()
			m.suggestions = nil
			return m.sendResolved(ref)
		}
		return m.addEntry("error", m.app.msg.UnknownSlashCmd(msg)), nil
	}
}

func (m tuiModel) listModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.client.ListModels(context.Background())
		if err != nil {
			return infoMsg{err.Error()}
		}
		var b strings.Builder
		b.WriteString(m.app.msg.ModelsTitle + "\n")
		for _, md := range models {
			marker := " "
			if md.ID == m.model {
				marker = "*"
			}
			fmt.Fprintf(&b, "  %s %s\n", marker, md.ID)
		}
		return infoMsg{b.String()}
	}
}

func (m tuiModel) showModelSelect() tea.Cmd {
	return func() tea.Msg {
		models, err := m.client.ListModels(context.Background())
		if err != nil {
			return infoMsg{err.Error()}
		}
		return modelListMsg{models}
	}
}

func (m tuiModel) setModel(id string) tea.Cmd {
	return func() tea.Msg {
		warn := false
		window := 0
		if models, err := m.client.ListModels(context.Background()); err == nil {
			if !modelAvailable(models, id) {
				warn = true // dated snapshot or unknown id: warn but allow
			}
			for _, md := range models {
				if md.ID == id {
					window = md.ContextWindow
				}
			}
		}
		return setModelMsg{id: id, warn: warn, window: window}
	}
}

func (m tuiModel) pickModelFromInput() (tuiModel, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	m.textarea.Reset()
	m.textarea.Placeholder = m.app.msg.TUIInputPlaceholder
	m.mode = modeChat
	m.pendingMsg = ""
	switch {
	case input == "":
		// keep the current model
	case modelAvailable(m.modelList, input):
		m = m.setModelNow(input)
	default:
		if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(m.modelList) {
			m = m.setModelNow(m.modelList[idx-1].ID)
		} else {
			m = m.addEntry("error", m.app.msg.InvalidModelSelection)
		}
	}
	m = m.refreshViewport()
	return m, nil
}

func (m tuiModel) setModelNow(id string) tuiModel {
	m.app.cfg.Model = id
	_ = m.app.cfg.Save()
	m.model = id
	m.app.recordModelInfo(m.modelList, id)
	return m.addEntry("info", m.app.msg.ModelSetDefault(id))
}

// setThinking shows or sets the thinking mode in the TUI.
func (m tuiModel) setThinking(v string) (tuiModel, tea.Cmd) {
	if v == "" {
		return m.addEntry("info", "thinking: "+display(m.app.cfg.Thinking, "enabled", m.app.msg.Default)), nil
	}
	v = strings.ToLower(v)
	if v == "on" {
		v = "enabled"
	}
	if v == "off" {
		v = "disabled"
	}
	if v != "enabled" && v != "disabled" {
		return m.addEntry("error", m.app.msg.InvalidThinkingValue), nil
	}
	m.app.cfg.Thinking = v
	_ = m.app.cfg.Save()
	return m.addEntry("info", m.app.msg.Saved), nil
}

// setEffort shows or sets the reasoning effort in the TUI.
func (m tuiModel) setEffort(v string) (tuiModel, tea.Cmd) {
	if v == "" {
		return m.addEntry("info", "effort: "+display(m.app.cfg.ReasoningEffort, "high", m.app.msg.Default)), nil
	}
	v = strings.ToLower(v)
	if v != "low" && v != "high" && v != "max" {
		return m.addEntry("error", m.app.msg.InvalidEffortValue), nil
	}
	m.app.cfg.ReasoningEffort = v
	_ = m.app.cfg.Save()
	return m.addEntry("info", m.app.msg.Saved), nil
}

func (m tuiModel) finishLogin() (tuiModel, tea.Cmd) {
	key := strings.TrimSpace(m.secret.String())
	m.secret.Reset()
	m.mode = modeChat
	m.pendingMsg = ""
	if key == "" {
		return m.addEntry("error", m.app.msg.NoAPIKey), nil
	}
	if err := config.SetAPIKey(m.app.cfg, key); err != nil {
		return m.addEntry("error", err.Error()), nil
	}
	return m.addEntry("info", m.app.msg.LoginOK), nil
}

// complete handles Tab: completes the current token.
func (m tuiModel) complete() tuiModel {
	value := m.textarea.Value()
	pos := len(value) // the textarea does not expose its cursor position
	comp := m.app.completer()(value, pos)
	if len(comp.Candidates) == 0 {
		m.suggestions = nil
		return m
	}
	if len(comp.Candidates) == 1 {
		m.textarea.SetValue(replaceAt(value, pos, comp.Prefix, comp.Candidates[0]))
		m.textarea.CursorEnd()
		m.suggestions = nil
		return m
	}
	cp := commonPrefix(comp.Candidates)
	if len(cp) > len(comp.Prefix) {
		m.textarea.SetValue(replaceAt(value, pos, comp.Prefix, cp))
		m.textarea.CursorEnd()
	}
	m.suggestions = comp.Candidates
	if len(m.suggestions) > tuiMaxSuggestions {
		m.suggestions = m.suggestions[:tuiMaxSuggestions]
	}
	return m
}

func (m tuiModel) computeSuggestions() []string {
	value := m.textarea.Value()
	pos := len(value) // the textarea does not expose its cursor position
	comp := m.app.completer()(value, pos)
	if len(comp.Candidates) == 0 {
		return nil
	}
	sugs := comp.Candidates
	if len(sugs) > tuiMaxSuggestions {
		sugs = sugs[:tuiMaxSuggestions]
	}
	return sugs
}

func replaceAt(value string, pos int, prefix, candidate string) string {
	start := pos - len(prefix)
	if start < 0 {
		start = 0
	}
	return value[:start] + candidate + value[pos:]
}

func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	p := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func (m tuiModel) addEntry(role, content string) tuiModel {
	m.entries = append(m.entries, chatEntry{role: role, content: content})
	return m
}

func (m tuiModel) appendAssistant(text string) tuiModel {
	if len(m.entries) > 0 && m.entries[len(m.entries)-1].role == "assistant" {
		m.entries[len(m.entries)-1].content += text
	} else {
		m.entries = append(m.entries, chatEntry{role: "assistant", content: text})
	}
	return m
}

// refreshViewport re-renders the chat history into the viewport. It returns
// the model because Bubble Tea updates are value-based.
//
// It keeps the current scroll position unless the view was already at the
// bottom: while a long answer streams in, yanking the user back to the bottom
// would make it impossible to read what scrolled past.
func (m tuiModel) refreshViewport() tuiModel {
	atBottom := m.viewport.AtBottom()
	offset := m.viewport.YOffset
	m.viewport.SetContent(m.renderEntries())
	if atBottom {
		m.viewport.GotoBottom()
	} else {
		m.viewport.SetYOffset(offset)
	}
	return m
}

// refreshViewportFollow re-renders and always scrolls to the last message.
func (m tuiModel) refreshViewportFollow() tuiModel {
	m.viewport.SetContent(m.renderEntries())
	m.viewport.GotoBottom()
	return m
}

// lastAssistantReply returns the most recent assistant entry.
func (m tuiModel) lastAssistantReply() (string, bool) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].role == "assistant" {
			return m.entries[i].content, true
		}
	}
	return "", false
}

// plainEntries renders the conversation without colours or decorations, so it
// can be copied or saved as plain text.
func (m tuiModel) plainEntries() string {
	var b strings.Builder
	for _, e := range m.entries {
		switch e.role {
		case "user":
			b.WriteString("▸ " + m.app.msg.UserLabel + "\n" + e.content + "\n\n")
		case "assistant":
			b.WriteString("◂ " + m.app.msg.AssistantLabel + "\n" + e.content + "\n\n")
		case "info":
			b.WriteString(e.content + "\n\n")
		case "error":
			b.WriteString("✗ " + e.content + "\n\n")
		default:
			b.WriteString(e.content + "\n\n")
		}
	}
	return b.String()
}

// copyLastReply copies the last assistant answer to the clipboard.
func (m tuiModel) copyLastReply() tea.Cmd {
	reply, ok := m.lastAssistantReply()
	if !ok || strings.TrimSpace(reply) == "" {
		return func() tea.Msg { return copiedMsg{err: clipboard.ErrEmpty} }
	}
	return copyText(reply)
}

// copyConversation copies the whole chat to the clipboard.
func (m tuiModel) copyConversation() tea.Cmd {
	text := m.plainEntries()
	if strings.TrimSpace(text) == "" {
		return func() tea.Msg { return copiedMsg{err: clipboard.ErrEmpty} }
	}
	return copyText(text)
}

func copyText(text string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.Copy(text); err != nil {
			return copiedMsg{err: err}
		}
		return copiedMsg{chars: len([]rune(text))}
	}
}

// saveTranscript writes the conversation as Markdown to path (or to a
// timestamped file in the working directory when path is empty).
func (m tuiModel) saveTranscript(path string) tea.Cmd {
	text := m.plainEntries()
	return func() tea.Msg {
		abs, err := saveTranscriptFile(path, text)
		if err != nil {
			return savedMsg{err: err}
		}
		return savedMsg{path: abs}
	}
}

// runShell runs a /run command without blocking the UI.
func (m tuiModel) runShell(cmdline string) tea.Cmd {
	timeout := m.app.shellTimeout()
	return func() tea.Msg {
		res, err := m.app.runShellCommand(context.Background(), cmdline, "", timeout)
		return runDoneMsg{command: cmdline, res: res, err: err}
	}
}

// fetchBalance queries the account balance off the UI goroutine.
func (m tuiModel) fetchBalance() tea.Cmd {
	return func() tea.Msg {
		text, err := m.app.balanceString(context.Background())
		return balanceMsg{text: text, err: err}
	}
}

// compactHistory summarises the oldest part of the conversation off the UI
// goroutine.
func (m tuiModel) compactHistory() tea.Cmd {
	return func() tea.Msg {
		before, after, err := m.app.compactNow(context.Background(), m.client, m.model, chatCallbacks{})
		return compactMsg{before: before, after: after, err: err}
	}
}

// headerText is the title bar: model, session token counter and a warning when
// the session is running without tool approvals.
func (m tuiModel) headerText() string {
	head := "DeepSeek Coder · " + m.model
	if total := m.app.tokensIn + m.app.tokensOut; total > 0 {
		head += fmt.Sprintf(" · ↑%s ↓%s", compactTokens(m.app.tokensIn), compactTokens(m.app.tokensOut))
	}
	if m.app.approvalAuto {
		head += " · ⚠ sin confirmaciones"
	}
	if m.app.planMode {
		head += " · 📋 plan"
	}
	// The sandbox mode is only worth a badge when it is not the default.
	switch m.app.sandboxMode {
	case "read-only":
		head += " · 🔒 solo lectura"
	case "full":
		head += " · ⚠ sin confinamiento"
	}
	return head
}

// renderEntries renders the whole conversation, reusing the per-entry cache.
//
// It takes a pointer receiver because the cache it fills lives on the model:
// with a value receiver the reallocated slice would be thrown away and every
// call would rebuild the whole history.
func (m *tuiModel) renderEntries() string {
	width := m.viewport.Width - 4
	if width < markdown.MinWidth {
		width = markdown.MinWidth
	}
	if len(m.renderCache) != len(m.entries) || m.renderWidth != width {
		m.renderCache = make([]renderedEntry, len(m.entries))
		m.renderWidth = width
	}
	var b strings.Builder
	for i, e := range m.entries {
		if c := m.renderCache[i]; c.src == e.content && c.out != "" {
			b.WriteString(c.out)
			continue
		}
		out := m.renderEntry(e, width)
		m.renderCache[i] = renderedEntry{src: e.content, out: out}
		b.WriteString(out)
	}
	return b.String()
}

// renderEntry renders one chat entry. Assistant answers go through the
// Markdown renderer; everything else is already plain text produced by the CLI.
func (m tuiModel) renderEntry(e chatEntry, width int) string {
	switch e.role {
	case "user":
		return styleUser.Render("▸ "+m.app.msg.UserLabel) + "\n" + e.content + "\n\n"
	case "assistant":
		return styleAI.Render("◂ "+m.app.msg.AssistantLabel) + "\n" + markdown.Render(e.content, width) + "\n\n"
	case "error":
		return styleError.Render("✗ "+e.content) + "\n\n"
	case "info":
		return styleInfo.Render(e.content) + "\n\n"
	default:
		return styleSystem.Render(e.content) + "\n\n"
	}
}

func (m tuiModel) handleResize() tuiModel {
	m.viewport.Width = m.width
	m.textarea.SetWidth(m.width)
	m.help.Width = m.width
	headerH := 1
	pendingH := 0
	if m.streaming || m.mode != modeChat {
		pendingH = 1
	}
	if m.mode == modeQuestion {
		pendingH = len(m.questionOpts) + 2
	}
	sugH := 0
	if len(m.suggestions) > 0 {
		sugH = 1
	}
	inputH := 3
	helpH := 1
	todoH := 0
	if m.app.session != nil {
		if list := m.app.session.Todos(); len(list) > 0 {
			lines := len(strings.Split(list.Render(), "\n"))
			if lines > 4 {
				lines = 4
			}
			todoH = lines + 1 // title
		}
	}
	m.viewport.Height = m.height - headerH - pendingH - sugH - todoH - inputH - helpH
	if m.viewport.Height < 5 {
		m.viewport.Height = 5
	}
	return m
}

// tuiKeyMap implements help.KeyMap.
type tuiKeyMap struct {
	send, newline, complete, copyKey, scroll, quit key.Binding
}

func (k tuiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.send, k.newline, k.complete, k.copyKey, k.scroll, k.quit}
}

func (k tuiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func (m tuiModel) keys() tuiKeyMap {
	return tuiKeyMap{
		send:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.app.msg.TUIKeySend)),
		newline:  key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", m.app.msg.TUIKeyNewline)),
		complete: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", m.app.msg.TUIKeyComplete)),
		copyKey:  key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", m.app.msg.TUIKeyCopy)),
		scroll:   key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", m.app.msg.TUIKeyScroll)),
		quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", m.app.msg.TUIKeyQuit)),
	}
}

// todoPanel renders the current task list above the input box.
//
// It is a fixed-height panel rather than a scrolling one: the point is to see
// at a glance what is pending, so it shows only the first few lines and a
// counter when the list is long.
func (m tuiModel) todoPanel() string {
	if m.app.session == nil {
		return ""
	}
	list := m.app.session.Todos()
	if len(list) == 0 {
		return ""
	}
	const maxLines = 4
	lines := strings.Split(list.Render(), "\n")
	if len(lines) > maxLines {
		extra := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("… %d más", extra))
	}
	if m.width > 0 {
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
	}
	head := styleSystem.Render(m.app.msg.TodosTitle + " " + list.Progress())
	return lipgloss.JoinVertical(lipgloss.Left, head, styleSystem.Render(strings.Join(lines, "\n")))
}
func (m tuiModel) View() string {
	header := styleHeader.Render(m.headerText())
	vp := m.viewport.View()

	var pending string
	switch {
	case m.streaming:
		elapsed := int(time.Since(m.streamStart).Seconds())
		pending = m.spinner.View() + " " + m.pendingMsg + fmt.Sprintf(" (%ds)", elapsed)
		if m.reasoning != "" {
			pending += " " + styleSystem.Render("· "+m.reasoning)
		}
	case m.mode == modeConfirmTool:
		pending = styleInfo.Render(m.pendingMsg)
	case m.mode == modeQuestion:
		pending = m.renderQuestion()
	case m.mode == modeModelSelect:
		pending = styleInfo.Render(m.pendingMsg)
	case m.mode == modeSecret:
		pending = styleInfo.Render(m.pendingMsg)
	}

	var sug string
	if len(m.suggestions) > 0 {
		sug = styleSystem.Render(strings.Join(m.suggestions, "  "))
		// One line only: a wrapped suggestion would change the layout height
		// and push the input box off the screen.
		if m.width > 0 {
			sug = ansi.Truncate(sug, m.width, "…")
		}
	}

	var input string
	if m.mode == modeSecret {
		input = stylePrompt.Render("> ") + strings.Repeat("•", m.secret.Len())
	} else {
		input = m.textarea.View()
	}

	todoPanel := m.todoPanel()

	helpLine := m.help.View(m.keys())
	// Keep the help bar on a single line: if it wrapped, the layout height
	// computed for the viewport would be off and the input would drift.
	if m.width > 0 {
		helpLine = ansi.Truncate(helpLine, m.width, "…")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, vp, pending, sug, todoPanel, input, helpLine)
}
