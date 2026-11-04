package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"deekseep-coder/internal/config"
	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/session"
)

// seededApp builds an app with a session full of large messages.
func seededApp(t *testing.T) (*app, *session.Session) {
	t.Helper()
	sess, err := session.Create(t.TempDir(), session.Meta{Model: "deepseek-chat"})
	if err != nil {
		t.Fatalf("session.Create: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	for i := 0; i < 10; i++ {
		if err := sess.AppendUser(strings.Repeat("u", 3000)); err != nil {
			t.Fatal(err)
		}
		if err := sess.AppendAssistant(strings.Repeat("a", 3000), nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := sess.AppendTurnEnd("completed"); err != nil {
			t.Fatal(err)
		}
	}

	a := testApp()
	a.session = sess
	a.modelWindow = 20000
	return a, sess
}

// summaryServer answers /chat/completions with a fixed summary and captures the
// request it was sent.
func summaryServer(t *testing.T, summary string, got *deepseek.ChatRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deepseek.ChatResponse{
			Choices: []struct {
				Index        int              `json:"index"`
				Message      deepseek.Message `json:"message"`
				FinishReason string           `json:"finish_reason"`
			}{{Message: deepseek.Message{Role: "assistant", Content: summary}}},
		})
	}))
}

// Pruning happens without a model call and only shortens old tool output.
func TestPruneNowShortensOldToolOutput(t *testing.T) {
	sess, err := session.Create(t.TempDir(), session.Meta{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	call := deepseek.ToolCall{ID: "c1"}
	call.Function.Name = "run_shell"
	if err := sess.AppendUser("ejecuta el build"); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendAssistant("", []deepseek.ToolCall{call}, nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendToolResult("c1", strings.Repeat("compilando... ", 2000), false); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendAssistant("listo", nil, nil); err != nil {
		t.Fatal(err)
	}

	a := testApp()
	a.session = sess
	a.cfg.ContextWindow = 6000

	saved, pruned, err := a.pruneNow(chatCallbacks{})
	if err != nil {
		t.Fatalf("pruneNow: %v", err)
	}
	if !pruned {
		t.Fatal("expected something to prune")
	}
	if saved <= 0 {
		t.Errorf("saved = %d, want a positive estimate", saved)
	}

	msgs := sess.Messages()
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "c1" {
		t.Errorf("tool result lost its identity: %+v", msgs[2])
	}
	if !strings.Contains(msgs[2].Content, "pruned") {
		t.Errorf("tool output was not shortened: %q", msgs[2].Content[:60])
	}
	// The head and tail are kept, so compare sizes rather than substrings.
	if len(msgs[2].Content) > 1000 {
		t.Errorf("tool output is still %d bytes, want it shortened", len(msgs[2].Content))
	}
}

// A second prune has nothing left to do.
func TestPruneNowIsIdempotent(t *testing.T) {
	a, _ := seededApp(t)
	// The seeded session has no tool results at all.
	if _, pruned, err := a.pruneNow(chatCallbacks{}); err != nil || pruned {
		t.Errorf("pruneNow = %v, %v; want nothing to prune", pruned, err)
	}
}
func TestCompactNowReplacesTheOldestSpan(t *testing.T) {
	a, sess := seededApp(t)
	before := len(sess.Messages())
	if before < 10 {
		t.Fatalf("seed produced only %d messages", before)
	}

	var req deepseek.ChatRequest
	server := summaryServer(t, "- resumen: el usuario pidió varias cosas", &req)
	defer server.Close()
	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})

	gotBefore, gotAfter, err := a.compactNow(context.Background(), client, "deepseek-chat", chatCallbacks{})
	if err != nil {
		t.Fatalf("compactNow: %v", err)
	}
	if gotAfter >= gotBefore {
		t.Errorf("size after = %d, want less than %d", gotAfter, gotBefore)
	}

	// The summary call must carry the system instruction and the transcript.
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" {
		t.Fatalf("summary request = %+v", req.Messages)
	}
	if !strings.Contains(req.Messages[1].Content, "uuuu") {
		t.Error("the transcript was not sent to the summariser")
	}

	msgs := sess.Messages()
	if len(msgs) >= before {
		t.Errorf("history has %d messages, want fewer than %d", len(msgs), before)
	}
	if !strings.Contains(msgs[0].Content, "resumen") {
		t.Errorf("first message is not the summary: %+v", msgs[0])
	}

	// The log keeps every original event: compaction hides, never deletes.
	list, err := session.List(dirOf(t, sess))
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
}

