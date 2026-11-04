// Package compact decides which part of a conversation to replace with a
// summary, and builds the request that produces that summary.
//
// Compaction never deletes anything: the caller records a compaction event that
// hides a span of the log, so the transcript stays auditable.
package compact

import (
	"fmt"
	"strings"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/tokens"
)

// Options are the thresholds that decide when and how much to compact.
type Options struct {
	// Threshold is the fraction of the context window that triggers compaction.
	Threshold float64
	// Headroom is the room reserved for the answer, in tokens.
	Headroom int
	// Retain is the fraction of the window kept verbatim.
	Retain float64
	// KeepRecent is the minimum number of messages kept, whatever the budget.
	KeepRecent int
}

// Default is the shipped configuration: compact at 80 % of the window (or at
// window minus headroom, whichever is lower) and keep the most recent 16 %.
var Default = Options{Threshold: 0.8, Headroom: 8192, Retain: 0.16, KeepRecent: 2}

// Trigger returns the token count at which compaction starts.
func (o Options) Trigger(window int) int {
	if window <= 0 {
		return 0
	}
	byRatio := int(float64(window) * o.Threshold)
	byHeadroom := window - o.Headroom
	if byHeadroom < byRatio {
		return byHeadroom
	}
	return byRatio
}

// Needed reports whether the current pressure has reached the trigger.
func Needed(used, window int, opts Options) bool {
	if window <= 0 {
		return false
	}
	trigger := opts.Trigger(window)
	return trigger > 0 && used >= trigger
}

// MinPruneTokens is the size below which shortening a tool result is not worth
// the marker left behind.
const MinPruneTokens = 100

// Prunable reports whether a message is a tool result big enough to shorten.
//
// Shortening is free (no model call) and often enough on its own: command output
// is what makes a transcript grow, and most of an old output is never read again.
func Prunable(m deepseek.Message) bool {
	return m.Role == "tool" && tokens.Text(m.Content) > MinPruneTokens
}

// Span is the part of the conversation a compaction replaces.
type Span struct {
	// From and To are inclusive message indexes.
	From, To int
	// Tokens is the estimated size of the replaced messages.
	Tokens int
	// Messages is the replaced content.
	Messages []deepseek.Message
}

// Choose picks the oldest messages to replace.
//
// It keeps a suffix of the conversation verbatim, sized by the retain fraction,
// and never starts that suffix with a tool result: a tool message whose
// assistant call was hidden would make the history invalid.
func Choose(msgs []deepseek.Message, window int, opts Options) (Span, bool) {
	if len(msgs) < 2 || window <= 0 {
		return Span{}, false
	}
	budget := int(float64(window) * opts.Retain)
	if budget < 1 {
		budget = 1
	}

	// Walk backwards accumulating what to keep.
	cut := len(msgs)
	kept := 0
	for cut > 0 && kept < budget {
		cut--
		kept += tokens.Message(msgs[cut])
	}
	// Keep at least KeepRecent messages.
	if min := len(msgs) - opts.KeepRecent; cut > min {
		cut = min
	}
	// A tool result must not open the kept region.
	for cut < len(msgs) && msgs[cut].Role == "tool" {
		cut++
	}
	if cut <= 0 || cut >= len(msgs) {
		return Span{}, false
	}

	span := msgs[:cut]
	total := tokens.Messages(span)
	if total <= 0 {
		return Span{}, false
	}
	return Span{From: 0, To: cut - 1, Tokens: total, Messages: span}, true
}

// summarySystem is the instruction for the summarising call.
const summarySystem = `You compress a conversation so another assistant can continue it.
Write a dense handoff summary in the user's language. Keep:
- what the user wants, including constraints and preferences;
- decisions taken and why, and options already discarded;
- files, paths and symbols touched, with what changed;
- commands run and their relevant output, including errors and how they were fixed;
- what is still pending or unverified.
Do not invent anything. Do not greet. Use short bullets.`

// SummaryPrompt builds the request that produces the summary of a span.
//
// The transcript is rendered as plain text with a bounded size: if the span is
// huge, the middle is dropped, because the beginning and the end carry the task
// and the latest state.
func SummaryPrompt(span []deepseek.Message) []deepseek.Message {
	return []deepseek.Message{
		{Role: "system", Content: summarySystem},
		{Role: "user", Content: renderTranscript(span)},
	}
}

// maxTranscript bounds the text sent to the summarising call.
const maxTranscript = 120 * 1024

// renderTranscript writes a span as readable text.
func renderTranscript(span []deepseek.Message) string {
	var b strings.Builder
	b.WriteString("Conversation to compress:\n\n")
	for _, m := range span {
		switch m.Role {
		case "user":
			fmt.Fprintf(&b, "USER:\n%s\n\n", m.Content)
		case "assistant":
			if m.Content != "" {
				fmt.Fprintf(&b, "ASSISTANT:\n%s\n", m.Content)
			}
			for _, call := range m.ToolCalls {
				fmt.Fprintf(&b, "TOOL CALL %s(%s):\n%s\n", call.Function.Name, call.ID, call.Function.Arguments)
			}
			b.WriteString("\n")
		case "tool":
			fmt.Fprintf(&b, "TOOL RESULT %s:\n%s\n\n", m.ToolCallID, m.Content)
		default:
			fmt.Fprintf(&b, "%s:\n%s\n\n", strings.ToUpper(m.Role), m.Content)
		}
	}
	out := b.String()
	if len(out) <= maxTranscript {
		return out
	}
	// Keep the head and the tail: the task is at the start, the state at the end.
	head := maxTranscript / 3
	tail := maxTranscript - head
	cut := head
	for cut > 0 && out[cut]&0xC0 == 0x80 {
		cut--
	}
	start := len(out) - tail
	for start < len(out) && out[start]&0xC0 == 0x80 {
		start++
	}
	return out[:cut] + "\n\n[…middle of the conversation omitted…]\n\n" + out[start:]
}
