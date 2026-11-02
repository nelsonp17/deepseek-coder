package main

import (
	"context"
	"fmt"
	"os"
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

	"deekseep-coder/internal/config"
	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/history"
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
	modeModelSelect
	modeSecret
)

type chatEntry struct {
	role    string // user, assistant, system, error, info
	content string
}

// Messages exchanged with the tea program.
type streamChunkMsg struct{ text string }
type streamDoneMsg struct {
	content  string
	messages []deepseek.Message
	err      error
}
type toolRequestMsg struct {
	tc deepseek.ToolCall
	ch chan bool
}
type infoMsg struct{ text string }
type modelListMsg struct{ models []deepseek.Model }
type setModelMsg struct {
	id   string
	warn bool
}
type reasoningMsg struct{ text string }

type tuiModel struct {
	app    *app
	client *deepseek.Client
	model  string
	store  *history.Store
	convID int64

	messages []deepseek.Message

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

	permCh      chan bool
	pendingTool string
	modelList   []deepseek.Model
	secret      strings.Builder
	suggestions []string

	chatCh chan tea.Msg // messages from the chat goroutine

	streamStart time.Time
	reasoning   string // live reasoning content of thinking models
}

// slowResponseMsg fires when the model has not produced any content for a
// while, so the user knows the request is still alive.
type slowResponseMsg struct{ elapsed time.Duration }

// replTUI runs the interactive chat as a full-screen TUI.
func (a *app) replTUI(client *deepseek.Client, model string) error {
	store, err := history.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, a.msg.HistoryDisabled, err)
		store = nil
	}
	m := newTUIModel(a, client, model, store)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func newTUIModel(a *app, client *deepseek.Client, model string, store *history.Store) tuiModel {
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
		store:    store,
		messages: []deepseek.Message{{Role: "system", Content: a.systemPrompt()}},
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		help:     help.New(),
	}
	m = m.addEntry("info", a.msg.Welcome)
	m.viewport.SetContent(m.renderEntries())
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textarea.Blink)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		cmds = append(cmds, m.readChat())

	case streamDoneMsg:
		m.streaming = false
		m.pendingMsg = ""
		if msg.err != nil {
			m = m.addEntry("error", msg.err.Error())
		} else {
			m.messages = msg.messages
			if m.store != nil {
				if err := m.store.AppendMessage(m.convID, "assistant", msg.content); err != nil {
					m = m.addEntry("error", m.app.msg.SaveFailed+" "+err.Error())
				}
			}
		}
		m = m.refreshViewport()

	case toolRequestMsg:
		m.permCh = msg.ch
		m.pendingTool = msg.tc.Function.Name
		m.mode = modeConfirmTool
		m.pendingMsg = m.app.msg.ToolRequest(msg.tc.Function.Name, msg.tc.Function.Arguments)
		m.textarea.Reset()
		m.textarea.Placeholder = m.app.msg.ToolPrompt
		m = m.refreshViewport()

	case infoMsg:
		m = m.addEntry("info", msg.text)
		m = m.refreshViewport()
		cmds = append(cmds, m.readChat())

	case reasoningMsg:
		m.reasoning += msg.text
		if len(m.reasoning) > 120 {
			m.reasoning = m.reasoning[:120]
		}
		cmds = append(cmds, m.readChat())

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
		m = m.setModelNow(msg.id)
		m = m.refreshViewport()

	case slowResponseMsg:
		if m.streaming {
			m = m.addEntry("info", m.app.msg.SlowResponse(msg.elapsed))
			m = m.refreshViewport()
		}
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
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		if msg.Alt {
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

	m = m.addEntry("user", msg)
	m.messages = append(m.messages, deepseek.Message{Role: "user", Content: msg})

	if m.store != nil && m.convID == 0 {
		id, err := m.store.CreateConversation(shortTitle(msg))
		if err != nil {
			m = m.addEntry("error", m.app.msg.SaveFailed+" "+err.Error())
			m.store = nil
		} else {
			m.convID = id
		}
	}
	if m.store != nil {
		if err := m.store.AppendMessage(m.convID, "user", msg); err != nil {
			m = m.addEntry("error", m.app.msg.SaveFailed+" "+err.Error())
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
			askPerm: func(tc deepseek.ToolCall) (bool, error) {
				permCh := make(chan bool)
				ch <- toolRequestMsg{tc: tc, ch: permCh}
				return <-permCh, nil
			},
		}
		content, messages, err := m.app.chatTurn(ctx, m.client, m.model, m.messages, cb)
		flush()
		ch <- streamDoneMsg{content: content, messages: messages, err: err}
	}()
	return m, m.readChat()
}

