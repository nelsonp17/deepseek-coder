package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"deepseek-coder/internal/attach"
	"deepseek-coder/internal/clipboard"
	"deepseek-coder/internal/compact"
	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/editor"
	"deepseek-coder/internal/fstools"
	"deepseek-coder/internal/i18n"
	"deepseek-coder/internal/jobs"
	"deepseek-coder/internal/mcpx"
	"deepseek-coder/internal/project"
	"deepseek-coder/internal/session"
	"deepseek-coder/internal/shell"
	"deepseek-coder/internal/tokens"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type app struct {
	cfg  *config.Config
	msg  *i18n.Messages
	proj *project.Project
	mcp  *mcpx.Manager
	perm *permissions

	// lastReply is the most recent assistant answer, so /copy can put it on
	// the clipboard without the user having to select it. transcript keeps
	// the whole session so /save can dump it to a file.
	lastReply  string
	transcript strings.Builder

	// Token accounting for the session. tokensIn/tokensOut accumulate every
	// API round (tool loops bill more than one), lastUsage is the most recent
	// request so the context size can be shown against the model window.
	tokensIn    int
	tokensOut   int
	lastUsage   deepseek.Usage
	modelWindow int // context window of the current model, 0 when unknown

	// sandboxMode is how much the tools may touch: read-only, workspace-write or
	// full. It is the other half of the permission model.
	sandboxMode string

	// approvalAuto is the session approval policy: false asks before every tool
	// call, true runs without asking. It is deliberately session-scoped and
	// visible in the UI: the old "always" remembered a grant per tool name,
	// which for run_shell meant "any command until you close the CLI".
	approvalAuto bool

	// json switches one-shot output (ask, --pipe) to NDJSON events, so a script
	// or a CI job can consume the answer and the token usage without parsing
	// prose.
	json bool

	// fs holds the workspace file tools and the read-before-edit guard.
	fs *fstools.Set

	// askUser asks the human a structured question. It is nil where the CLI
	// cannot own stdin, and the tool says so instead of stealing input.
	askUser func(question string, options []string, multi bool) ([]string, error)

	// planMode keeps the model reading and planning until the user approves the
	// plan, so a big change is agreed before a single file is touched.
	planMode bool

	// jobs runs long commands in the background and reports when they finish.
	jobs *jobs.Manager

	// session is the durable log of the conversation. It is nil for one-shot
	// asks, which have nothing to resume.
	session *session.Session
	// resumeID and continueLast select an existing session at startup.
	resumeID     string
	continueLast bool
}

// systemPrompt builds the system prompt including the project context.
func (a *app) systemPrompt() string {
	sp := a.msg.SystemPrompt
	sp += "\n\n" + todoHint
	if a.planMode {
		sp += "\n\n" + planHint
	}
	if a.proj != nil {
		if ctx := a.proj.Context(); ctx != "" {
			sp += "\n\n" + ctx
		}
	}
	sp += "\n\n" + a.environmentString()
	return sp
}

// loadProject loads the project context honouring the configured mode.
func (a *app) loadProject() {
	if a.cfg.ContextMode() == "off" {
		a.proj = nil
		return
	}
	a.proj, _ = project.LoadWith(".", a.projectOptions())
}

// projectOptions describes what to load in the current mode. "light" drops the
// README and the git diff, which are the two big contributors; the branch,
// status and recent commits are cheap and stay.
func (a *app) projectOptions() project.Options {
	if a.cfg.ContextMode() == "light" {
		return project.Options{}
	}
	return project.Full
}

// todoHint tells the model how to use the task list without being nagged about
// it on every turn.
const todoHint = `When the work takes more than one step, keep a task list with the todo_write tool: rewrite it as you go, keep exactly one task in_progress, and mark a task completed only when it is truly finished. The user sees the list while you work. Do not use it for trivial one-step requests.`

// refreshSystemPrompt rewrites the system message before every request so it
// carries the current date and project context.
//
// It used to be built once when the session started, which left the date stale
// as soon as the session crossed midnight and made /context reload pointless.
func (a *app) refreshSystemPrompt(messages []deepseek.Message) []deepseek.Message {
	content := a.systemPrompt()
	if len(messages) > 0 && messages[0].Role == "system" {
		messages[0].Content = content
		return messages
	}
	return append([]deepseek.Message{{Role: "system", Content: content}}, messages...)
}

// environmentString describes the machine the assistant is running on. Without
// it the model proposes bash commands on Windows (or PowerShell on Linux) and
// every run_shell call fails.
func (a *app) environmentString() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "?"
	}
	tool := a.msg.EnvToolOff
	if a.cfg.ShellEnabled() {
		tool = a.msg.EnvToolOn
	}
	var b strings.Builder
	b.WriteString(a.msg.EnvTitle + "\n")
	fmt.Fprintf(&b, "- %s: %s/%s\n", a.msg.EnvOS, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- %s: %s (%s)\n", a.msg.EnvShell, shell.Name(), shell.Detect())
	fmt.Fprintf(&b, "- %s: %s\n", a.msg.EnvDir, cwd)
	fmt.Fprintf(&b, "- %s: %s\n", a.msg.EnvTool, tool)
	// The model has no clock of its own: without this line it guesses the
	// weekday (and gets it wrong) even when it could run a shell command.
	fmt.Fprintf(&b, "- %s: %s\n", a.msg.EnvNow, time.Now().Format("2006-01-02 15:04:05 -07:00 (Monday)"))
	b.WriteString(a.msg.EnvNowHint + "\n")
	b.WriteString(a.msg.EnvSyntax)
	return b.String()
}

func run(args []string) error {
	// --version must work before anything else: a broken config or an
	// unavailable keyring should not stop a user from asking which build they
	// are running.
	if wantsVersion(args) {
		fmt.Println(versionString())
		return nil
	}
	// --debug enables verbose logging of API requests.
	if len(args) > 0 && (args[0] == "--debug" || args[0] == "-d") {
		os.Setenv("DEEPSEEK_DEBUG", "1")
		args = args[1:]
	}
	// --json may appear anywhere: it switches one-shot output to NDJSON.
	// --continue and --resume pick an existing session for the interactive chat.
	wantJSON := false
	continueLast := false
	resumeID := ""
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			wantJSON = true
		case arg == "-c" || arg == "--continue":
			continueLast = true
		case arg == "--resume":
			if i+1 < len(args) {
				resumeID = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--resume="):
			resumeID = strings.TrimPrefix(arg, "--resume=")
		default:
			filtered = append(filtered, arg)
		}
	}
	args = filtered

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// First run: ask for the preferred language. Skipped when the user is
	// explicitly setting it, or when stdin is not interactive (defaults to
	// English to avoid hanging on pipes).
	explicitLang := len(args) >= 3 && args[0] == "config" && args[1] == "set" && args[2] == "lang"
	if cfg.Lang == "" && !explicitLang {
		if isInteractive() {
			lang, err := promptLanguage(os.Stdin)
			if err != nil {
				return err
			}
			cfg.Lang = string(lang)
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Println(i18n.For(lang).LanguageSet)
		} else {
			cfg.Lang = string(i18n.EN)
		}
	}
	a := &app{
		cfg:          cfg,
		msg:          i18n.For(i18n.Lang(cfg.Lang)),
		perm:         &permissions{deny: map[string]bool{}},
		json:         wantJSON,
		resumeID:     resumeID,
		continueLast: continueLast,
	}
	// The file tools are rooted at the directory the CLI was launched from, and
	// their read-before-edit memory lasts for the whole session.
	if set, err := fstools.New("."); err == nil {
		a.fs = set
	}
	a.jobs = jobs.New()
	a.sandboxMode = cfg.SandboxMode()
	a.approvalAuto = cfg.ApprovalAuto()
	a.applySandbox()

	// --pipe turns stdin into the question, so the command fits in a shell
	// pipeline: cat main.go | deepseek --pipe "revisa esto".
	if len(args) > 0 && (args[0] == "--pipe" || args[0] == "-p") {
		q, err := a.pipeQuestion(args[1:], os.Stdin, isInteractive())
		if err != nil {
			return err
		}
		args = []string{"ask", q}
	}

	if len(args) == 0 {
		return a.repl()
	}
	switch args[0] {
	case "ask":
		err := a.ask(strings.Join(args[1:], " "))
		if err != nil && a.json {
			writeJSONEvent(jsonEvent{Type: "error", Message: err.Error()})
		}
		return err
	case "config":
		return a.configCmd(args[1:])
	case "history":
		return a.historyCmd(args[1:])
	case "models":
		return a.modelsCmd()
	case "balance", "saldo":
		return a.balanceCmd()
	case "mcp":
		return a.mcpCmd()
	case "help", "-h", "--help":
		fmt.Print(a.msg.Usage)
		return nil
	default:
		fmt.Print(a.msg.Usage)
		return errors.New(a.msg.UnknownCommand(args[0]))
	}
}

// isInteractive reports whether stdin is a terminal. It also accepts
// character devices as a fallback for Windows setups (Git Bash, ConPTY)
// where stdin is a pipe even when interactive.
func isInteractive() bool {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// stdinInteractive reports whether the user can answer a permission prompt.
//
// It is stricter than isInteractive(): a character device that is not a real
// terminal accepts the prompt text but fails the read, which leaves the prompt
// on screen and denies the call anyway. Requiring a terminal keeps scripted
// runs quiet and predictable.
var stdinInteractive = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func promptLanguage(r io.Reader) (i18n.Lang, error) {
	fmt.Print(i18n.For(i18n.EN).SelectLanguage)
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	if lang, ok := i18n.Parse(line); ok {
		return lang, nil
	}
	return i18n.ES, nil // default to Spanish
}

func (a *app) newClient() (*deepseek.Client, error) {
	apiKey := config.GetAPIKey(a.cfg)
	if apiKey == "" {
		if !isInteractive() {
			return nil, errors.New(a.msg.NoAPIKey)
		}
		key, err := a.promptAPIKey()
		if err != nil {
			return nil, err
		}
		if err := config.SetAPIKey(a.cfg, key); err != nil {
			return nil, err
		}
		apiKey = key
	}
	return a.buildClient(apiKey), nil
}

// buildClient creates a client without prompting for an API key (used by
// commands that can work without one, like `models`).
func (a *app) buildClient(apiKey string) *deepseek.Client {
	model := a.cfg.Model
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		model = v
	}
	return deepseek.New(deepseek.Options{
		APIKey:  apiKey,
		BaseURL: a.cfg.BaseURL,
		Model:   model,
	})
}

