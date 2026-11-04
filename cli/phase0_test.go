package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/i18n"
	"deepseek-coder/internal/project"
)

// writeSkillFile creates a SKILL.md with frontmatter and returns its path.
func writeSkillFile(t *testing.T, dir, name, description, body string) string {
	t.Helper()
	path := filepath.Join(dir, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Referencing a skill must send its instructions, not just its name.
func TestSkillReferenceLoadsTheBody(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillFile(t, dir, "caveman", "habla como un cavernícola", "# Reglas\n\nUsa frases cortas.\n")

	a := &app{
		msg:  i18n.For(i18n.ES),
		perm: &permissions{deny: map[string]bool{}},
		proj: &project.Project{Skills: []project.Skill{{Name: "caveman", Description: "habla como un cavernícola", Path: path}}},
	}
	msg, ok := a.skillOrToolRef("/caveman revisa esto")
	if !ok {
		t.Fatal("skill reference not resolved")
	}
	if !strings.Contains(msg.Payload, "Usa frases cortas") {
		t.Errorf("skill body was not injected:\n%s", msg.Payload)
	}
	if !strings.Contains(msg.Payload, "revisa esto") {
		t.Errorf("the rest of the message was lost:\n%s", msg.Payload)
	}
	// The transcript shows the reference and the typed text, never the body.
	if strings.Contains(msg.Display, "Usa frases cortas") {
		t.Errorf("the skill body leaked into the display:\n%s", msg.Display)
	}
	if !strings.Contains(msg.Display, "revisa esto") {
		t.Errorf("the display lost the typed text:\n%s", msg.Display)
	}
}

func TestReadSkillBodyStripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillFile(t, dir, "demo", "una skill", "# Instrucciones\n\nHaz algo.\n")

	body, err := project.ReadSkillBody(path)
	if err != nil {
		t.Fatalf("ReadSkillBody: %v", err)
	}
	if strings.Contains(body, "description:") || strings.HasPrefix(body, "---") {
		t.Errorf("frontmatter leaked into the body:\n%s", body)
	}
	if !strings.Contains(body, "Haz algo") {
		t.Errorf("body lost its content:\n%s", body)
	}
}

// The system message has to be rebuilt so the date and the project context are
// current; a prompt built once at startup goes stale at midnight.
func TestRefreshSystemPromptUpdatesInPlace(t *testing.T) {
	a := testApp()
	messages := []deepseek.Message{
		{Role: "system", Content: "viejo"},
		{Role: "user", Content: "hola"},
	}

	got := a.refreshSystemPrompt(messages)
	if got[0].Content == "viejo" {
		t.Error("system prompt was not refreshed")
	}
	if !strings.Contains(got[0].Content, time.Now().Format("2006-01-02")) {
		t.Errorf("refreshed prompt does not carry today's date:\n%s", got[0].Content)
	}
	if len(got) != 2 || got[1].Content != "hola" {
		t.Errorf("the conversation was altered: %+v", got)
	}
}

func TestRefreshSystemPromptPrependsWhenMissing(t *testing.T) {
	a := testApp()
	got := a.refreshSystemPrompt([]deepseek.Message{{Role: "user", Content: "hola"}})
	if len(got) != 2 || got[0].Role != "system" {
		t.Fatalf("expected a system message first, got %+v", got)
	}
}

// An aborted batch must leave one tool message per call, or the next request is
// rejected by the API.
func TestAppendAbortedResultsCompletesTheBatch(t *testing.T) {
	pending := []deepseek.ToolCall{
		{ID: "a"}, {ID: "b"},
	}
	messages := appendAbortedResults(nil, pending, "no ejecutado")
	if len(messages) != 2 {
		t.Fatalf("got %d results, want 2", len(messages))
	}
	for i, m := range messages {
		if m.Role != "tool" {
			t.Errorf("message %d role = %q, want tool", i, m.Role)
		}
		if m.ToolCallID != pending[i].ID {
			t.Errorf("message %d id = %q, want %q", i, m.ToolCallID, pending[i].ID)
		}
		if m.Content != "no ejecutado" {
			t.Errorf("message %d content = %q", i, m.Content)
		}
	}
}

func TestProjectedTokens(t *testing.T) {
	a := testApp()
	if got := a.projectedTokens(); got != 0 {
		t.Errorf("projectedTokens() = %d before any request, want 0", got)
	}
	a.addUsage(deepseek.Usage{PromptTokens: 1000, CompletionTokens: 200, TotalTokens: 1200})
	if got := a.projectedTokens(); got != 1200 {
		t.Errorf("projectedTokens() = %d, want 1200 (prompt + answer)", got)
	}
	if !strings.Contains(a.tokensString(), "1 200") {
		t.Errorf("/tokens does not show the next request:\n%s", a.tokensString())
	}
}

func TestJSONEventShape(t *testing.T) {
	// The event struct must serialise the fields a script reads.
	ev := jsonEvent{Type: "usage", PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data := string(raw)
	for _, want := range []string{`"type":"usage"`, `"prompt_tokens":10`, `"total_tokens":12`} {
		if !strings.Contains(data, want) {
			t.Errorf("event JSON %s is missing %s", data, want)
		}
	}
	if strings.Contains(data, "model") {
		t.Errorf("empty fields should be omitted: %s", data)
	}
}