// readChat returns a command that forwards one message from the chat channel.
func (m tuiModel) readChat() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.chatCh
		if !ok {
			return nil // channel closed
		}
		return msg
	}
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
	case "a", "always":
		m.app.perm.allow[m.pendingTool] = true
		allowed = true
	case "d", "deny":
		m.app.perm.deny[m.pendingTool] = true
	}
	if m.permCh != nil {
		m.permCh <- allowed
		m.permCh = nil
	}
	m = m.refreshViewport()
	return m, m.readChat()
}

func (m tuiModel) handleSlash(msg string) (tuiModel, tea.Cmd) {
	switch {
	case msg == "/exit":
		return m, tea.Quit
	case msg == "/help":
		return m.addEntry("info", m.app.msg.SlashHelp), nil
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
	default:
		if ref, ok := m.app.skillOrToolRef(msg); ok {
			m.textarea.SetValue(ref)
			return m.sendMessage()
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
		if models, err := m.client.ListModels(context.Background()); err == nil {
			if !modelAvailable(models, id) {
				warn = true // dated snapshot or unknown id: warn but allow
			}
		}
		return setModelMsg{id: id, warn: warn}
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
func (m tuiModel) refreshViewport() tuiModel {
	m.viewport.SetContent(m.renderEntries())
	m.viewport.GotoBottom()
	return m
}

func (m tuiModel) renderEntries() string {
	var b strings.Builder
	for _, e := range m.entries {
		switch e.role {
		case "user":
			b.WriteString(styleUser.Render("▸ tú") + "\n" + e.content + "\n\n")
		case "assistant":
			b.WriteString(styleAI.Render("◂ deepseek") + "\n" + e.content + "\n\n")
		case "error":
			b.WriteString(styleError.Render("✗ "+e.content) + "\n\n")
		case "info":
			b.WriteString(styleInfo.Render(e.content) + "\n\n")
		default:
			b.WriteString(styleSystem.Render(e.content) + "\n\n")
		}
	}
	return b.String()
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
	sugH := 0
	if len(m.suggestions) > 0 {
		sugH = 1
	}
	inputH := 3
	helpH := 1
	m.viewport.Height = m.height - headerH - pendingH - sugH - inputH - helpH
	if m.viewport.Height < 5 {
		m.viewport.Height = 5
	}
	return m
}

// tuiKeyMap implements help.KeyMap.
type tuiKeyMap struct {
	send, newline, complete, scroll, quit key.Binding
}

func (k tuiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.send, k.newline, k.complete, k.scroll, k.quit}
}

func (k tuiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func (m tuiModel) keys() tuiKeyMap {
	return tuiKeyMap{
		send:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", m.app.msg.TUIKeySend)),
		newline:  key.NewBinding(key.WithKeys("alt+enter"), key.WithHelp("alt+enter", m.app.msg.TUIKeyNewline)),
		complete: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", m.app.msg.TUIKeyComplete)),
		scroll:   key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", m.app.msg.TUIKeyScroll)),
		quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", m.app.msg.TUIKeyQuit)),
	}
}

func (m tuiModel) View() string {
	header := styleHeader.Render(fmt.Sprintf("DeepSeek Coder · %s", m.model))
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
	case m.mode == modeModelSelect:
		pending = styleInfo.Render(m.pendingMsg)
	case m.mode == modeSecret:
		pending = styleInfo.Render(m.pendingMsg)
	}

	var sug string
	if len(m.suggestions) > 0 {
		sug = styleSystem.Render(strings.Join(m.suggestions, "  "))
	}

	var input string
	if m.mode == modeSecret {
		input = stylePrompt.Render("> ") + strings.Repeat("•", m.secret.Len())
	} else {
		input = m.textarea.View()
	}

	helpLine := m.help.View(m.keys())

	return lipgloss.JoinVertical(lipgloss.Left, header, vp, pending, sug, input, helpLine)
}