// currentModel resolves the effective model: session override > env > config > default.
func currentModel(cfg *config.Config, sessionModel string) string {
	if sessionModel != "" {
		return sessionModel
	}
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		return v
	}
	if cfg.Model != "" {
		return cfg.Model
	}
	return deepseek.DefaultModel
}

func (a *app) modelsCmd() error {
	client := a.buildClient(config.GetAPIKey(a.cfg))
	return a.printModels(client, currentModel(a.cfg, ""))
}

// ensureModel validates the configured model against the API. When no model
// is configured, or the configured one is no longer available, it prompts the
// user to pick one (and saves it as the default). Returns the effective model.
func (a *app) ensureModel(client *deepseek.Client) (string, error) {
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		return v, nil // env var is explicit
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		// Cannot validate; fall back to the configured model.
		fmt.Fprintln(os.Stderr, a.msg.ModelsUnavailable, err)
		return a.cfg.Model, nil
	}
	if a.cfg.Model == "" {
		if !stdinInteractive() {
			return "", errors.New(a.msg.NoModelNonInteractive)
		}
		return a.promptModelSelection(models, "")
	}
	if modelAvailable(models, a.cfg.Model) {
		a.recordModelInfo(models, a.cfg.Model)
		return a.cfg.Model, nil
	}
	if !stdinInteractive() {
		return "", errors.New(a.msg.ModelInvalidNonInteractive(a.cfg.Model))
	}
	fmt.Println(a.msg.ModelNotAvailable(a.cfg.Model))
	return a.promptModelSelection(models, a.cfg.Model)
}

// promptModelSelection shows a numbered list of models and asks the user to
// pick one (by number or ID). The selection is saved as the default model.
// Pressing Enter with no input keeps the previous model when there is one.
func (a *app) promptModelSelection(models []deepseek.Model, previous string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println(a.msg.SelectModel)
		for i, m := range models {
			marker := " "
			if m.ID == previous {
				marker = "*"
			}
			name := m.Name
			if name == "" {
				name = "-"
			}
			fmt.Printf("  %d) %s %s %s\n", i+1, marker, m.ID, name)
		}
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		input := strings.TrimSpace(line)
		if input == "" && previous != "" {
			return previous, nil // keep the current model
		}
		if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(models) {
			return a.saveDefaultModel(models[idx-1].ID)
		}
		if modelAvailable(models, input) {
			return a.saveDefaultModel(input)
		}
		fmt.Println(a.msg.InvalidModelSelection)
	}
}

// saveDefaultModel persists the model as the default and confirms it.
func (a *app) saveDefaultModel(id string) (string, error) {
	a.cfg.Model = id
	if err := a.cfg.Save(); err != nil {
		return "", err
	}
	fmt.Println(a.msg.ModelSetDefault(id))
	return id, nil
}

func modelAvailable(models []deepseek.Model, id string) bool {
	for _, m := range models {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (a *app) printModels(client *deepseek.Client, current string) error {
	ctx := context.Background()
	models, err := client.ListModels(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		fmt.Println(a.msg.NoModels)
		return nil
	}
	fmt.Println(a.msg.ModelsTitle)
	for _, m := range models {
		marker := " "
		if m.ID == current {
			marker = "*"
		}
		name := m.Name
		if name == "" {
			name = "-"
		}
		fmt.Printf("%s %-22s %-28s ctx: %-9d out: %d\n", marker, m.ID, name, m.ContextWindow, m.MaxOutputTokens)
	}
	return nil
}

func (a *app) promptAPIKey() (string, error) {
	fmt.Print(a.msg.EnterAPIKey + " ")
	var key string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		key = string(pw)
	} else {
		// Fallback for non-console terminals (Git Bash, ConPTY) and tests.
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		key = line
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New(a.msg.NoAPIKey)
	}
	return key, nil
}

// userMessage is what the user typed, split into what the chat shows and what
// the model actually receives. Attachments (@path) only ever reach the model:
// showing 30 KB of file in the transcript would wreck the log, the clipboard
// and the SQLite history.
type userMessage struct {
	Display string // message plus a compact note of the attachments
	Payload string // message plus the file contents
	Note    string // just the attachment line; empty when there is nothing to say
}

// resolveUserMessage expands the @path references of a message.
func (a *app) resolveUserMessage(msg string) userMessage {
	payload, atts := attach.Resolve(msg, a.attachOptions())
	if len(atts) == 0 {
		return userMessage{Display: msg, Payload: msg}
	}
	note := a.attachmentNote(atts)

	included := 0
	for _, at := range atts {
		if at.Err == nil {
			included++
		}
	}
	if included == 0 {
		// Nothing could be attached: say why, but leave the payload alone.
		return userMessage{Display: msg + "\n\n" + note, Payload: msg, Note: note}
	}
	return userMessage{Display: msg + "\n\n" + note, Payload: payload, Note: note}
}

// attachOptions wires the localized strings into the attach package.
func (a *app) attachOptions() attach.Options {
	return attach.Options{
		Header:  a.msg.AttachHeader,
		FileTag: a.msg.AttachFileTag,
	}
}

// attachmentNote renders the one-line summary shown to the user.
func (a *app) attachmentNote(atts []attach.Attachment) string {
	parts := make([]string, 0, len(atts))
	for _, at := range atts {
		if at.Err != nil {
			parts = append(parts, a.msg.AttachSkipped(at.Path, a.attachReason(at.Err)))
			continue
		}
		parts = append(parts, a.msg.AttachItem(at.Path, attach.HumanBytes(at.Bytes), at.Truncated))
	}
	if len(parts) == 0 {
		return ""
	}
	return a.msg.AttachPrefix + strings.Join(parts, " · ")
}

// attachReason translates an attachment failure.
func (a *app) attachReason(err error) string {
	switch {
	case errors.Is(err, attach.ErrNotFound):
		return a.msg.AttachNotFound
	case errors.Is(err, attach.ErrDirectory):
		return a.msg.AttachDirectory
	case errors.Is(err, attach.ErrTooLarge):
		return a.msg.AttachTooLarge
	case errors.Is(err, attach.ErrBinary):
		return a.msg.AttachBinary
	case errors.Is(err, attach.ErrBudget):
		return a.msg.AttachBudget
	case errors.Is(err, attach.ErrRead):
		return a.msg.AttachReadFailed
	default:
		return err.Error()
	}
}

// defaultContextWindow is assumed when neither the API nor the config report a
// window. It is deliberately conservative: compacting early costs a summary,
// compacting late costs a failed request.
const defaultContextWindow = 65536

// contextWindow returns the window to plan compaction against.
//
// An explicit setting wins over the API: a user who sets a smaller window wants
// the CLI to compact earlier, which is exactly what the setting is for.
func (a *app) contextWindow() int {
	if a.cfg.ContextWindow > 0 {
		return a.cfg.ContextWindow
	}
	if a.modelWindow > 0 {
		return a.modelWindow
	}
	return defaultContextWindow
}

// jobNotification renders a finished job as the message injected into the
// conversation, so the model learns about it without polling for it.
func jobNotification(r jobs.Result) string {
	status := fmt.Sprintf("exit %d", r.ExitCode)
	switch {
	case r.TimedOut:
		status = "timed out"
	case r.Err != nil && r.ExitCode == 0:
		status = r.Err.Error()
	}
	out := r.Output
	const maxJobOutput = 4000
	if len(out) > maxJobOutput {
		out = out[:maxJobOutput] + "\n[…output truncated…]"
	}
	return fmt.Sprintf("[background job #%d finished (%s, %s)]\n$ %s\n%s",
		r.ID, status, r.Duration.Round(time.Millisecond), r.Command, strings.TrimRight(out, "\n"))
}

// injectJobResults drains the finished jobs into the session. The model sees
// them as a user message on its next turn.
func (a *app) injectJobResults() []string {
	results := a.jobs.Drain()
	if len(results) == 0 {
		return nil
	}
	out := make([]string, 0, len(results))
	for _, r := range results {
		note := jobNotification(r)
		if a.session != nil {
			_ = a.session.AppendUser(note)
		}
		out = append(out, note)
	}
	return out
}

// jobsString lists the jobs still running.
func (a *app) jobsString() string {
	running := a.jobs.Running()
	if len(running) == 0 {
		return a.msg.JobsNone
	}
	var b strings.Builder
	b.WriteString(a.msg.JobsTitle)
	for _, job := range running {
		fmt.Fprintf(&b, "\n  #%d  %s  (%s)", job.ID, job.Command, time.Since(job.Started).Round(time.Second))
	}
	return b.String()
}

// pressure estimates what the next request will cost, including the system
// prompt that is rebuilt for every turn.
func (a *app) pressure() int {
	if a.session == nil {
		return 0
	}
	snap := a.session.Snapshot()
	return tokens.Pressure(snap.Messages, snap.AnchorUsage, snap.AnchorLen) + tokens.Text(a.systemPrompt())
}

// pruneNow shortens the old tool output that a compaction would summarise away.
// It costs nothing: no model call, just a derived view of the log.
func (a *app) pruneNow(cb chatCallbacks) (int, bool, error) {
	if a.session == nil {
		return 0, false, nil
	}
	snap := a.session.Snapshot()
	window := a.contextWindow()
	span, ok := compact.Choose(snap.Messages, window, compact.Default)
	if !ok {
		return 0, false, nil
	}
	candidates := 0
	for _, m := range span.Messages {
		if compact.Prunable(m) {
			candidates++
		}
	}
	if candidates == 0 {
		return 0, false, nil
	}
	before := tokens.Messages(snap.Messages)
	if err := a.session.AppendPrune(snap.Seqs[span.From], snap.Seqs[span.To], 0); err != nil {
		return 0, false, err
	}
	after := tokens.Messages(a.session.Messages())
	saved := before - after
	if saved < 0 {
		saved = 0
	}
	return saved, true, nil
}

// compactNow replaces the oldest span of the session with a summary and returns
// the estimated size before and after.
func (a *app) compactNow(ctx context.Context, client *deepseek.Client, model string, cb chatCallbacks) (int, int, error) {
	if a.session == nil {
		return 0, 0, errors.New(a.msg.CompactionNoSession)
	}
	window := a.contextWindow()
	if window <= 0 {
		return 0, 0, errors.New(a.msg.CompactionNoWindow)
	}
	snap := a.session.Snapshot()
	before := tokens.Messages(snap.Messages)
	// Shorten old tool output first: it is free, and the span is about to be
	// summarised anyway, so the summary call gets cheaper.
	if _, _, perr := a.pruneNow(cb); perr == nil {
		snap = a.session.Snapshot()
	}
	span, ok := compact.Choose(snap.Messages, window, compact.Default)
	if !ok {
		return before, before, errors.New(a.msg.CompactionNothing)
	}
	if cb.onStatus != nil {
		cb.onStatus(a.msg.CompactionRunning(span.Tokens))
	}

	resp, err := client.Chat(ctx, deepseek.ChatRequest{
		Model:    model,
		Messages: compact.SummaryPrompt(span.Messages),
	})
	if err != nil {
		return before, before, err
	}
	summary := ""
	if len(resp.Choices) > 0 {
		summary = strings.TrimSpace(resp.Choices[0].Message.Content)
	}
	if summary == "" {
		return before, before, errors.New(a.msg.CompactionEmpty)
	}

	// The span is addressed by log sequence, not by message index, so a
	// compaction still hides the right events after a reload.
	if err := a.session.AppendCompaction(summary, snap.Seqs[span.From], snap.Seqs[span.To], span.Tokens); err != nil {
		return before, before, err
	}
	after := tokens.Messages(a.session.Messages())
	return before, after, nil
}

// maybeCompact summarises the conversation when it approaches the window.
func (a *app) maybeCompact(ctx context.Context, client *deepseek.Client, model string, cb chatCallbacks) {
	if a.session == nil || !a.cfg.CompactEnabled() {
		return
	}
	window := a.contextWindow()
	if window <= 0 || !compact.Needed(a.pressure(), window, compact.Default) {
		return
	}
	if _, _, err := a.compactNow(ctx, client, model, cb); err != nil {
		if cb.onStatus != nil {
			cb.onStatus(a.msg.CompactionFailed + " " + err.Error())
		}
	}
}

// isContextOverflow reports whether an API error means the request was too big.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, needle := range []string{"context length", "context_length", "maximum context", "context window", "too many tokens", "prompt is too long"} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return strings.Contains(s, "400") && strings.Contains(s, "token")
}

