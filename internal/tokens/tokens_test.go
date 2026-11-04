package tokens

import (
	"strings"
	"testing"

	"deepseek-coder/internal/deepseek"
)

func TestTextAndMessages(t *testing.T) {
	if got := Text(""); got != 0 {
		t.Errorf("Text(\"\") = %d, want 0", got)
	}
	if got := Text("abc"); got != 1 {
		t.Errorf("Text(\"abc\") = %d, want 1 (never zero for non-empty)", got)
	}
	long := strings.Repeat("x", 3000)
	if got := Text(long); got != 1000 {
		t.Errorf("Text(3000 bytes) = %d, want 1000", got)
	}

	msgs := []deepseek.Message{
		{Role: "user", Content: long},
		{Role: "assistant", Content: long},
	}
	total := Messages(msgs)
	if total <= 2000 {
		t.Errorf("Messages = %d, want more than the sum of the two texts", total)
	}
	// Tool calls add their name and arguments to the cost.
	call := deepseek.ToolCall{}
	call.Function.Name = "read_file"
	call.Function.Arguments = strings.Repeat("a", 300)
	withCall := Messages([]deepseek.Message{{Role: "assistant", ToolCalls: []deepseek.ToolCall{call}}})
	if withCall <= roleOverhead {
		t.Errorf("tool call cost = %d, want it to include name and arguments", withCall)
	}
}

func TestPressureWithoutAnchorEstimates(t *testing.T) {
	msgs := []deepseek.Message{{Role: "user", Content: strings.Repeat("x", 300)}}
	if got, want := Pressure(msgs, nil, 0), Messages(msgs); got != want {
		t.Errorf("Pressure = %d, want the estimate %d", got, want)
	}
}

func TestPressureAnchorsToReportedUsage(t *testing.T) {
	// The request had 1000 prompt tokens and produced 100; 900 bytes were added
	// afterwards (300 tokens). The estimate must be anchored, not recomputed.
	anchor := &deepseek.Usage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100}
	msgs := []deepseek.Message{
		{Role: "assistant", Content: "respuesta"}, // already counted by the anchor
		{Role: "user", Content: strings.Repeat("x", 900)},
	}
	got := Pressure(msgs, anchor, 1)
	// The delta is priced like any other message, framing included.
	if want := 1100 + Messages(msgs[1:]); got != want {
		t.Errorf("Pressure = %d, want %d (anchor plus delta)", got, want)
	}
	if got <= 1100 {
		t.Errorf("Pressure = %d, the delta was not added to the anchor", got)
	}
}

func TestPressureClampsABadAnchorLength(t *testing.T) {
	anchor := &deepseek.Usage{PromptTokens: 500, CompletionTokens: 50}
	msgs := []deepseek.Message{{Role: "user", Content: "hola"}}
	if got := Pressure(msgs, anchor, 99); got != 550 {
		t.Errorf("Pressure = %d, want the anchor alone %d", got, 550)
	}
}
