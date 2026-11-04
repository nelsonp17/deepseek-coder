package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		perm: &permissions{deny: map[string]bool{}},
	}
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	m := newTUIModel(a, client, "deepseek-chat")
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
		perm: &permissions{deny: map[string]bool{}},
	}
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	m := newTUIModel(a, client, "deepseek-chat")
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
		perm: &permissions{deny: map[string]bool{}},
	}
	m := newTUIModel(a, client, "deepseek-chat")
	in := &delayedReader{first: []byte("/models\r"), second: []byte("\x03"), delay: 2 * time.Second}
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
		// The usage chunk arrives after the answer. It used to kill the reader
		// chain and leave the TUI stuck on "pensando...", so this line is part
		// of the regression: the turn must still close.
		fmt.Fprint(w, `data: {"id":"1","choices":[],"usage":{"prompt_tokens":42,"completion_tokens":2,"total_tokens":44}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{deny: map[string]bool{}},
	}
	m := newTUIModel(a, client, "deepseek-chat")
	in := &delayedReader{first: []byte("hola\r"), second: []byte("\x03"), delay: 2 * time.Second}
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

func testTUIModel(t *testing.T) tuiModel {
	t.Helper()
	a := &app{
		cfg:  &config.Config{Model: "deepseek-chat"},
		msg:  i18n.For(i18n.ES),
		perm: &permissions{deny: map[string]bool{}},
	}
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	return newTUIModel(a, client, "deepseek-chat")
}

// Scrolling up to read history must not be undone by the next streamed chunk.
func TestViewportKeepsScrollPosition(t *testing.T) {
	m := testTUIModel(t)
	m.width, m.height = 40, 20
	m = m.handleResize()
	for i := 0; i < 40; i++ {
		m = m.addEntry("assistant", fmt.Sprintf("linea %d", i))
	}
	m = m.refreshViewportFollow()
	if !m.viewport.AtBottom() {
		t.Fatal("expected the viewport to start at the bottom")
	}

	m.viewport.SetYOffset(0)
	offset := m.viewport.YOffset

	m = m.addEntry("assistant", "texto nuevo")
	m = m.refreshViewport()

	if m.viewport.AtBottom() {
		t.Error("viewport jumped to the bottom while the user was reading history")
	}
	if m.viewport.YOffset != offset {
		t.Errorf("YOffset = %d, want %d", m.viewport.YOffset, offset)
	}
}

// Arrow keys are the terminal's wheel translation: they may scroll the chat
// only while the input is empty. With text in the input they belong to the
// textarea, so typing is never disturbed.
func TestViewportOnlyGetsScrollKeys(t *testing.T) {
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyUp}, true); !ok {
		t.Error("arrow keys with an empty input should scroll the viewport")
	}
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyDown}, true); !ok {
		t.Error("arrow keys with an empty input should scroll the viewport")
	}
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyUp}, false); ok {
		t.Error("arrow keys while typing must not reach the viewport")
	}
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, true); ok {
		t.Error("typing must not reach the viewport")
	}
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyPgUp}, false); !ok {
		t.Error("pgup must always reach the viewport")
	}
	if _, ok := viewportMsg(tea.KeyMsg{Type: tea.KeyPgDown}, false); !ok {
		t.Error("pgdown must always reach the viewport")
	}
	if _, ok := viewportMsg(tea.MouseMsg{}, false); !ok {
		t.Error("mouse events must reach the viewport")
	}
}

func TestPlainEntriesIsCopyable(t *testing.T) {
	m := testTUIModel(t)
	m = m.addEntry("user", "hola")
	m = m.addEntry("assistant", "mundo")
	got := m.plainEntries()
	if !strings.Contains(got, "hola") || !strings.Contains(got, "mundo") {
		t.Errorf("transcript lost content: %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("transcript contains ANSI escapes: %q", got)
	}
	if !strings.Contains(got, m.app.msg.UserLabel) {
		t.Errorf("transcript is missing the user label: %q", got)
	}
}

func TestSaveTranscriptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.md")
	got, err := saveTranscriptFile(path, "hola\n")
	if err != nil {
		t.Fatalf("saveTranscriptFile: %v", err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hola\n" {
		t.Errorf("saved %q, want %q", string(data), "hola\n")
	}
}

// Assistant answers are shown rendered, not as raw Markdown source.
func TestAssistantMessagesAreRendered(t *testing.T) {
	m := testTUIModel(t)
	m.width, m.height = 60, 24
	m = m.handleResize()
	m = m.addEntry("user", "hola")
	m = m.addEntry("assistant", "**Importante:** usa `go build`.\n\n- uno\n- dos")
	m = m.refreshViewportFollow()

	vp := m.viewport.View()
	for _, marker := range []string{"**", "`"} {
		if strings.Contains(vp, marker) {
			t.Errorf("raw Markdown marker %q leaked into the viewport:\n%s", marker, vp)
		}
	}
	for _, want := range []string{"Importante", "go build", "• uno"} {
		if !strings.Contains(vp, want) {
			t.Errorf("missing %q in the viewport:\n%s", want, vp)
		}
	}
}

// The per-entry render cache must notice content and width changes.
func TestRenderCacheInvalidates(t *testing.T) {
	m := testTUIModel(t)
	m.width, m.height = 60, 24
	m = m.handleResize()
	m = m.addEntry("assistant", "uno")
	m = m.refreshViewport()

	idx := len(m.entries) - 1
	before := m.renderCache[idx].out

	m.entries[idx].content = "dos"
	m = m.refreshViewport()
	if m.renderCache[idx].out == before {
		t.Error("cache was not invalidated by a content change")
	}
	if !strings.Contains(m.viewport.View(), "dos") {
		t.Errorf("new content is not visible:\n%s", m.viewport.View())
	}

	// A resize changes the wrap width, so everything must be re-rendered.
	m.width = 40
	m = m.handleResize()
	m = m.refreshViewport()
	if m.renderWidth != m.viewport.Width-4 {
		t.Errorf("renderWidth = %d, want %d", m.renderWidth, m.viewport.Width-4)
	}
}