// Markers the model recognises: they say which policy blocked the call, so it
// can react instead of retrying blindly.
const (
	markerFileDenied    = "[sandbox: file access denied under %s mode]"
	markerCommandDenied = "[sandbox: command execution denied under %s mode]"
	// markerPlanDenied tells the model what to do instead of just saying no.
	markerPlanDenied = "[plan mode: this tool is disabled until the plan is approved. Think it through and call exit_plan_mode with your plan.]"
)

// applySandbox pushes the current mode into the file tools.
func (a *app) applySandbox() {
	if a.fs != nil {
		a.fs.SetConfined(a.sandboxMode != "full")
	}
}

// setSandbox changes the session sandbox mode.
func (a *app) setSandbox(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "read-only", "readonly", "ro", "read":
		a.sandboxMode = "read-only"
	case "workspace-write", "workspace", "write":
		a.sandboxMode = "workspace-write"
	case "full", "danger-full-access", "unrestricted":
		a.sandboxMode = "full"
	default:
		return errors.New(a.msg.SandboxUsage)
	}
	a.applySandbox()
	return nil
}

// sandboxString reports the session sandbox mode.
func (a *app) sandboxString() string {
	state := a.msg.SandboxReadOnly
	switch a.sandboxMode {
	case "workspace-write":
		state = a.msg.SandboxWorkspace
	case "full":
		state = a.msg.SandboxFull
	}
	return a.msg.SandboxTitle + "\n  " + a.sandboxMode + ": " + state + "\n  " + a.msg.SandboxUsage
}

// sandboxDenial reports a marker when the current mode forbids the call.
//
// In read-only mode the shell is refused outright rather than filtered: this
// CLI cannot confine a process on every platform, and pretending otherwise
// would be worse than saying no.
func (a *app) sandboxDenial(tc deepseek.ToolCall) (string, bool) {
	if a.planMode {
		switch tc.Function.Name {
		case toolWrite, toolEdit, shellToolName:
			return markerPlanDenied, true
		}
	}
	if a.sandboxMode != "read-only" {
		return "", false
	}
	switch tc.Function.Name {
	case toolWrite, toolEdit:
		return fmt.Sprintf(markerFileDenied, a.sandboxMode), true
	case shellToolName:
		return fmt.Sprintf(markerCommandDenied, a.sandboxMode), true
	}
	return "", false
}

// callNeedsApproval reports whether a call must be confirmed.
//
// Reading is not a side effect, so it never asks. A destructive shell command
// always asks, even when the session runs without approvals.
func (a *app) callNeedsApproval(tc deepseek.ToolCall) bool {
	// Approving a plan is exactly the moment a human must look.
	if tc.Function.Name == toolPlan {
		return true
	}
	if a.approvalAuto {
		if tc.Function.Name != shellToolName {
			return false
		}
		_, dangerous := shell.Danger(shellCommandOf(tc.Function.Arguments))
		return dangerous
	}
	return a.toolNeedsApproval(tc.Function.Name)
}

// shellCommandOf extracts the command line from a run_shell call.
func shellCommandOf(args string) string {
	var a shellArgs
	_ = json.Unmarshal([]byte(args), &a)
	return a.Command
}

// recordAssistant writes a model answer to the session log.
func (a *app) recordAssistant(content string, calls []deepseek.ToolCall, usage *deepseek.Usage) error {
	if a.session == nil {
		return nil
	}
	return a.session.AppendAssistant(content, calls, usage)
}

// recordToolResult writes a tool outcome to the session log.
func (a *app) recordToolResult(callID, content string, isErr bool) error {
	if a.session == nil {
		return nil
	}
	return a.session.AppendToolResult(callID, content, isErr)
}

// appendAbortedResults records a result for tool calls that will not run, so a
// conversation interrupted mid-batch can still be replayed or resumed. Without
// it the assistant message keeps tool calls with no matching tool messages and
// the next request is rejected by the API.
func appendAbortedResults(messages []deepseek.Message, pending []deepseek.ToolCall, reason string) []deepseek.Message {
	for _, tc := range pending {
		messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: reason})
	}
	return messages
}

// projectedTokens estimates the cost of the next request: the last prompt plus
// the answer it produced, which the next request sends again.
func (a *app) projectedTokens() int {
	if a.lastUsage.TotalTokens == 0 {
		return 0
	}
	return a.lastUsage.PromptTokens + a.lastUsage.CompletionTokens
}

// addUsage accumulates the token accounting of one API round. A tool loop
// bills several rounds per user message, so the totals add up.
func (a *app) addUsage(u deepseek.Usage) {
	a.lastUsage = u
	a.tokensIn += u.PromptTokens
	a.tokensOut += u.CompletionTokens
}

// recordModelInfo remembers the context window of the model in use.
func (a *app) recordModelInfo(models []deepseek.Model, id string) {
	for _, m := range models {
		if m.ID == id {
			a.modelWindow = m.ContextWindow
			return
		}
	}
}

// tokensString renders the token accounting of the session.
func (a *app) tokensString() string {
	var b strings.Builder
	b.WriteString(a.msg.TokensTitle + "\n")

	effort := a.cfg.ReasoningEffort
	if effort == "" {
		effort = "high"
	}
	fmt.Fprintf(&b, "  %-16s %s (%s %s)\n", a.msg.TokensModel+":", currentModel(a.cfg, ""), a.msg.TokensEffort, effort)

	if a.lastUsage.TotalTokens == 0 && a.tokensIn == 0 && a.tokensOut == 0 {
		b.WriteString("  " + a.msg.TokensNone + "\n")
		return b.String()
	}

	fmt.Fprintf(&b, "  %-16s %s (↑ %s · ↓ %s)\n", a.msg.TokensLast+":",
		groupNumber(a.lastUsage.TotalTokens),
		groupNumber(a.lastUsage.PromptTokens),
		groupNumber(a.lastUsage.CompletionTokens))

	pressure := a.pressure()
	if pressure == 0 {
		pressure = a.lastUsage.PromptTokens
	}
	if window := a.contextWindow(); window > 0 {
		pct := pressure * 100 / window
		fmt.Fprintf(&b, "  %-16s ≈ %s / %s (%d %%)\n", a.msg.TokensContext+":",
			groupNumber(pressure), groupNumber(window), pct)
	} else {
		fmt.Fprintf(&b, "  %-16s ≈ %s\n", a.msg.TokensContext+":", groupNumber(pressure))
	}

	fmt.Fprintf(&b, "  %-16s ↑ %s · ↓ %s · %s %s\n", a.msg.TokensSession+":",
		groupNumber(a.tokensIn), groupNumber(a.tokensOut),
		a.msg.TokensTotal, groupNumber(a.tokensIn+a.tokensOut))

	if a.lastUsage.PromptCacheHitTokens > 0 {
		fmt.Fprintf(&b, "  %-16s %s\n", a.msg.TokensCache+":", groupNumber(a.lastUsage.PromptCacheHitTokens))
	}
	if a.proj != nil {
		if size := len(a.proj.Context()); size > 0 {
			fmt.Fprintf(&b, "  %-16s %s (≈ %s tokens, %s %s)\n", a.msg.TokensProject+":",
				attach.HumanBytes(size), estimateTokens(size),
				a.msg.ContextModeLabel, a.cfg.ContextMode())
		}
	}
	return b.String()
}

