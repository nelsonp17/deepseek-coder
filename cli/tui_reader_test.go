package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"deepseek-coder/internal/deepseek"
)

// newReaderModel builds a TUI in the middle of a turn, with a live chat channel.
func newReaderModel(t *testing.T) (tuiModel, *app) {
	t.Helper()
	a := testApp()
	m := newTUIModel(a, nil, "deepseek-chat")
	m.chatCh = make(chan tea.Msg, 8)
	m.streaming = true
	m.reading = true
	return m, a
}

// Every message from the chat goroutine must leave a reader armed, or the turn
// hangs with the answer already on screen. That is exactly the bug this covers:
// usageMsg used to arrive and kill the chain, leaving streamDoneMsg unread.
func TestReaderIsRearmedAfterEachChatMessage(t *testing.T) {
	m, _ := newReaderModel(t)

	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"usage", usageMsg{usage: deepseek.Usage{PromptTokens: 10, CompletionTokens: 2}}},
		{"info", infoMsg{text: "Ejecutando read_file..."}},
		{"reasoning", reasoningMsg{text: "pensando"}},
		{"tool request", toolRequestMsg{tc: toolCall(toolRead, "{}"), ch: make(chan bool)}},
		{"question", questionRequestMsg{question: "¿?", options: []string{"a"}, ch: make(chan []string)}},
	}
	for _, c := range cases {
		next, _ := m.Update(chatMsg{inner: c.msg})
		got := next.(tuiModel)
		if !got.reading {
			t.Errorf("%s: the reader was not re-armed; the turn would hang", c.name)
		}
		if !got.streaming {
			t.Errorf("%s: streaming was cleared too early", c.name)
		}
		m = got
	}
}

// The final message must close the turn and leave no reader behind.
func TestStreamDoneEndsTheTurn(t *testing.T) {
	m, _ := newReaderModel(t)
	next, _ := m.Update(chatMsg{inner: streamDoneMsg{content: "hola"}})
	got := next.(tuiModel)
	if got.streaming {
		t.Error("streaming is still true after streamDoneMsg")
	}
	if got.reading {
		t.Error("a reader is still armed after the turn ended")
	}
}

// A closed channel must not leave a reader looping.
func TestChatClosedClearsTheChannel(t *testing.T) {
	m, _ := newReaderModel(t)
	next, _ := m.Update(chatClosedMsg{})
	got := next.(tuiModel)
	if got.chatCh != nil {
		t.Error("chatCh was not cleared")
	}
	if got.reading {
		t.Error("a reader is still armed on a closed channel")
	}
}

// The whole sequence, exactly as the goroutine emits it, must end with the turn
// closed. This is the end-to-end shape of the hang.
func TestFullChatSequenceEndsTheTurn(t *testing.T) {
	m, _ := newReaderModel(t)
	sequence := []tea.Msg{
		streamChunkMsg{text: "¡Hola!"},
		usageMsg{usage: deepseek.Usage{PromptTokens: 100, CompletionTokens: 5}},
		streamDoneMsg{content: "¡Hola!"},
	}
	for _, msg := range sequence {
		next, _ := m.Update(chatMsg{inner: msg})
		m = next.(tuiModel)
	}
	if m.streaming {
		t.Fatal("the turn never finished: this is the 'pensando...' hang")
	}
	if m.reading {
		t.Error("a reader is still armed after the turn")
	}
}

// Answering a tool prompt must re-arm the reader: the chat goroutine resumes
// right after and its messages have nowhere to go otherwise.
func TestApprovingAToolRearmsTheReader(t *testing.T) {
	m, _ := newReaderModel(t)
	m.mode = modeConfirmTool
	m.permCh = make(chan bool, 1)
	m.pendingTool = toolRead
	m.textarea.SetValue("y")
	// The reader that delivered the prompt was consumed: nothing is armed now.
	m.reading = false

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(tuiModel)
	if !got.reading {
		t.Error("no reader was armed after answering a tool prompt")
	}
	select {
	case allowed := <-m.permCh:
		if !allowed {
			t.Error("the answer was not delivered")
		}
	default:
		t.Error("the answer never reached the chat goroutine")
	}
}
