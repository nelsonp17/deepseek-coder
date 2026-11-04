package compact

import (
	"strings"
	"testing"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/tokens"
)

func msg(role, content string) deepseek.Message {
	return deepseek.Message{Role: role, Content: content}
}

func TestPrunable(t *testing.T) {
	if !Prunable(deepseek.Message{Role: "tool", Content: strings.Repeat("x", 1000)}) {
		t.Error("a large tool result should be prunable")
	}
	if Prunable(deepseek.Message{Role: "tool", Content: "ok"}) {
		t.Error("a small tool result is not worth pruning")
	}
	if Prunable(deepseek.Message{Role: "user", Content: strings.Repeat("x", 1000)}) {
		t.Error("only tool results are prunable")
	}
}
func TestTrigger(t *testing.T) {
	opts := Default
	// 80 % of 100 000 is 80 000, and window minus headroom is 91 808: the lower
	// one wins.
	if got := opts.Trigger(100000); got != 80000 {
		t.Errorf("Trigger(100000) = %d, want 80000", got)
	}
	// With a small window the headroom dominates.
	if got := opts.Trigger(10000); got != 1808 {
		t.Errorf("Trigger(10000) = %d, want 1808 (window minus headroom)", got)
	}
	if got := opts.Trigger(0); got != 0 {
		t.Errorf("Trigger(0) = %d, want 0", got)
	}
}

func TestNeeded(t *testing.T) {
	if Needed(0, 0, Default) {
		t.Error("an unknown window must never trigger compaction")
	}
	if Needed(79999, 100000, Default) {
		t.Error("below the trigger should not compact")
	}
	if !Needed(80000, 100000, Default) {
		t.Error("at the trigger it should compact")
	}
}

func TestChooseKeepsTheRecentSuffix(t *testing.T) {
	var msgs []deepseek.Message
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("user", strings.Repeat("u", 3000)))      // ~1000 tokens
		msgs = append(msgs, msg("assistant", strings.Repeat("a", 3000))) // ~1000 tokens
	}
	window := 20000 // retain 16 % = 3200 tokens ≈ 3 messages
	span, ok := Choose(msgs, window, Default)
	if !ok {
		t.Fatal("expected a span to compact")
	}
	if span.From != 0 {
		t.Errorf("From = %d, want 0", span.From)
	}
	if span.To >= len(msgs)-1 {
		t.Errorf("To = %d, the whole conversation must not be compacted", span.To)
	}
	if kept := len(msgs) - (span.To + 1); kept < Default.KeepRecent {
		t.Errorf("kept %d messages, want at least %d", kept, Default.KeepRecent)
	}
	if span.Tokens <= 0 {
		t.Errorf("Tokens = %d", span.Tokens)
	}
}

// The kept suffix must not open with a tool result: its assistant call would be
// hidden and the history would become invalid.
func TestChooseNeverStartsOnAToolResult(t *testing.T) {
	call := deepseek.ToolCall{ID: "c1"}
	call.Function.Name = "read_file"
	msgs := []deepseek.Message{
		msg("user", strings.Repeat("u", 3000)),
		{Role: "assistant", ToolCalls: []deepseek.ToolCall{call}},
		msg("tool", strings.Repeat("t", 3000)),
		msg("user", strings.Repeat("u", 3000)),
	}
	span, ok := Choose(msgs, 9000, Default) // retain 1440 tokens ≈ 1 message
	if !ok {
		t.Fatal("expected a span")
	}
	keptStart := msgs[span.To+1]
	if keptStart.Role == "tool" {
		t.Errorf("kept region starts with an orphan tool result: %+v", keptStart)
	}
}

func TestChooseBailsOutWhenThereIsNothingToDo(t *testing.T) {
	if _, ok := Choose(nil, 10000, Default); ok {
		t.Error("empty conversation must not compact")
	}
	if _, ok := Choose([]deepseek.Message{msg("user", "hola")}, 10000, Default); ok {
		t.Error("a single message must not compact")
	}
	if _, ok := Choose([]deepseek.Message{msg("user", "a"), msg("assistant", "b")}, 0, Default); ok {
		t.Error("an unknown window must not compact")
	}
}

func TestSummaryPromptRendersBothSides(t *testing.T) {
	call := deepseek.ToolCall{ID: "c1"}
	call.Function.Name = "read_file"
	call.Function.Arguments = `{"path":"a.go"}`
	span := []deepseek.Message{
		msg("user", "arregla el bug"),
		{Role: "assistant", ToolCalls: []deepseek.ToolCall{call}},
		msg("tool", "contenido del archivo"),
		msg("assistant", "ya está"),
	}
	prompt := SummaryPrompt(span)
	if len(prompt) != 2 {
		t.Fatalf("got %d messages, want 2", len(prompt))
	}
	if prompt[0].Role != "system" || !strings.Contains(prompt[0].Content, "handoff") {
		t.Errorf("system prompt = %q", prompt[0].Content)
	}
	body := prompt[1].Content
	for _, want := range []string{"arregla el bug", "read_file", "contenido del archivo", "ya está"} {
		if !strings.Contains(body, want) {
			t.Errorf("transcript is missing %q:\n%s", want, body)
		}
	}
}

// A huge span must be cut in the middle, keeping the task and the latest state.
func TestSummaryPromptBoundsAHugeTranscript(t *testing.T) {
	span := []deepseek.Message{
		msg("user", "PRINCIPIO "+strings.Repeat("x", 200000)),
		msg("assistant", strings.Repeat("y", 200000)+" FINAL"),
	}
	body := SummaryPrompt(span)[1].Content
	if len(body) > maxTranscript+200 {
		t.Errorf("transcript is %d bytes, want it bounded to ~%d", len(body), maxTranscript)
	}
	if !strings.Contains(body, "PRINCIPIO") {
		t.Error("the head of the span was lost")
	}
	if !strings.Contains(body, "FINAL") {
		t.Error("the tail of the span was lost")
	}
	if !strings.Contains(body, "omitted") {
		t.Error("the cut is not announced")
	}
}

func TestSpanTokensMatchTheEstimate(t *testing.T) {
	var msgs []deepseek.Message
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("user", strings.Repeat("u", 300)))
		msgs = append(msgs, msg("assistant", strings.Repeat("a", 300)))
	}
	span, ok := Choose(msgs, 3000, Default)
	if !ok {
		t.Fatal("expected a span")
	}
	if got, want := span.Tokens, tokens.Messages(span.Messages); got != want {
		t.Errorf("span.Tokens = %d, want %d", got, want)
	}
}