// balanceString queries the account balance.
func (a *app) balanceString(ctx context.Context) (string, error) {
	client, err := a.newClient()
	if err != nil {
		return "", err
	}
	res, err := client.Balance(ctx)
	if err != nil {
		return "", err
	}
	status := a.msg.BalanceOK
	if !res.IsAvailable {
		status = a.msg.BalanceUnavailable
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", a.msg.BalanceTitle, status)
	for _, info := range res.BalanceInfos {
		fmt.Fprintf(&b, "  %s  %s %s", info.Currency, a.msg.BalanceTotal, info.TotalBalance)
		if info.ToppedUpBalance != "" || info.GrantedBalance != "" {
			fmt.Fprintf(&b, "  (%s %s · %s %s)",
				a.msg.BalanceToppedUp, info.ToppedUpBalance,
				a.msg.BalanceGranted, info.GrantedBalance)
		}
		b.WriteString("\n")
	}
	if len(res.BalanceInfos) == 0 {
		b.WriteString("  " + a.msg.BalanceEmpty + "\n")
	}
	return b.String(), nil
}

// balanceCmd prints the account balance.
func (a *app) balanceCmd() error {
	s, err := a.balanceString(context.Background())
	if err != nil {
		return fmt.Errorf("%s %w", a.msg.BalanceFailed, err)
	}
	fmt.Print(s)
	return nil
}

// groupNumber formats an integer with thin grouping: 1234567 -> "1 234 567".
func groupNumber(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// compactTokens abbreviates a token count for the header: 12345 -> "12.3k".
func compactTokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1000000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

// pipeQuestion builds the question for --pipe from the arguments and stdin.
//
// stdin is only read when it is not a terminal: otherwise `deepseek --pipe
// "pregunta"` typed by hand would block forever waiting for input.
func (a *app) pipeQuestion(rest []string, stdin io.Reader, interactive bool) (string, error) {
	prompt := strings.TrimSpace(strings.Join(rest, " "))
	var piped string
	if !interactive && stdin != nil {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		piped = strings.TrimSpace(string(data))
	}
	switch {
	case prompt == "" && piped == "":
		return "", errors.New(a.msg.PipeUsage)
	case piped == "":
		return prompt, nil
	case prompt == "":
		return piped, nil
	}
	return prompt + "\n\n" + a.msg.PipeStdinLabel(attach.HumanBytes(len(piped))) + "\n\n" + piped, nil
}

// jsonEvent is one line of the NDJSON stream produced by --json. Every field is
// optional so each event type stays small.
type jsonEvent struct {
	Type             string `json:"type"` // start, delta, usage, done, error
	Model            string `json:"model,omitempty"`
	Text             string `json:"text,omitempty"`
	Content          string `json:"content,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	Message          string `json:"message,omitempty"`
}

// writeJSONEvent writes one NDJSON line to stdout. A write failure is ignored:
// the caller is usually a pipe that may have been closed early.
func writeJSONEvent(ev jsonEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	os.Stdout.Write(append(data, '\n'))
}

func (a *app) ask(question string) error {
	if question == "" {
		return errors.New(a.msg.EmptyQuestion)
	}
	// The chat resolves "/skill" and "/mcp-tool" references; a one-shot ask must
	// behave the same way, or the same prompt means different things depending
	// on how it was launched.
	if ref, ok := a.skillOrToolRef(question); ok {
		question = ref.Payload
	}
	client, err := a.newClient()
	if err != nil {
		return err
	}
	model, err := a.ensureModel(client)
	if err != nil {
		return err
	}
	a.loadProject()
	a.connectMCP()
	if a.mcp != nil {
		defer a.mcp.Close()
	}
	um := a.resolveUserMessage(question)
	if um.Note != "" {
		// stderr keeps stdout clean for pipelines.
		fmt.Fprintln(os.Stderr, um.Note)
	}
	ctx := context.Background()
	messages := []deepseek.Message{
		{Role: "system", Content: a.systemPrompt()},
		{Role: "user", Content: um.Payload},
	}
	if a.json {
		writeJSONEvent(jsonEvent{Type: "start", Model: model})
	}
	content, _, err := a.chatTurn(ctx, client, model, messages, chatCallbacks{
		onChunk: func(delta string) {
			if a.json {
				writeJSONEvent(jsonEvent{Type: "delta", Text: delta})
				return
			}
			fmt.Print(delta)
		},
		onStatus: func(s string) {
			fmt.Fprintln(os.Stderr, s)
		},
		onUsage: a.addUsage,
		askPerm: a.confirmTool,
	})
	if err != nil {
		return err
	}
	if a.json {
		if a.lastUsage.TotalTokens > 0 {
			writeJSONEvent(jsonEvent{
				Type:             "usage",
				PromptTokens:     a.lastUsage.PromptTokens,
				CompletionTokens: a.lastUsage.CompletionTokens,
				TotalTokens:      a.lastUsage.TotalTokens,
			})
		}
		writeJSONEvent(jsonEvent{Type: "done", Content: content})
		return nil
	}
	fmt.Println()
	if a.lastUsage.TotalTokens > 0 {
		// stderr, so `--pipe` output stays usable in a pipeline.
		fmt.Fprintf(os.Stderr, "tokens: ↑%s ↓%s\n",
			groupNumber(a.lastUsage.PromptTokens), groupNumber(a.lastUsage.CompletionTokens))
	}
	return nil
}

// maxToolRounds bounds the tool-calling loop to prevent infinite loops.
const maxToolRounds = 10

// chatCallbacks wires the chat loop to the UI (terminal or TUI).
type chatCallbacks struct {
	onChunk     func(string)                          // streamed content
	onReasoning func(string)                          // reasoning content (thinking models)
	onStatus    func(string)                          // status lines (tool running, denied...)
	onUsage     func(deepseek.Usage)                  // token accounting of one API round
	askPerm     func(deepseek.ToolCall) (bool, error) // ask the user for tool permission
}

// chatTurn runs the model until it stops requesting tools. Every tool call
// is confirmed with the user before execution. It returns the final content
// and the full message history (including tool calls and results).
func (a *app) chatTurn(ctx context.Context, client *deepseek.Client, model string, messages []deepseek.Message, cb chatCallbacks) (string, []deepseek.Message, error) {
	recovered := false
	for round := 0; round < maxToolRounds; round++ {
		if a.session != nil {
			// The log is the source of truth: rebuild the request every round so
			// a compaction, or a turn resumed from disk, is reflected at once.
			a.maybeCompact(ctx, client, model, cb)
			messages = a.conversationMessages()
		}
		req := deepseek.ChatRequest{Model: model, Messages: messages}
		if a.cfg.Thinking != "" {
			req.Thinking = map[string]any{"type": a.cfg.Thinking}
		}
		if a.cfg.ReasoningEffort != "" {
			req.ReasoningEffort = a.cfg.ReasoningEffort
		}
		if tools := a.allTools(); len(tools) > 0 {
			req.Tools = tools
		}
		result, err := client.StreamChat(ctx, req, cb.onChunk, cb.onReasoning)
		if err != nil {
			// A request that overflows the window is recovered once by
			// compacting and retrying the same step.
			if a.session != nil && !recovered && isContextOverflow(err) {
				recovered = true
				if _, _, cerr := a.compactNow(ctx, client, model, cb); cerr == nil {
					continue
				}
			}
			return "", messages, err
		}
		if result.Usage != nil && cb.onUsage != nil {
			// The caller owns the accounting: under the TUI this callback runs
			// on the chat goroutine, so it must not touch shared state.
			cb.onUsage(*result.Usage)
		}
		if len(result.ToolCalls) == 0 {
			messages = append(messages, deepseek.Message{Role: "assistant", Content: result.Content})
			if err := a.recordAssistant(result.Content, nil, result.Usage); err != nil {
				return "", messages, err
			}
			return result.Content, messages, nil
		}

		messages = append(messages, deepseek.Message{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
		if err := a.recordAssistant(result.Content, result.ToolCalls, result.Usage); err != nil {
			return "", messages, err
		}
		for idx, tc := range result.ToolCalls {
			// A policy denial never reaches the user: it is the mode saying no.
			if denial, blocked := a.sandboxDenial(tc); blocked {
				cb.onStatus(a.msg.ToolBlocked(tc.Function.Name, a.sandboxMode))
				messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: denial})
				if err := a.recordToolResult(tc.ID, denial, true); err != nil {
					return "", messages, err
				}
				continue
			}
			// Reading is not a side effect, so those calls skip the prompt.
			allowed := true
			if a.callNeedsApproval(tc) {
				var permErr error
				allowed, permErr = cb.askPerm(tc)
				if permErr != nil {
					// Leave the history valid: an assistant message with tool calls
					// must be followed by one tool message per call, or the next
					// request is rejected. Record the rest as not executed.
					rest := result.ToolCalls[idx:]
					messages = appendAbortedResults(messages, rest, a.msg.ToolNotStarted)
					for _, pending := range rest {
						_ = a.recordToolResult(pending.ID, a.msg.ToolNotStarted, true)
					}
					return "", messages, permErr
				}
			}
			if !allowed {
				cb.onStatus(a.msg.ToolDenied(tc.Function.Name))
				messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: a.msg.ToolDeniedContent})
				if err := a.recordToolResult(tc.ID, a.msg.ToolDeniedContent, true); err != nil {
					return "", messages, err
				}
				continue
			}
			cb.onStatus(a.msg.ToolRunning(tc.Function.Name))
			output, err := a.executeTool(ctx, tc)
			if err != nil {
				failed := "Error: " + err.Error()
				messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: failed})
				if recErr := a.recordToolResult(tc.ID, failed, true); recErr != nil {
					return "", messages, recErr
				}
				continue
			}
			messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: output})
			if err := a.recordToolResult(tc.ID, output, false); err != nil {
				return "", messages, err
			}
		}
	}
	return "", messages, errors.New(a.msg.ToolLoopLimit)
}

// permissions remembers which tools the user denied for the rest of the
// session. There is deliberately no "allow" side: allowing everything is a
// policy change (app.approvalAuto), not a per-tool grant.
type permissions struct {
	deny map[string]bool
}

// confirmTool asks the user for permission before executing a tool call.
// When the user cannot be asked (non-interactive), the call is denied.
func (a *app) confirmTool(tc deepseek.ToolCall) (bool, error) {
	if a.approvalAuto {
		return true, nil
	}
	if a.perm.deny[tc.Function.Name] {
		return false, nil
	}
	if !stdinInteractive() {
		return false, nil // deny by default when we cannot ask
	}
	fmt.Printf("\n%s\n", a.msg.ToolRequest(tc.Function.Name, toolDetail(tc)))
	if tc.Function.Name == shellToolName {
		// "auto" is a much bigger deal for the shell than for a read-only MCP
		// tool, so say it out loud.
		fmt.Println(a.msg.ShellAlwaysWarning)
	}
	fmt.Print(a.msg.ToolPrompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		// No answer available (EOF on a non-tty that still looks interactive):
		// deny this call, but let the conversation continue. Aborting the whole
		// turn because a prompt could not be answered loses the model's answer.
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "a", "auto":
		// Session policy, not a per-call grant: from here on nothing is asked.
		a.approvalAuto = true
		fmt.Fprintln(os.Stderr, a.msg.ApprovalAutoWarning)
		return true, nil
	case "d", "deny":
		a.perm.deny[tc.Function.Name] = true
		return false, nil
	default:
		return false, nil
	}
}

// shellToolName is the name the model uses to call the native shell.
const shellToolName = "run_shell"

// shellArgs is the JSON the model sends to run_shell.
type shellArgs struct {
	Command        string `json:"command"`
	Cwd            string `json:"cwd"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	// Background starts the command as a job and returns at once; the result is
	// delivered as an injected message when it finishes.
	Background bool `json:"background"`
}

// shellTool describes the native shell tool to the API.
func (a *app) shellTool() deepseek.Tool {
	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        shellToolName,
			Description: fmt.Sprintf(a.msg.ShellToolDescription, shell.Name()),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": a.msg.ShellParamCommand,
					},
					"cwd": map[string]any{
						"type":        "string",
						"description": a.msg.ShellParamDir,
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": a.msg.ShellParamTimeout,
					},
					"background": map[string]any{
						"type":        "boolean",
						"description": a.msg.ShellParamBackground,
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

// allTools returns every tool offered to the model: the native shell (unless
// disabled) plus the tools of the connected MCP servers.
func (a *app) allTools() []deepseek.Tool {
	var tools []deepseek.Tool
	if a.cfg.ShellEnabled() {
		tools = append(tools, a.shellTool())
	}
	tools = append(tools, a.fileTools()...)
	tools = append(tools, a.todoTool())
	tools = append(tools, a.questionTool())
	tools = append(tools, a.searchTool())
	if a.planMode {
		tools = append(tools, a.planTool())
	}
	return append(tools, a.deepSeekTools()...)
}

// shellTimeout returns the configured command timeout.
func (a *app) shellTimeout() time.Duration {
	if a.cfg.ShellTimeout > 0 {
		return time.Duration(a.cfg.ShellTimeout) * time.Second
	}
	return shell.DefaultTimeout
}

// runShellCommand executes a command in the native shell.
func (a *app) runShellCommand(ctx context.Context, cmdline, dir string, timeout time.Duration) (shell.Result, error) {
	if strings.TrimSpace(cmdline) == "" {
		return shell.Result{}, errors.New(a.msg.ShellEmptyCommand)
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return shell.Run(ctx, shell.Options{Command: cmdline, Dir: dir, Timeout: timeout})
}

// runShellTool executes the run_shell tool requested by the model and returns
// the report that goes back to it.
func (a *app) runShellTool(ctx context.Context, rawArgs string) (string, error) {
	if !a.cfg.ShellEnabled() {
		return "", errors.New(a.msg.ShellDisabled)
	}
	var args shellArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}
	if args.Background {
		dir := args.Cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}
		id := a.jobs.Start(args.Command, dir, a.shellTimeout())
		return fmt.Sprintf("Job %d started in the background: %s\nYou will receive a message when it finishes; continue with other work meanwhile.", id, args.Command), nil
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = a.shellTimeout()
	}
	res, err := a.runShellCommand(ctx, args.Command, args.Cwd, timeout)
	if err != nil {
		return "", err
	}
	return res.Summary(), nil
}

// toolDetail renders the arguments of a tool call for the permission prompt.
// The shell shows the command itself and the file tools show the change, so the
// user approves an action rather than a JSON blob.
func toolDetail(tc deepseek.ToolCall) string {
	if preview, ok := fileToolPreview(tc.Function.Name, tc.Function.Arguments); ok {
		return preview
	}
	if tc.Function.Name == shellToolName {
		var args shellArgs
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil && args.Command != "" {
			detail := "$ " + args.Command
			if args.Cwd != "" {
				detail += "\n(cwd: " + args.Cwd + ")"
			}
			return detail
		}
	}
	return tc.Function.Arguments
}

// executeTool runs a tool call: built-in file tools first, then the shell, then
// the connected MCP servers.
func (a *app) executeTool(ctx context.Context, tc deepseek.ToolCall) (string, error) {
	if out, handled, err := a.executeFileTool(tc); handled {
		return out, err
	}
	if out, handled, err := a.executeTodoTool(tc); handled {
		return out, err
	}
	if out, handled, err := a.executePlanTool(tc); handled {
		return out, err
	}
	if out, handled, err := a.executeQuestionTool(tc); handled {
		return out, err
	}
	if out, handled, err := a.executeSearchTool(tc); handled {
		return out, err
	}
	if tc.Function.Name == shellToolName {
		return a.runShellTool(ctx, tc.Function.Arguments)
	}
	if a.mcp == nil {
		return "", errors.New(a.msg.NoMCP)
	}
	srv, ok := a.mcp.FindTool(tc.Function.Name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", tc.Function.Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}
	return srv.CallTool(ctx, tc.Function.Name, args)
}

// deepSeekTools converts the connected MCP tools to the API tool format.
func (a *app) deepSeekTools() []deepseek.Tool {
	if a.mcp == nil {
		return nil
	}
	var out []deepseek.Tool
	for _, srv := range a.mcp.ConnectedServers() {
		for _, t := range srv.Tools() {
			schemaJSON, err := json.Marshal(t.InputSchema)
			if err != nil {
				continue
			}
			var params map[string]any
			if err := json.Unmarshal(schemaJSON, &params); err != nil {
				continue
			}
			out = append(out, deepseek.Tool{
				Type: "function",
				Function: deepseek.ToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			})
		}
	}
	return out
}

// connectMCP loads mcp.json and connects to the configured servers.
func (a *app) connectMCP() {
	cfg, err := mcpx.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, a.msg.MCPError, err)
		return
	}
	if len(cfg.Servers) == 0 {
		return
	}
	mgr, err := mcpx.Connect(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, a.msg.MCPError, err)
		return
	}
	a.mcp = mgr
	for _, s := range mgr.Servers() {
		if s.ConnectErr() != nil {
			fmt.Fprint(os.Stderr, a.msg.MCPFailed(s.Name, s.ConnectErr()))
		} else {
			fmt.Fprint(os.Stderr, a.msg.MCPConnected(s.Name, len(s.Tools())))
		}
	}
}

