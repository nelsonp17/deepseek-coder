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
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"

	"deekseep-coder/internal/config"
	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/editor"
	"deekseep-coder/internal/history"
	"deekseep-coder/internal/i18n"
	"deekseep-coder/internal/mcpx"
	"deekseep-coder/internal/project"
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
}

// systemPrompt builds the system prompt including the project context.
func (a *app) systemPrompt() string {
	sp := a.msg.SystemPrompt
	if a.proj != nil {
		if ctx := a.proj.Context(); ctx != "" {
			sp += "\n\n" + ctx
		}
	}
	return sp
}

func run(args []string) error {
	// --debug enables verbose logging of API requests.
	if len(args) > 0 && (args[0] == "--debug" || args[0] == "-d") {
		os.Setenv("DEEPSEEK_DEBUG", "1")
		args = args[1:]
	}

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
		cfg:  cfg,
		msg:  i18n.For(i18n.Lang(cfg.Lang)),
		perm: &permissions{allow: map[string]bool{}, deny: map[string]bool{}},
	}

	if len(args) == 0 {
		return a.repl()
	}
	switch args[0] {
	case "ask":
		return a.ask(strings.Join(args[1:], " "))
	case "config":
		return a.configCmd(args[1:])
	case "history":
		return a.historyCmd(args[1:])
	case "models":
		return a.modelsCmd()
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

// stdinInteractive is overridable in tests.
var stdinInteractive = isInteractive

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

func (a *app) ask(question string) error {
	if question == "" {
		return errors.New(a.msg.EmptyQuestion)
	}
	client, err := a.newClient()
	if err != nil {
		return err
	}
	model, err := a.ensureModel(client)
	if err != nil {
		return err
	}
	a.proj, _ = project.Load(".")
	a.connectMCP()
	if a.mcp != nil {
		defer a.mcp.Close()
	}
	ctx := context.Background()
	messages := []deepseek.Message{
		{Role: "system", Content: a.systemPrompt()},
		{Role: "user", Content: question},
	}
	_, _, err = a.chatTurn(ctx, client, model, messages, chatCallbacks{
		onChunk: func(delta string) {
			fmt.Print(delta)
		},
		onStatus: func(s string) {
			fmt.Fprintln(os.Stderr, s)
		},
		askPerm: a.confirmTool,
	})
	if err != nil {
		return err
	}
	fmt.Println()
	return nil
}

// maxToolRounds bounds the tool-calling loop to prevent infinite loops.
const maxToolRounds = 10

// chatCallbacks wires the chat loop to the UI (terminal or TUI).
type chatCallbacks struct {
	onChunk     func(string)                          // streamed content
	onReasoning func(string)                          // reasoning content (thinking models)
	onStatus    func(string)                          // status lines (tool running, denied...)
	askPerm     func(deepseek.ToolCall) (bool, error) // ask the user for tool permission
}

// chatTurn runs the model until it stops requesting tools. Every tool call
// is confirmed with the user before execution. It returns the final content
// and the full message history (including tool calls and results).
func (a *app) chatTurn(ctx context.Context, client *deepseek.Client, model string, messages []deepseek.Message, cb chatCallbacks) (string, []deepseek.Message, error) {
	for round := 0; round < maxToolRounds; round++ {
		req := deepseek.ChatRequest{Model: model, Messages: messages}
		if a.cfg.Thinking != "" {
			req.Thinking = map[string]any{"type": a.cfg.Thinking}
		}
		if a.cfg.ReasoningEffort != "" {
			req.ReasoningEffort = a.cfg.ReasoningEffort
		}
		if a.mcp != nil {
			req.Tools = a.deepSeekTools()
		}
		result, err := client.StreamChat(ctx, req, cb.onChunk, cb.onReasoning)
		if err != nil {
			return "", messages, err
		}
		if len(result.ToolCalls) == 0 {
			messages = append(messages, deepseek.Message{Role: "assistant", Content: result.Content})
			return result.Content, messages, nil
		}

		messages = append(messages, deepseek.Message{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
		for _, tc := range result.ToolCalls {
			allowed, err := cb.askPerm(tc)
			if err != nil {
				return "", messages, err
			}
			if !allowed {
				cb.onStatus(a.msg.ToolDenied(tc.Function.Name))
				messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: a.msg.ToolDeniedContent})
				continue
			}
			cb.onStatus(a.msg.ToolRunning(tc.Function.Name))
			output, err := a.executeTool(ctx, tc)
			if err != nil {
				messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: "Error: " + err.Error()})
				continue
			}
			messages = append(messages, deepseek.Message{Role: "tool", ToolCallID: tc.ID, Content: output})
		}
	}
	return "", messages, errors.New(a.msg.ToolLoopLimit)
}

// permissions remembers per-tool allow/deny decisions for the session.
type permissions struct {
	allow map[string]bool
	deny  map[string]bool
}

// confirmTool asks the user for permission before executing a tool call.
// When the user cannot be asked (non-interactive), the call is denied.
func (a *app) confirmTool(tc deepseek.ToolCall) (bool, error) {
	if a.perm.allow[tc.Function.Name] {
		return true, nil
	}
	if a.perm.deny[tc.Function.Name] {
		return false, nil
	}
	if !stdinInteractive() {
		return false, nil // deny by default when we cannot ask
	}
	fmt.Printf("\n%s\n", a.msg.ToolRequest(tc.Function.Name, tc.Function.Arguments))
	fmt.Print(a.msg.ToolPrompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "a", "always":
		a.perm.allow[tc.Function.Name] = true
		return true, nil
	case "d", "deny":
		a.perm.deny[tc.Function.Name] = true
		return false, nil
	default:
		return false, nil
	}
}

