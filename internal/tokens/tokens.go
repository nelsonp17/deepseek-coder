// Package tokens estimates how many tokens a conversation costs.
//
// The API reports exact usage for the requests it answered, but the CLI also
// needs a number for what has been added *since* that answer: that is the
// request the user is about to pay for. Estimates are anchored to the reported
// value whenever one exists, and only the delta is estimated.
package tokens

import "deekseep-coder/internal/deepseek"

// BytesPerToken is the divisor used by the estimator.
//
// It is calibrated against measurements of this project's own prompts: 12,4 KB
// of mixed Spanish, Markdown and tool schemas was reported by the API as 4 151
// tokens, about 3 bytes per token. Plain English prose would be closer to 4.
const BytesPerToken = 3

// roleOverhead covers the framing the API adds per message.
const roleOverhead = 4

// Text estimates the tokens of a string.
func Text(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / BytesPerToken
	if n == 0 {
		n = 1
	}
	return n
}

// Message estimates the tokens of one message.
func Message(m deepseek.Message) int {
	n := roleOverhead + Text(m.Content)
	for _, call := range m.ToolCalls {
		n += roleOverhead + Text(call.Function.Name) + Text(call.Function.Arguments)
	}
	return n
}

// Messages estimates the tokens of a message list.
func Messages(msgs []deepseek.Message) int {
	total := 0
	for _, m := range msgs {
		total += Message(m)
	}
	return total
}

// Pressure estimates what the next request will cost.
//
// anchor is the usage reported for the last answered request and anchorLen how
// many messages that request already contained. When there is an anchor only the
// messages added after it are estimated, so the number stays tied to the
// provider's own count instead of drifting with a heuristic.
func Pressure(msgs []deepseek.Message, anchor *deepseek.Usage, anchorLen int) int {
	if anchor == nil || anchor.PromptTokens <= 0 {
		return Messages(msgs)
	}
	used := anchor.PromptTokens + anchor.CompletionTokens
	if anchorLen < 0 || anchorLen > len(msgs) {
		return used
	}
	return used + Messages(msgs[anchorLen:])
}