func (a *app) mcpCmd() error {
	cfg, err := mcpx.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Servers) == 0 {
		fmt.Println(a.msg.NoMCPServers)
		return nil
	}
	mgr, err := mcpx.Connect(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()
	fmt.Println(mcpStatusText(mgr, a.msg))
	return nil
}

func (a *app) showMCPStatus() error {
	fmt.Println(a.mcpStatusString())
	return nil
}

// mcpStatusString returns the MCP server status as text.
func (a *app) mcpStatusString() string {
	if a.mcp == nil {
		return a.msg.NoMCPServers
	}
	return mcpStatusText(a.mcp, a.msg)
}

// mcpStatusText renders the status of the given MCP manager.
func mcpStatusText(mgr *mcpx.Manager, msg *i18n.Messages) string {
	var b strings.Builder
	for _, s := range mgr.Servers() {
		if s.ConnectErr() != nil {
			fmt.Fprintf(&b, "%s: %v\n", s.Name, s.ConnectErr())
			continue
		}
		fmt.Fprintf(&b, "%s (%d tools):\n", s.Name, len(s.Tools()))
		for _, t := range s.Tools() {
			fmt.Fprintf(&b, "  - %s: %s\n", t.Name, t.Description)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *app) repl() error {
	client, err := a.newClient()
	if err != nil {
		return err
	}
	model, err := a.ensureModel(client)
	if err != nil {
		return err
	}
	a.loadProject()
	a.connectMCP()
	if a.mcp != nil {
		defer a.mcp.Close()
	}
	if err := a.openSession(model); err != nil {
		return err
	}
	if a.session != nil {
		defer a.session.Close()
	}

	if isInteractive() {
		return a.replTUI(client, model)
	}
	return a.replLine(client, model)
}

// openSession creates a session, or resumes one when the user asked for it.
func (a *app) openSession(model string) error {
	dir := session.Dir()
	switch {
	case a.resumeID != "":
		s, err := session.Open(dir, a.resumeID)
		if err != nil {
			return fmt.Errorf("%s %w", a.msg.SessionOpenFailed, err)
		}
		a.session = s
	case a.continueLast:
		id, err := session.Latest(dir)
		if err != nil {
			if !errors.Is(err, session.ErrNotFound) {
				return err
			}
			// Nothing to continue: start a fresh one.
			break
		}
		s, err := session.Open(dir, id)
		if err != nil {
			return fmt.Errorf("%s %w", a.msg.SessionOpenFailed, err)
		}
		a.session = s
	}
	if a.session != nil {
		fmt.Fprintln(os.Stderr, a.msg.SessionResumed(a.session.ID()))
		return nil
	}
	cwd, _ := os.Getwd()
	s, err := session.Create(dir, session.Meta{CWD: cwd, Model: model})
	if err != nil {
		// A session that cannot be written must not stop the chat; it only
		// costs durability.
		fmt.Fprintln(os.Stderr, a.msg.HistoryDisabledAt(dir, err))
		return nil
	}
	a.session = s
	return nil
}

// conversationMessages rebuilds the request from the log: the session is the
// source of truth, so a resumed session and a live one take the same path.
func (a *app) conversationMessages() []deepseek.Message {
	if a.session == nil {
		return a.refreshSystemPrompt(nil)
	}
	return a.refreshSystemPrompt(a.session.Messages())
}

// replLine is the fallback line-based REPL used when stdin is not a terminal.
func (a *app) replLine(client *deepseek.Client, model string) error {
	ctx := context.Background()
	sessionModel := model

	fmt.Println(a.msg.Welcome)
	scanner := bufio.NewScanner(os.Stdin)
	interactive := true
	for {
		for _, note := range a.injectJobResults() {
			fmt.Println(note)
		}
		fmt.Println()
		var line string
		var err error
		if interactive {
			line, err = editor.ReadMultiline("> ", a.completer())
			if errors.Is(err, io.EOF) {
				break // Ctrl+C / Ctrl+D
			}
			if err != nil {
				interactive = false // fall back to simple line input
				fmt.Fprintln(os.Stderr, a.msg.EditorFallback, err)
				continue
			}
		} else {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}
			line = scanner.Text()
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" || line == "/exit" {
			break
		}
		// um carries what the user sees and what the model receives. A skill
		// reference fills both: the marker and the typed text are shown, the
		// instructions travel only in the payload.
		var um userMessage
		refreshed := false
		if strings.HasPrefix(line, "/") {
			handled, err := a.slashCmd(client, line, &sessionModel)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			if handled {
				continue
			}
			// Not a known command: try a skill or MCP tool reference.
			if ref, ok := a.skillOrToolRef(line); ok {
				um = ref
				refreshed = true
			} else {
				fmt.Fprintln(os.Stderr, "error:", a.msg.UnknownSlashCmd(line))
				continue
			}
		}
		if !refreshed {
			um = a.resolveUserMessage(line)
		}
		if um.Note != "" {
			fmt.Println(um.Note)
		}
		if a.session != nil {
			if err := a.session.AppendUser(um.Payload); err != nil {
				fmt.Fprintln(os.Stderr, a.msg.SaveFailed, err)
			}
		}
		messages := a.conversationMessages()
		fmt.Println()
		reply, _, err := a.chatTurn(ctx, client, sessionModel, messages, chatCallbacks{
			onChunk: func(delta string) {
				fmt.Print(delta)
			},
			onStatus: func(s string) {
				fmt.Fprintln(os.Stderr, s)
			},
			onUsage: a.addUsage,
			askPerm: a.confirmTool,
		})
		fmt.Println()
		if err != nil {
			return err
		}
		if a.session != nil {
			if err := a.session.AppendTurnEnd("completed"); err != nil {
				fmt.Fprintln(os.Stderr, a.msg.SaveFailed, err)
			}
		}
		a.lastReply = reply
		fmt.Fprintf(&a.transcript, "▸ %s\n%s\n\n◂ %s\n%s\n\n", a.msg.UserLabel, um.Display, a.msg.AssistantLabel, reply)
	}
	return scanner.Err()
}

// slashCmd handles session commands inside the interactive chat. It returns
// false when the line is not a known command (so the caller can try a skill
// or MCP tool reference).
func (a *app) slashCmd(client *deepseek.Client, line string, sessionModel *string) (bool, error) {
	switch {
	case line == "/list-models":
		return true, a.printModels(client, currentModel(a.cfg, *sessionModel))
	case line == "/models" || line == "/model":
		return true, a.pickModel(client, sessionModel)
	case strings.HasPrefix(line, "/models ") || strings.HasPrefix(line, "/model "):
		id := cmdArg(line)
		if id == "reset" {
			a.cfg.Model = ""
			if err := a.cfg.Save(); err != nil {
				return true, err
			}
			return true, a.pickModel(client, sessionModel)
		}
		if id == "" {
			return true, errors.New(a.msg.ModelUsage)
		}
		// Warn (but allow) when the model is not in the API list: the user may
		// be using a dated snapshot that is still valid.
		if models, err := client.ListModels(context.Background()); err == nil {
			if !modelAvailable(models, id) {
				fmt.Fprintln(os.Stderr, a.msg.ModelNotListed(id))
			}
		}
		*sessionModel = id
		if _, err := a.saveDefaultModel(id); err != nil {
			return true, err
		}
		return true, nil
	case line == "/approvals":
		fmt.Println(a.approvalsString())
		return true, nil
	case strings.HasPrefix(line, "/approvals "):
		return true, a.setApprovals(cmdArg(line))
	case line == "/login":
		return true, a.login()
	case strings.HasPrefix(line, "/login "):
		return true, a.loginWithKey(cmdArg(line))
	case line == "/thinking" || strings.HasPrefix(line, "/thinking "):
		return true, a.setThinking(cmdArg(line))
	case line == "/effort" || strings.HasPrefix(line, "/effort "):
		return true, a.setEffort(cmdArg(line))
	case line == "/mcp":
		return true, a.showMCPStatus()
	case line == "/shell":
		return true, a.showShell()
	case line == "/run" || strings.HasPrefix(line, "/run "):
		return true, a.runManually(cmdArg(line))
	case line == "/plan":
		a.planMode = !a.planMode
		fmt.Println(a.planString())
		return true, nil
	case line == "/jobs":
		fmt.Println(a.jobsString())
		return true, nil
	case line == "/todos":
		fmt.Println(a.todosString())
		return true, nil
	case line == "/sandbox":
		fmt.Println(a.sandboxString())
		return true, nil
	case strings.HasPrefix(line, "/sandbox "):
		if err := a.setSandbox(strings.TrimSpace(line[len("/sandbox "):])); err != nil {
			return true, err
		}
		fmt.Println(a.sandboxString())
		return true, nil
	case line == "/tokens":
		fmt.Print(a.tokensString())
		return true, nil
	case line == "/compact":
		before, after, cerr := a.compactNow(context.Background(), client, *sessionModel, chatCallbacks{
			onStatus: func(s string) { fmt.Fprintln(os.Stderr, s) },
		})
		if cerr != nil {
			return true, cerr
		}
		fmt.Println(a.msg.CompactionDone(before, after))
		return true, nil
	case line == "/balance":
		s, err := a.balanceString(context.Background())
		if err != nil {
			return true, fmt.Errorf("%s %w", a.msg.BalanceFailed, err)
		}
		fmt.Print(s)
		return true, nil
	case line == "/context":
		return true, a.showContext()
	case line == "/context reload":
		a.loadProject()
		fmt.Println(a.msg.ContextReloaded)
		return true, a.showContext()
	case strings.HasPrefix(line, "/context"):
		return true, errors.New(a.msg.ContextUsage)
	case line == "/copy":
		return true, a.copyLastReply()
	case line == "/copy all" || line == "/copy todo":
		return true, a.copyTranscript()
	case line == "/save" || strings.HasPrefix(line, "/save "):
		path, err := saveTranscriptFile(cmdArg(line), a.transcript.String())
		if err != nil {
			return true, fmt.Errorf("%s %w", a.msg.SaveTranscriptFailed, err)
		}
		fmt.Println(a.msg.TranscriptSaved(path))
		return true, nil
	case line == "/help" || line == "/help all":
		fmt.Print(a.helpCatalog())
		return true, nil
	case strings.HasPrefix(line, "/help "):
		topic := cmdArg(line)
		text, ok := a.helpTopic(topic)
		if !ok {
			return true, errors.New(a.msg.HelpUnknownTopic(topic))
		}
		fmt.Println(text)
		return true, nil
	default:
		return false, nil
	}
}

// copyLastReply copies the last assistant answer to the system clipboard.
func (a *app) copyLastReply() error {
	if strings.TrimSpace(a.lastReply) == "" {
		return clipboard.ErrEmpty
	}
	if err := clipboard.Copy(a.lastReply); err != nil {
		return fmt.Errorf("%s %w", a.msg.CopyFailed, err)
	}
	fmt.Println(a.msg.Copied(len([]rune(a.lastReply))))
	return nil
}

// copyTranscript copies the whole session to the system clipboard.
func (a *app) copyTranscript() error {
	text := a.transcript.String()
	if strings.TrimSpace(text) == "" {
		return clipboard.ErrEmpty
	}
	if err := clipboard.Copy(text); err != nil {
		return fmt.Errorf("%s %w", a.msg.CopyFailed, err)
	}
	fmt.Println(a.msg.Copied(len([]rune(text))))
	return nil
}

// saveTranscriptFile writes text to path, or to a timestamped Markdown file in
// the working directory when path is empty. It returns the absolute path.
func saveTranscriptFile(path, text string) (string, error) {
	if path == "" {
		path = fmt.Sprintf("deepseek-chat-%s.md", time.Now().Format("20060102-150405"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if err := os.WriteFile(abs, []byte(text), 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

// shellString describes the native shell and the shell tool settings.
func (a *app) shellString() string {
	var b strings.Builder
	fmt.Fprintln(&b, fmt.Sprintf(a.msg.ShellDetected, shell.Name(), shell.Detect()))
	state := a.msg.EnvToolOn
	if !a.cfg.ShellEnabled() {
		state = a.msg.EnvToolOff
	}
	fmt.Fprintf(&b, "%s: %s\n", a.msg.EnvTool, state)
	fmt.Fprintf(&b, "%s: %s\n", a.msg.ShellTimeoutLabel, a.shellTimeout().Round(time.Second))
	fmt.Fprint(&b, a.msg.RunUsage)
	return b.String()
}

// showShell reports the native shell and the shell tool settings.
func (a *app) showShell() error {
	fmt.Println(a.shellString())
	return nil
}

// renderRunResult formats a /run execution for the TUI.
func (a *app) renderRunResult(cmdline string, res shell.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", cmdline)
	out := strings.TrimRight(res.Output, " \t\r\n")
	if out == "" {
		out = a.msg.ShellNoOutput
	}
	b.WriteString(out)
	switch {
	case res.TimedOut:
		b.WriteString("\n" + a.msg.ShellTimedOutNotice(res.Duration.Round(time.Second)))
	case res.ExitCode != 0:
		b.WriteString("\n" + a.msg.ShellExitCode(res.ExitCode))
	}
	return b.String()
}

// approvalsString reports the session approval policy.
func (a *app) approvalsString() string {
	state := a.msg.ApprovalStateAsk
	if a.approvalAuto {
		state = a.msg.ApprovalStateAuto
	}
	var b strings.Builder
	b.WriteString(a.msg.ApprovalsTitle + "\n")
	b.WriteString("  " + state + "\n")
	blocked := make([]string, 0, len(a.perm.deny))
	for name := range a.perm.deny {
		blocked = append(blocked, name)
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		b.WriteString("  " + a.msg.ApprovalsBlocked + ": " + strings.Join(blocked, ", ") + "\n")
	}
	b.WriteString("  " + a.msg.ApprovalUsage)
	return b.String()
}

// setApprovals changes the session approval policy.
func (a *app) setApprovals(v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ask", "preguntar", "on":
		a.approvalAuto = false
	case "auto", "off":
		a.approvalAuto = true
	default:
		return errors.New(a.msg.ApprovalUsage)
	}
	return nil
}

// runManually runs a command the user typed with /run. It needs no permission:
// the user is the one asking for it.
func (a *app) runManually(cmdline string) error {
	if strings.TrimSpace(cmdline) == "" {
		return errors.New(a.msg.RunUsage)
	}
	res, err := a.runShellCommand(context.Background(), cmdline, "", a.shellTimeout())
	if err != nil {
		return err
	}
	fmt.Print(res.Output)
	if res.TimedOut {
		fmt.Fprintln(os.Stderr, a.msg.ShellTimedOutNotice(res.Duration.Round(time.Second)))
	} else if res.ExitCode != 0 {
		fmt.Fprintln(os.Stderr, a.msg.ShellExitCode(res.ExitCode))
	}
	return nil
}

// pickModel shows the interactive model selection and applies the choice.
func (a *app) pickModel(client *deepseek.Client, sessionModel *string) error {
	models, err := client.ListModels(context.Background())
	if err != nil {
		return err
	}
	m, err := a.promptModelSelection(models, currentModel(a.cfg, *sessionModel))
	if err != nil {
		return err
	}
	*sessionModel = m
	return nil
}

// setThinking shows or sets the thinking mode (enabled/disabled).
func (a *app) setThinking(v string) error {
	if v == "" {
		fmt.Printf("thinking: %s\n", display(a.cfg.Thinking, "enabled", a.msg.Default))
		return nil
	}
	v = strings.ToLower(v)
	if v == "on" {
		v = "enabled"
	}
	if v == "off" {
		v = "disabled"
	}
	if v != "enabled" && v != "disabled" {
		return errors.New(a.msg.InvalidThinkingValue)
	}
	a.cfg.Thinking = v
	if err := a.cfg.Save(); err != nil {
		return err
	}
	fmt.Println(a.msg.Saved)
	return nil
}

// setEffort shows or sets the reasoning effort (low/high/max).
func (a *app) setEffort(v string) error {
	if v == "" {
		fmt.Printf("effort: %s\n", display(a.cfg.ReasoningEffort, "high", a.msg.Default))
		return nil
	}
	v = strings.ToLower(v)
	if v != "low" && v != "high" && v != "max" {
		return errors.New(a.msg.InvalidEffortValue)
	}
	a.cfg.ReasoningEffort = v
	if err := a.cfg.Save(); err != nil {
		return err
	}
	fmt.Println(a.msg.Saved)
	return nil
}

// login prompts for the API key (hidden) and stores it in the keyring.
func (a *app) login() error {
	key, err := a.promptAPIKey()
	if err != nil {
		return err
	}
	if err := config.SetAPIKey(a.cfg, key); err != nil {
		return err
	}
	fmt.Println(a.msg.LoginOK)
	return nil
}

// loginWithKey stores the given API key.
func (a *app) loginWithKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New(a.msg.NoAPIKey)
	}
	if err := config.SetAPIKey(a.cfg, key); err != nil {
		return err
	}
	fmt.Println(a.msg.LoginOK)
	return nil
}

// skillOrToolRef converts a line like "/skill-name rest" or "/tool-name rest"
// into a user message that references the skill or MCP tool.
//
// Display and Payload differ on purpose: a skill's instructions are for the
// model, not for the transcript. Showing them would fill the user's own message
// with a wall of text they never typed.
func (a *app) skillOrToolRef(line string) (userMessage, bool) {
	parts := strings.SplitN(line, " ", 2)
	name := strings.TrimPrefix(parts[0], "/")
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}
	for _, s := range a.projectSkills() {
		if s.Name != name {
			continue
		}
		display := a.msg.SkillRef(s.Name)
		payload := display
		// Send the skill's instructions, not just its name: a reference that
		// only names the skill leaves the model guessing what it does.
		if body, err := project.ReadSkillBody(s.Path); err == nil {
			payload += "\n\n" + body
		}
		if rest != "" {
			display += "\n\n" + rest
			payload += "\n\n" + rest
		}
		return userMessage{Display: display, Payload: payload}, true
	}
	for _, t := range a.mcpToolNames() {
		if t == name {
			msg := a.msg.ToolRef(t)
			if rest != "" {
				msg += " " + rest
			}
			return userMessage{Display: msg, Payload: msg}, true
		}
	}
	return userMessage{}, false
}

// completer builds the autocompletion for the editor: @ triggers file paths,
// / triggers commands, skills and MCP tools.
func (a *app) completer() editor.Completer {
	return func(line string, pos int) editor.Completion {
		before := line[:pos]
		trig, idx := lastTrigger(before)
		if idx < 0 {
			return editor.Completion{}
		}
		token := before[idx+1:]
		if trig == '@' {
			return fileCompletion(token)
		}
		return a.commandCompletion(token)
	}
}

// lastTrigger finds the last '@' or '/' that starts a token (preceded by a
// space, newline or the start of the line).
func lastTrigger(before string) (byte, int) {
	for i := len(before) - 1; i >= 0; i-- {
		c := before[i]
		if (c == '@' || c == '/') && (i == 0 || before[i-1] == ' ' || before[i-1] == '\n' || before[i-1] == '\t') {
			return c, i
		}
	}
	return 0, -1
}

// fileCompletion completes a file path token (after '@').
func fileCompletion(token string) editor.Completion {
	dir := filepath.Dir(token)
	base := filepath.Base(token)
	if len(token) > 0 && os.IsPathSeparator(token[len(token)-1]) {
		base = "" // token ends with a separator: list everything inside
	}
	entries, err := os.ReadDir(dir) // "." reads the current directory
	if err != nil {
		return editor.Completion{}
	}
	var candidates []string
	for _, e := range entries {
		// File names are case-insensitive on Windows and macOS, so typing
		// "@rea" has to find README.md. On Linux this only widens the match;
		// the candidate keeps its real case.
		if !hasPrefixFold(e.Name(), base) {
			continue
		}
		cand := e.Name()
		if e.IsDir() {
			cand += string(filepath.Separator)
		}
		path := filepath.Join(dir, cand)
		// Quote paths with spaces so the reference stays a single token, the
		// same form the attachment resolver understands.
		if strings.ContainsAny(path, " \t") {
			path = `"` + path + `"`
		}
		candidates = append(candidates, "@"+path)
	}
	sort.Strings(candidates)
	return editor.Completion{Prefix: "@" + token, Candidates: candidates}
}

// hasPrefixFold reports whether s starts with prefix, ignoring case.
//
// Byte slicing would cut a multi-byte rune in half, so it compares lowercased
// strings instead.
func hasPrefixFold(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// commandCompletion completes a command token (after '/') with commands,
// their subcommands, skills and MCP tools. "/help <topic>" completes the
// topics the help catalog knows about.
func (a *app) commandCompletion(token string) editor.Completion {
	if rest, ok := strings.CutPrefix(token, "help "); ok {
		var topics []string
		for _, t := range a.helpTopics() {
			if strings.HasPrefix(t, rest) {
				topics = append(topics, "/help "+t)
			}
		}
		sort.Strings(topics)
		return editor.Completion{Prefix: "/" + token, Candidates: topics}
	}

	var candidates []string
	for _, c := range commandCompletions() {
		if strings.HasPrefix(c, token) {
			candidates = append(candidates, "/"+c)
		}
	}
	for _, s := range a.skillNames() {
		if strings.HasPrefix(s, token) {
			candidates = append(candidates, "/"+s)
		}
	}
	for _, t := range a.mcpToolNames() {
		if strings.HasPrefix(t, token) {
			candidates = append(candidates, "/"+t)
		}
	}
	sort.Strings(candidates)
	return editor.Completion{Prefix: "/" + token, Candidates: candidates}
}

func (a *app) skillNames() []string {
	if a.proj == nil {
		return nil
	}
	var out []string
	for _, s := range a.proj.Skills {
		out = append(out, s.Name)
	}
	return out
}

func (a *app) mcpToolNames() []string {
	if a.mcp == nil {
		return nil
	}
	var out []string
	for _, srv := range a.mcp.ConnectedServers() {
		for _, t := range srv.Tools() {
			out = append(out, t.Name)
		}
	}
	return out
}

// cmdArg returns the argument after the command word, trimmed.
func cmdArg(line string) string {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (a *app) showContext() error {
	fmt.Println(a.contextString())
	return nil
}

// contextString reports what the project context contains, how big each part
// is and which mode is in effect.
func (a *app) contextString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s: %s)\n", a.msg.ContextTitle, a.msg.ContextModeLabel, a.cfg.ContextMode())

	if a.proj == nil {
		b.WriteString("  " + a.msg.NoContext + "\n")
		if omitted := a.omittedParts(); len(omitted) > 0 {
			b.WriteString("  " + a.msg.ContextOmitted + ": " + strings.Join(omitted, ", ") + "\n")
		}
		return b.String()
	}

	total := 0
	for _, part := range a.proj.Parts() {
		size := len(part.Content)
		total += size
		fmt.Fprintf(&b, "  %-22s %9s  ≈ %s\n", a.msg.ContextPart(part.Name), attach.HumanBytes(size), estimateTokens(size))
	}
	if total == 0 {
		b.WriteString("  " + a.msg.NoContext + "\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  %-22s %9s  ≈ %s\n", a.msg.ContextTotal, attach.HumanBytes(total), estimateTokens(total))
	if omitted := a.omittedParts(); len(omitted) > 0 {
		b.WriteString("\n  " + a.msg.ContextOmitted + ": " + strings.Join(omitted, ", ") + "\n")
	}
	// In full mode, show how much the light mode would save.
	if a.cfg.ContextMode() == "full" {
		if light, err := project.LoadWith(".", project.Options{}); err == nil {
			if saved := total - len(light.Context()); saved > 0 {
				fmt.Fprintf(&b, "  %s: %s (≈ %s tokens)\n", a.msg.ContextSaveLight, attach.HumanBytes(saved), estimateTokens(saved))
			}
		}
	}
	return b.String()
}

// omittedParts lists what the current mode leaves out of the prompt.
func (a *app) omittedParts() []string {
	switch a.cfg.ContextMode() {
	case "light":
		return []string{"README.md", "git diff"}
	case "off":
		return []string{a.msg.ContextAll}
	}
	return nil
}

// estimateTokens approximates a token count from a byte size.
//
// The divisor is 3 because that is what this content mix actually measures:
// a prompt of 12.4 KB was reported by the API as 4 151 tokens. Plain English
// prose is closer to 4 bytes per token, markdown and JSON closer to 3. The
// authoritative numbers come from the API and are shown by /tokens.
func estimateTokens(bytes int) string {
	return groupNumber(bytes / 3)
}

func shortTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}

func (a *app) configCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(a.msg.ConfigUsage)
	}
	cfg := a.cfg
	switch args[0] {
	case "get":
		src := config.APIKeySource(cfg)
		keyDisplay := a.msg.NotSet
		if src != "not set" {
			keyDisplay = "****"
		}
		fmt.Printf("api_key:  %s (%s)\n", keyDisplay, src)
		fmt.Printf("model:    %s\n", display(cfg.Model, deepseek.DefaultModel, a.msg.Default))
		fmt.Printf("base_url: %s\n", display(cfg.BaseURL, deepseek.DefaultBaseURL, a.msg.Default))
		fmt.Printf("lang:     %s\n", display(cfg.Lang, string(i18n.EN), a.msg.Default))
		fmt.Printf("thinking: %s\n", display(cfg.Thinking, "enabled", a.msg.Default))
		fmt.Printf("effort:   %s\n", display(cfg.ReasoningEffort, "high", a.msg.Default))
		shellState := "off"
		if cfg.ShellEnabled() {
			shellState = "on"
		}
		fmt.Printf("shell:    %s (%s)\n", shellState, shell.Name())
		fmt.Printf("shell_timeout: %s\n", a.shellTimeout().Round(time.Second))
		fmt.Printf("context:  %s\n", cfg.ContextMode())
		compact := "off"
		if cfg.CompactEnabled() {
			compact = "auto"
		}
		fmt.Printf("compact:  %s\n", compact)
		fmt.Printf("context_window: %d\n", a.contextWindow())
		return nil
	case "set":
		if len(args) != 3 {
			return errors.New(a.msg.SetUsage)
		}
		switch args[1] {
		case "api_key":
			if err := config.SetAPIKey(cfg, args[2]); err != nil {
				return err
			}
		case "model":
			cfg.Model = args[2]
			if err := cfg.Save(); err != nil {
				return err
			}
		case "base_url":
			cfg.BaseURL = args[2]
			if err := cfg.Save(); err != nil {
				return err
			}
		case "lang":
			lang, ok := i18n.Parse(args[2])
			if !ok {
				return errors.New(a.msg.InvalidLangValue)
			}
			cfg.Lang = string(lang)
			if err := cfg.Save(); err != nil {
				return err
			}
			a.msg = i18n.For(lang) // apply to the current session
		case "thinking":
			v := strings.ToLower(args[2])
			if v != "enabled" && v != "disabled" && v != "on" && v != "off" {
				return errors.New(a.msg.InvalidThinkingValue)
			}
			if v == "on" {
				v = "enabled"
			}
			if v == "off" {
				v = "disabled"
			}
			cfg.Thinking = v
			if err := cfg.Save(); err != nil {
				return err
			}
		case "reasoning_effort":
			v := strings.ToLower(args[2])
			if v != "low" && v != "high" && v != "max" {
				return errors.New(a.msg.InvalidEffortValue)
			}
			cfg.ReasoningEffort = v
			if err := cfg.Save(); err != nil {
				return err
			}
		case "shell":
			v := strings.ToLower(args[2])
			switch v {
			case "on", "true", "yes", "1", "enabled":
				v = "on"
			case "off", "false", "no", "0", "disabled":
				v = "off"
			default:
				return errors.New(a.msg.InvalidShellValue)
			}
			cfg.Shell = v
			if err := cfg.Save(); err != nil {
				return err
			}
		case "shell_timeout":
			secs, err := strconv.Atoi(args[2])
			if err != nil || secs <= 0 || secs > int(shell.MaxTimeout/time.Second) {
				return errors.New(a.msg.InvalidShellTimeout)
			}
			cfg.ShellTimeout = secs
			if err := cfg.Save(); err != nil {
				return err
			}
		case "context":
			v := strings.ToLower(args[2])
			switch v {
			case "full", "completo":
				v = "full"
			case "light", "lite", "minimal":
				v = "light"
			case "off", "none", "no":
				v = "off"
			default:
				return errors.New(a.msg.InvalidContextValue)
			}
			cfg.Context = v
			if err := cfg.Save(); err != nil {
				return err
			}
			a.loadProject()
		case "compact":
			v := strings.ToLower(args[2])
			switch v {
			case "auto", "on", "true", "yes", "1", "enabled":
				v = "auto"
			case "off", "false", "no", "0", "disabled":
				v = "off"
			default:
				return errors.New(a.msg.InvalidCompactValue)
			}
			cfg.Compact = v
			if err := cfg.Save(); err != nil {
				return err
			}
		case "sandbox":
			v := strings.ToLower(args[2])
			switch v {
			case "read-only", "readonly", "ro", "read":
				v = "read-only"
			case "workspace-write", "workspace", "write":
				v = "workspace-write"
			case "full", "unrestricted":
				v = "full"
			default:
				return errors.New(a.msg.InvalidSandboxValue)
			}
			cfg.Sandbox = v
			if err := cfg.Save(); err != nil {
				return err
			}
			if err := a.setSandbox(v); err != nil {
				return err
			}
		case "approval":
			v := strings.ToLower(args[2])
			switch v {
			case "ask", "always", "on":
				v = "ask"
			case "auto", "never", "off":
				v = "auto"
			default:
				return errors.New(a.msg.InvalidApprovalValue)
			}
			cfg.Approval = v
			if err := cfg.Save(); err != nil {
				return err
			}
			a.approvalAuto = cfg.ApprovalAuto()
		case "context_window":
			n, err := strconv.Atoi(args[2])
			if err != nil || n <= 0 || n > 10_000_000 {
				return errors.New(a.msg.InvalidWindowValue)
			}
			cfg.ContextWindow = n
			if err := cfg.Save(); err != nil {
				return err
			}
		default:
			return errors.New(a.msg.UnknownConfigKey(args[1]))
		}
		fmt.Println(a.msg.Saved)
		return nil
	case "unset":
		if len(args) != 2 {
			return errors.New(a.msg.UnsetUsage)
		}
		switch args[1] {
		case "api_key":
			if err := config.DeleteAPIKey(cfg); err != nil {
				return err
			}
		case "model":
			cfg.Model = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "base_url":
			cfg.BaseURL = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "lang":
			cfg.Lang = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "thinking":
			cfg.Thinking = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "reasoning_effort":
			cfg.ReasoningEffort = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "shell":
			cfg.Shell = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "shell_timeout":
			cfg.ShellTimeout = 0
			if err := cfg.Save(); err != nil {
				return err
			}
		case "context":
			cfg.Context = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "compact":
			cfg.Compact = ""
			if err := cfg.Save(); err != nil {
				return err
			}
		case "sandbox":
			cfg.Sandbox = ""
			if err := cfg.Save(); err != nil {
				return err
			}
			if err := a.setSandbox("workspace-write"); err != nil {
				return err
			}
		case "approval":
			cfg.Approval = ""
			if err := cfg.Save(); err != nil {
				return err
			}
			a.approvalAuto = cfg.ApprovalAuto()
		case "context_window":
			cfg.ContextWindow = 0
			if err := cfg.Save(); err != nil {
				return err
			}
		default:
			return errors.New(a.msg.UnknownConfigKey(args[1]))
		}
		fmt.Println(a.msg.Removed)
		return nil
	default:
		return errors.New(a.msg.UnknownConfigKey(args[0]))
	}
}

