package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/i18n"
)

func testApp() *app {
	return &app{
		cfg:  &config.Config{},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{deny: map[string]bool{}},
	}
}

// TestChatTurnRunsShellTool exercises the whole loop: the model asks for
// run_shell, the command really runs, and its output goes back to the model.
func TestChatTurnRunsShellTool(t *testing.T) {
	var round int
	var firstReq deepseek.ChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		round++
		body, _ := io.ReadAll(r.Body)
		if round == 1 {
			_ = json.Unmarshal(body, &firstReq)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if round == 1 {
			// Tool call: run_shell("echo hola").
			fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"run_shell","arguments":"{\"command\":\"echo hola\"}"}}]},"finish_reason":null}]}`+"\n\n")
			fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"listo"},"finish_reason":null}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := testApp()
	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	cb := chatCallbacks{
		onChunk:     func(string) {},
		onReasoning: func(string) {},
		onStatus:    func(string) {},
		askPerm:     func(deepseek.ToolCall) (bool, error) { return true, nil },
	}
	content, msgs, err := a.chatTurn(context.Background(), client, "deepseek-chat",
		[]deepseek.Message{{Role: "user", Content: "di hola"}}, cb)
	if err != nil {
		t.Fatalf("chatTurn: %v", err)
	}
	if content != "listo" {
		t.Errorf("content = %q, want %q", content, "listo")
	}

	offered := false
	for _, tool := range firstReq.Tools {
		if tool.Function.Name == shellToolName {
			offered = true
		}
	}
	if !offered {
		t.Errorf("run_shell was not offered to the model: %+v", firstReq.Tools)
	}

	var toolOutput string
	for _, m := range msgs {
		if m.Role == "tool" {
			toolOutput = m.Content
		}
	}
	if !strings.Contains(toolOutput, "hola") {
		t.Errorf("tool output = %q, want the command output", toolOutput)
	}
}

// A denied shell call must not run anything and must report the denial back.
func TestChatTurnDeniesShellTool(t *testing.T) {
	var round int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		round++
		w.Header().Set("Content-Type", "text/event-stream")
		if round == 1 {
			fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"run_shell","arguments":"{\"command\":\"echo no\"}"}}]},"finish_reason":null}]}`+"\n\n")
		} else {
			fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a := testApp()
	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	cb := chatCallbacks{
		onChunk:     func(string) {},
		onReasoning: func(string) {},
		onStatus:    func(string) {},
		askPerm:     func(deepseek.ToolCall) (bool, error) { return false, nil },
	}
	_, msgs, err := a.chatTurn(context.Background(), client, "deepseek-chat",
		[]deepseek.Message{{Role: "user", Content: "x"}}, cb)
	if err != nil {
		t.Fatalf("chatTurn: %v", err)
	}
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, "hola") || strings.Contains(m.Content, "no\n") {
			t.Errorf("denied command produced output: %q", m.Content)
		}
		if m.Content != a.msg.ToolDeniedContent {
			t.Errorf("tool message = %q, want the denial notice", m.Content)
		}
	}
}

func TestShellToolNotOfferedWhenDisabled(t *testing.T) {
	a := testApp()
	a.cfg.Shell = "off"
	for _, tool := range a.allTools() {
		if tool.Function.Name == shellToolName {
			t.Fatal("run_shell offered even though the shell is disabled")
		}
	}
	if _, err := a.runShellTool(context.Background(), `{"command":"echo hola"}`); err == nil {
		t.Fatal("runShellTool should fail when the shell tool is disabled")
	}
}

func TestToolDetailShowsTheCommand(t *testing.T) {
	tc := deepseek.ToolCall{}
	tc.Function.Name = shellToolName
	tc.Function.Arguments = `{"command":"go test ./...","cwd":"/tmp"}`

	got := toolDetail(tc)
	if !strings.Contains(got, "go test ./...") {
		t.Errorf("toolDetail = %q, want the command", got)
	}
	if strings.Contains(got, `\"`) {
		t.Errorf("toolDetail = %q, want it to be readable rather than raw JSON", got)
	}

	other := deepseek.ToolCall{}
	other.Function.Name = "read_file"
	other.Function.Arguments = `{"path":"a.go"}`
	if got := toolDetail(other); !strings.Contains(got, "a.go") {
		t.Errorf("toolDetail for MCP tools = %q, want the raw arguments", got)
	}
}

func TestCommandRegistryCoversEveryCommand(t *testing.T) {
	// Every command handled by slashCmd/handleSlash must be in the registry,
	// otherwise it is missing from the help and from autocompletion.
	want := []string{"help", "exit", "login", "models", "model", "list-models",
		"thinking", "effort", "context", "mcp", "shell", "run", "copy", "save"}
	have := map[string]bool{}
	for _, name := range commandNames() {
		have[name] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("command %q is missing from the registry", name)
		}
	}
}

func TestCommandHelpListsEveryCommand(t *testing.T) {
	help := testApp().helpCatalog()
	for _, name := range commandNames() {
		// Hidden aliases are allowed to be absent from the listing.
		if strings.Contains(help, "/"+name+" ") || strings.Contains(help, "/"+name+"\n") {
			continue
		}
		if name == "model" {
			continue // documented alias of /models
		}
		t.Errorf("command %q does not appear in the generated help", name)
	}
	if !strings.Contains(help, "/copy all") {
		t.Error("help does not list /copy all")
	}
}

func TestHelpTopicCard(t *testing.T) {
	a := testApp()
	card, ok := a.helpTopic("copy")
	if !ok {
		t.Fatal("/help copy found nothing")
	}
	for _, want := range []string{"/copy", "Uso", "portapapeles", "/copy all"} {
		if !strings.Contains(card, want) {
			t.Errorf("/help copy is missing %q:\n%s", want, card)
		}
	}

	// Subcommand usages resolve too.
	if _, ok := a.helpTopic("copy all"); !ok {
		t.Error("/help copy all found nothing")
	}
	// A leading slash is accepted.
	if _, ok := a.helpTopic("/run"); !ok {
		t.Error("/help /run found nothing")
	}
	if _, ok := a.helpTopic("no-existe"); ok {
		t.Error("/help no-existe should not resolve")
	}
}

func TestHelpCatalogHasAllSections(t *testing.T) {
	help := testApp().helpCatalog()
	for _, want := range []string{"Comandos", "Atajos de teclado", "Skills del proyecto", "Servidores MCP"} {
		if !strings.Contains(help, want) {
			t.Errorf("catalog is missing the %q section:\n%s", want, help)
		}
	}
}