func dirOf(t *testing.T, s *session.Session) string {
	t.Helper()
	path := s.Path()
	if i := strings.LastIndexAny(path, `/\`); i > 0 {
		return path[:i]
	}
	return path
}

func TestCompactNowNeedsAWindowAndASession(t *testing.T) {
	a := testApp()
	client := deepseek.New(deepseek.Options{APIKey: "k"})
	if _, _, err := a.compactNow(context.Background(), client, "m", chatCallbacks{}); err == nil {
		t.Error("compaction without a session should fail")
	}

	_, sess := seededApp(t)
	a.session = sess
	a.modelWindow = 0
	if _, _, err := a.compactNow(context.Background(), client, "m", chatCallbacks{}); err == nil {
		t.Error("compaction without a context window should fail")
	}
}

func TestMaybeCompactRespectsConfigAndTrigger(t *testing.T) {
	a, sess := seededApp(t)
	server := summaryServer(t, "- resumen", nil)
	defer server.Close()
	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	before := len(sess.Messages())

	// Disabled by config: nothing happens even though the history is big.
	a.cfg.Compact = "off"
	a.maybeCompact(context.Background(), client, "m", chatCallbacks{})
	if len(sess.Messages()) != before {
		t.Error("compaction ran while disabled in the config")
	}

	// Enabled but far from the trigger.
	a.cfg.Compact = "auto"
	a.modelWindow = 10_000_000
	a.maybeCompact(context.Background(), client, "m", chatCallbacks{})
	if len(sess.Messages()) != before {
		t.Error("compaction ran before reaching the trigger")
	}

	// Enabled and over the trigger.
	a.modelWindow = 20000
	a.maybeCompact(context.Background(), client, "m", chatCallbacks{})
	if len(sess.Messages()) >= before {
		t.Error("compaction did not run when the window was nearly full")
	}
	a.cfg = &config.Config{}
}

func TestIsContextOverflow(t *testing.T) {
	overflow := []string{
		`deepseek API error: 400 Bad Request: {"error":{"message":"This model's maximum context length is 65536 tokens"}}`,
		`deepseek API error: 400: prompt is too long`,
		`context_length_exceeded`,
	}
	for _, s := range overflow {
		if !isContextOverflow(errors.New(s)) {
			t.Errorf("isContextOverflow(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"401 Unauthorized", "connection reset", "500 internal error"} {
		if isContextOverflow(errors.New(s)) {
			t.Errorf("isContextOverflow(%q) = true, want false", s)
		}
	}
	if isContextOverflow(nil) {
		t.Error("nil error must not be an overflow")
	}
}

func TestPressureUsesTheSessionAnchor(t *testing.T) {
	a, sess := seededApp(t)
	// Seed a usage anchor: the last request reported 12 000 prompt tokens.
	if err := sess.AppendUser("nueva pregunta"); err != nil {
		t.Fatal(err)
	}
	sess.AppendAssistant("respuesta", nil, &deepseek.Usage{PromptTokens: 12000, CompletionTokens: 100, TotalTokens: 12100})

	pressure := a.pressure()
	// The anchor plus the small message added afterwards, plus the system prompt.
	if pressure < 12100 {
		t.Errorf("pressure = %d, want it anchored above 12100", pressure)
	}
	if pressure > 14000 {
		t.Errorf("pressure = %d, it should stay close to the anchor", pressure)
	}
}