func (a *app) historyCmd(args []string) error {
	dir := session.Dir()

	if len(args) == 0 {
		list, err := session.List(dir)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println(a.msg.NoConversations)
			return nil
		}
		for _, s := range list {
			fmt.Printf("%-24s %s  %s  (%s)\n", s.ID,
				s.Updated.Format("2006-01-02 15:04"), s.Title, a.msg.HistoryTurns(s.Turns))
		}
		return nil
	}

	switch args[0] {
	case "search":
		if len(args) < 2 {
			return errors.New(a.msg.HistorySearchUsage)
		}
		return a.historySearch(strings.Join(args[1:], " "))
	case "show":
		if len(args) != 2 {
			return errors.New(a.msg.HistoryShowUsage)
		}
		s, err := session.Load(dir, args[1])
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return errors.New(a.msg.SessionNotFound(args[1]))
			}
			return err
		}
		msgs := s.Messages()
		if len(msgs) == 0 {
			return errors.New(a.msg.SessionNotFound(args[1]))
		}
		for _, m := range msgs {
			if m.Role == "assistant" && len(m.ToolCalls) > 0 {
				names := make([]string, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					names = append(names, tc.Function.Name)
				}
				fmt.Printf("\n[assistant → %s]\n%s\n", strings.Join(names, ", "), m.Content)
				continue
			}
			fmt.Printf("\n[%s]\n%s\n", m.Role, m.Content)
		}
		return nil
	case "delete":
		if len(args) != 2 {
			return errors.New(a.msg.HistoryDeleteUsage)
		}
		if err := session.Delete(dir, args[1]); err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return errors.New(a.msg.SessionNotFound(args[1]))
			}
			return err
		}
		fmt.Println(a.msg.SessionDeleted(args[1]))
		return nil
	default:
		return errors.New(a.msg.UnknownHistoryCmd(args[0]))
	}
}

func display(v, def, defaultSuffix string) string {
	if v == "" {
		return def + " " + defaultSuffix
	}
	return v
}
