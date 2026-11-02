package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"deekseep-coder/internal/config"
	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/i18n"
)

func TestTUIHelpers(t *testing.T) {
	if got := replaceAt("hola @x", 7, "@x", "@xy"); got != "hola @xy" {
		t.Errorf("replaceAt = %q", got)
	}
	if got := commonPrefix([]string{"/list-models", "/login"}); got != "/l" {
		t.Errorf("commonPrefix = %q", got)
	}
	if got := commonPrefix([]string{}); got != "" {
		t.Errorf("commonPrefix(empty) = %q", got)
	}
}

func TestTUIHelpAndQuit(t *testing.T) {
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{allow: map[string]bool{}, deny: map[string]bool{}},
	}
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	m := newTUIModel(a, client, "deepseek-chat", nil)
	p := tea.NewProgram(m, tea.WithInput(strings.NewReader("/help\r\x03")), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	fm, ok := final.(tuiModel)
	if !ok {
		t.Fatalf("final model type = %T", final)
	}
	if len(fm.entries) < 2 {
		t.Errorf("expected at least 2 entries, got %d", len(fm.entries))
	}
	found := false
	for _, e := range fm.entries {
		if e.role == "info" && strings.Contains(e.content, "/login") {
			found = true
		}
	}
	if !found {
		t.Errorf("/help did not show the slash help: %+v", fm.entries)
	}
}

func TestTUIUnknownCommand(t *testing.T) {
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{allow: map[string]bool{}, deny: map[string]bool{}},
	}
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	m := newTUIModel(a, client, "deepseek-chat", nil)
	p := tea.NewProgram(m, tea.WithInput(strings.NewReader("/noexiste\r\x03")), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	fm := final.(tuiModel)
	found := false
	for _, e := range fm.entries {
		if e.role == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error entry for unknown command: %+v", fm.entries)
	}
}

// delayedReader provides the first chunk immediately and the second after a
// delay, so the chat stream can finish before the quit key arrives.
type delayedReader struct {
	first, second []byte
	delay         time.Duration
	stage         int
}

func (r *delayedReader) Read(p []byte) (int, error) {
	switch r.stage {
	case 0:
		r.stage = 1
		copy(p, r.first)
		return len(r.first), nil
	case 1:
		time.Sleep(r.delay)
		r.stage = 2
		copy(p, r.second)
		return len(r.second), nil
	default:
		return 0, io.EOF
	}
}

func TestTUIModelSelect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(deepseek.ModelsResponse{
			Object: "list",
			Data: []deepseek.Model{
				{ID: "deepseek-chat"},
				{ID: "deepseek-reasoner"},
			},
		})
	}))
	defer server.Close()

	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{allow: map[string]bool{}, deny: map[string]bool{}},
	}
	m := newTUIModel(a, client, "deepseek-chat", nil)
	in := &delayedReader{first: []byte("/models\r"), second: []byte("\x03"), delay: 500 * time.Millisecond}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	fm := final.(tuiModel)
	vp := fm.viewport.View()
	t.Logf("mode: %v", fm.mode)
	t.Logf("entries: %+v", fm.entries)
	t.Logf("viewport: %q", vp)
	if !strings.Contains(vp, "deepseek-chat") || !strings.Contains(vp, "deepseek-reasoner") {
		t.Errorf("viewport missing model list: %q", vp)
	}
	if fm.mode != modeModelSelect {
		t.Errorf("mode = %v, want modeModelSelect", fm.mode)
	}
}

func TestTUIStreamingChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"reasoning_content":"The answer "},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hola"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{allow: map[string]bool{}, deny: map[string]bool{}},
	}
	m := newTUIModel(a, client, "deepseek-chat", nil)
	in := &delayedReader{first: []byte("hola\r"), second: []byte("\x03"), delay: 500 * time.Millisecond}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	fm := final.(tuiModel)
	found := false
	for _, e := range fm.entries {
		if e.role == "assistant" && strings.Contains(e.content, "Hola!") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected assistant response, entries: %+v", fm.entries)
	}
	// The response must also be visible in the viewport.
	if !strings.Contains(fm.viewport.View(), "Hola!") {
		t.Errorf("assistant response not visible in viewport: %q", fm.viewport.View())
	}
	if fm.streaming {
		t.Errorf("streaming did not finish: entries=%+v", fm.entries)
	}
	if fm.reasoning == "" {
		t.Errorf("reasoning content was not displayed, got %q", fm.reasoning)
	}
}