// executeTool runs a tool call through the connected MCP servers.
func (a *app) executeTool(ctx context.Context, tc deepseek.ToolCall) (string, error) {
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
	a.proj, _ = project.Load(".")
	a.connectMCP()
	if a.mcp != nil {
		defer a.mcp.Close()
	}

	if isInteractive() {
		return a.replTUI(client, model)
	}
	return a.replLine(client, model)
}

// replLine is the fallback line-based REPL used when stdin is not a terminal.
func (a *app) replLine(client *deepseek.Client, model string) error {
	ctx := context.Background()
	messages := []deepseek.Message{{Role: "system", Content: a.systemPrompt()}}
	sessionModel := model

	store, err := history.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, a.msg.HistoryDisabled, err)
		store = nil
	} else {
		defer store.Close()
	}
	var convID int64

	fmt.Println(a.msg.Welcome)
	scanner := bufio.NewScanner(os.Stdin)
	interactive := true
	for {
		fmt.Println()
		var line string
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
				line = ref
			} else {
				fmt.Fprintln(os.Stderr, "error:", a.msg.UnknownSlashCmd(line))
				continue
			}
		}
		messages = append(messages, deepseek.Message{Role: "user", Content: line})
		if store != nil && convID == 0 {
			convID, err = store.CreateConversation(shortTitle(line))
			if err != nil {
				fmt.Fprintln(os.Stderr, a.msg.SaveFailed, err)
				store = nil
			}
		}
		if store != nil {
			if err := store.AppendMessage(convID, "user", line); err != nil {
				fmt.Fprintln(os.Stderr, a.msg.SaveFailed, err)
			}
		}
		fmt.Println()
		reply, newMessages, err := a.chatTurn(ctx, client, sessionModel, messages, chatCallbacks{
			onChunk: func(delta string) {
				fmt.Print(delta)
			},
			onStatus: func(s string) {
				fmt.Fprintln(os.Stderr, s)
			},
			askPerm: a.confirmTool,
		})
		messages = newMessages
		fmt.Println()
		if err != nil {
			return err
		}
		if store != nil {
			if err := store.AppendMessage(convID, "assistant", reply); err != nil {
				fmt.Fprintln(os.Stderr, a.msg.SaveFailed, err)
			}
		}
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
	case line == "/context":
		return true, a.showContext()
	case line == "/context reload":
		a.proj, _ = project.Load(".")
		fmt.Println(a.msg.ContextReloaded)
		return true, a.showContext()
	case strings.HasPrefix(line, "/context"):
		return true, errors.New(a.msg.ContextUsage)
	case line == "/help":
		fmt.Print(a.msg.SlashHelp)
		return true, nil
	default:
		return false, nil
	}
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
func (a *app) skillOrToolRef(line string) (string, bool) {
	parts := strings.SplitN(line, " ", 2)
	name := strings.TrimPrefix(parts[0], "/")
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}
	for _, s := range a.skillNames() {
		if s == name {
			msg := a.msg.SkillRef(s)
			if rest != "" {
				msg += " " + rest
			}
			return msg, true
		}
	}
	for _, t := range a.mcpToolNames() {
		if t == name {
			msg := a.msg.ToolRef(t)
			if rest != "" {
				msg += " " + rest
			}
			return msg, true
		}
	}
	return "", false
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
		if !strings.HasPrefix(e.Name(), base) {
			continue
		}
		cand := e.Name()
		if e.IsDir() {
			cand += string(filepath.Separator)
		}
		candidates = append(candidates, "@"+filepath.Join(dir, cand))
	}
	sort.Strings(candidates)
	return editor.Completion{Prefix: "@" + token, Candidates: candidates}
}

// commandCompletion completes a command token (after '/') with commands,
// skills and MCP tools.
func (a *app) commandCompletion(token string) editor.Completion {
	var candidates []string
	for _, c := range a.commandNames() {
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

func (a *app) commandNames() []string {
	return []string{"login", "exit", "models", "model", "list-models", "mcp", "context", "help"}
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

// contextString returns the project context as text.
func (a *app) contextString() string {
	if a.proj == nil {
		return a.msg.NoContext
	}
	ctx := a.proj.Context()
	if ctx == "" {
		return a.msg.NoContext
	}
	return a.msg.ContextTitle + "\n" + ctx
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
	store, err := history.Open()
	if err != nil {
		return err
	}
	defer store.Close()

	if len(args) == 0 {
		convs, err := store.ListConversations()
		if err != nil {
			return err
		}
		if len(convs) == 0 {
			fmt.Println(a.msg.NoConversations)
			return nil
		}
		for _, c := range convs {
			fmt.Printf("%3d  %s  %s\n", c.ID, c.UpdatedAt.Format("2006-01-02 15:04"), c.Title)
		}
		return nil
	}

	switch args[0] {
	case "show":
		if len(args) != 2 {
			return errors.New(a.msg.HistoryShowUsage)
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return errors.New(a.msg.InvalidID(args[1]))
		}
		msgs, err := store.GetConversation(id)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			return errors.New(a.msg.ConversationNotFound(id))
		}
		for _, m := range msgs {
			fmt.Printf("\n[%s]\n%s\n", m.Role, m.Content)
		}
		return nil
	case "delete":
		if len(args) != 2 {
			return errors.New(a.msg.HistoryDeleteUsage)
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return errors.New(a.msg.InvalidID(args[1]))
		}
		if err := store.DeleteConversation(id); err != nil {
			return err
		}
		fmt.Println(a.msg.ConversationDeleted(id))
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
