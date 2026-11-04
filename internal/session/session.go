// Package session stores a conversation as an append-only JSONL log.
//
// The log is the source of truth: the model history is derived from it on every
// turn instead of living in a slice that dies with the process. That is what
// makes resuming, auditing and (later) compacting possible without destroying
// what was said: a compaction hides a span of events, it never deletes them.
package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/todo"
)

// Event types written to the log.
const (
	// EventStart is the first line: session id, working directory and model.
	EventStart = "session/start"
	// EventUser and EventAssistant are the conversation itself.
	EventUser      = "user/message"
	EventAssistant = "assistant/message"
	// EventToolResult completes one tool call of the preceding assistant turn.
	EventToolResult = "tool/result"
	// EventCompaction replaces a span of events with a summary.
	EventCompaction = "compaction"
	// EventPrune shortens the tool results inside a span without summarising it:
	// the cheap compaction that runs before paying for a summary.
	EventPrune = "prune"
	// EventTodos records the task list the model is working through.
	EventTodos = "todo/list"
	// EventTurnEnd closes a turn, so an interrupted one can be detected.
	EventTurnEnd = "turn/end"
)

// ErrNotFound is returned when a session id does not exist.
var ErrNotFound = errors.New("session not found")

// Event is one durable fact in the log.
type Event struct {
	Seq  int64           `json:"seq"`
	Time time.Time       `json:"time"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type startData struct {
	ID      string    `json:"id"`
	CWD     string    `json:"cwd"`
	Model   string    `json:"model"`
	Created time.Time `json:"created"`
}

type messageData struct {
	Content string `json:"content"`
}

type assistantData struct {
	Content   string              `json:"content"`
	ToolCalls []deepseek.ToolCall `json:"tool_calls,omitempty"`
	Usage     *deepseek.Usage     `json:"usage,omitempty"`
}

type toolResultData struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

type compactionData struct {
	Summary string `json:"summary"`
	FromSeq int64  `json:"from_seq"`
	ToSeq   int64  `json:"to_seq"`
	Tokens  int    `json:"tokens,omitempty"`
}

type pruneData struct {
	FromSeq int64 `json:"from_seq"`
	ToSeq   int64 `json:"to_seq"`
	Saved   int   `json:"saved,omitempty"`
}

type todosData struct {
	Items []todo.Item `json:"items"`
}

type turnEndData struct {
	Reason string `json:"reason"`
}

// Meta describes a new session.
type Meta struct {
	CWD   string
	Model string
}

// Info summarizes a stored session for listings.
type Info struct {
	ID      string
	Title   string
	Created time.Time
	Updated time.Time
	Turns   int
	Events  int
	Path    string
}

// Session is an append-only conversation log.
type Session struct {
	mu     sync.Mutex
	id     string
	path   string
	file   *os.File
	events []Event
	meta   startData
}

// Dir returns the directory that holds the session logs.
func Dir() string {
	return filepath.Join(userDataDir(), "deekseep-coder", "sessions")
}

func userDataDir() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(home, ".local", "share")
}

// Create starts a new session in dir.
func Create(dir string, meta Meta) (*Session, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	id := newID()
	s := &Session{id: id, path: filepath.Join(dir, id+".jsonl")}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s.file = f
	s.meta = startData{ID: id, CWD: meta.CWD, Model: meta.Model, Created: time.Now()}
	if err := s.append(EventStart, s.meta); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// Open loads an existing session and leaves it ready to append.
//
// An interrupted session is repaired on open: missing tool results are recorded
// and the dangling turn is closed, so the derived history is always valid.
func Open(dir, id string) (*Session, error) {
	path := filepath.Join(dir, id+".jsonl")
	events, meta, err := load(path)
	if err != nil {
		return nil, err
	}
	s := &Session{id: id, path: path, events: events, meta: meta}
	// The file must be open before repairing: the synthetic events have to be
	// persisted, not just kept in memory.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s.file = f
	if err := s.repair(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// Load reads a session without opening it for writing, so it never modifies the
// log. Use it for read-only views such as `history show`.
func Load(dir, id string) (*Session, error) {
	path := filepath.Join(dir, id+".jsonl")
	events, meta, err := load(path)
	if err != nil {
		return nil, err
	}
	return &Session{id: id, path: path, events: events, meta: meta}, nil
}

// load reads every event of a log file.
func load(path string) ([]Event, startData, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, startData{}, ErrNotFound
		}
		return nil, startData{}, err
	}
	defer f.Close()

	var events []Event
	var meta startData
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// A torn last line from a crash is expected; skip it.
			continue
		}
		if ev.Type == EventStart {
			_ = json.Unmarshal(ev.Data, &meta)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, startData{}, err
	}
	return events, meta, nil
}

// repair closes what an interrupted process left open.
func (s *Session) repair() error {
	// One tool result per pending call, so the history stays valid.
	if err := s.repairToolCalls(); err != nil {
		return err
	}
	// A turn that never ended: record it as interrupted.
	if s.openTurn() {
		if err := s.AppendTurnEnd("interrupted"); err != nil {
			return err
		}
	}
	return nil
}

// repairToolCalls appends a synthetic result for every call that never got one.
func (s *Session) repairToolCalls() error {
	answered := make(map[string]bool)
	for _, ev := range s.events {
		if ev.Type == EventToolResult {
			var d toolResultData
			if json.Unmarshal(ev.Data, &d) == nil {
				answered[d.ToolCallID] = true
			}
		}
	}
	for _, ev := range s.events {
		if ev.Type != EventAssistant {
			continue
		}
		var d assistantData
		if json.Unmarshal(ev.Data, &d) != nil {
			continue
		}
		for _, call := range d.ToolCalls {
			if call.ID == "" || answered[call.ID] {
				continue
			}
			answered[call.ID] = true
			if err := s.AppendToolResult(call.ID, "Not executed: the conversation was interrupted.", true); err != nil {
				return err
			}
		}
	}
	return nil
}

// openTurn reports whether events exist after the last turn/end.
func (s *Session) openTurn() bool {
	for i := len(s.events) - 1; i >= 0; i-- {
		switch s.events[i].Type {
		case EventTurnEnd:
			return false
		case EventStart:
			return false
		default:
			return true
		}
	}
	return false
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// Path returns the log file path.
func (s *Session) Path() string { return s.path }

// Meta returns the session header.
func (s *Session) Meta() (id, cwd, model string) {
	return s.meta.ID, s.meta.CWD, s.meta.Model
}

// append writes one event and keeps it in memory.
func (s *Session) append(kind string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(kind, data)
}

func (s *Session) appendLocked(kind string, data json.RawMessage) error {
	seq := int64(len(s.events))
	ev := Event{Seq: seq, Time: time.Now(), Type: kind, Data: data}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if s.file != nil {
		if _, err := s.file.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	s.events = append(s.events, ev)
	return nil
}

// AppendUser records a user message.
func (s *Session) AppendUser(content string) error {
	return s.append(EventUser, messageData{Content: content})
}

// AppendAssistant records a model answer with its tool calls.
func (s *Session) AppendAssistant(content string, calls []deepseek.ToolCall, usage *deepseek.Usage) error {
	return s.append(EventAssistant, assistantData{Content: content, ToolCalls: calls, Usage: usage})
}

// AppendToolResult records the outcome of one tool call.
func (s *Session) AppendToolResult(callID, content string, isError bool) error {
	return s.append(EventToolResult, toolResultData{ToolCallID: callID, Content: content, IsError: isError})
}

// AppendCompaction records a summary that hides a span of events.
func (s *Session) AppendCompaction(summary string, fromSeq, toSeq int64, tokens int) error {
	return s.append(EventCompaction, compactionData{Summary: summary, FromSeq: fromSeq, ToSeq: toSeq, Tokens: tokens})
}

// AppendPrune records that the tool results inside a span were shortened.
func (s *Session) AppendPrune(fromSeq, toSeq int64, saved int) error {
	return s.append(EventPrune, pruneData{FromSeq: fromSeq, ToSeq: toSeq, Saved: saved})
}

// AppendTodos records the current task list.
func (s *Session) AppendTodos(items []todo.Item) error {
	if items == nil {
		items = []todo.Item{}
	}
	return s.append(EventTodos, todosData{Items: items})
}

// Todos returns the most recent task list, or nil when none was ever written.
func (s *Session) Todos() todo.List {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		ev := s.events[i]
		if ev.Type != EventTodos {
			continue
		}
		var d todosData
		if json.Unmarshal(ev.Data, &d) == nil {
			return d.Items
		}
	}
	return nil
}

// AppendTurnEnd closes the current turn and flushes the log.
func (s *Session) AppendTurnEnd(reason string) error {
	if err := s.append(EventTurnEnd, turnEndData{Reason: reason}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Sync()
	}
	return nil
}

// Snapshot is the model history plus what the token meter needs.
type Snapshot struct {
	// Messages is what the next request would send, without the system prompt.
	Messages []deepseek.Message
	// Seqs is the log sequence of each message, so a compaction can address a
	// span by message index.
	Seqs []int64
	// AnchorUsage is the provider-reported usage of the last answered request.
	AnchorUsage *deepseek.Usage
	// AnchorLen is how many messages that request already contained.
	AnchorLen int
}

// Snapshot derives the history together with the measurement anchor.
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs, seqs, usage, anchorLen := s.deriveLocked()
	return Snapshot{Messages: msgs, Seqs: seqs, AnchorUsage: usage, AnchorLen: anchorLen}
}

// Messages derives the model history from the log.
//
// Events hidden by a compaction are skipped and replaced by the summary at the
// position where the hidden span started, so the conversation keeps its order.
func (s *Session) Messages() []deepseek.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs, _, _, _ := s.deriveLocked()
	return msgs
}

func (s *Session) messagesLocked() []deepseek.Message {
	msgs, _, _, _ := s.deriveLocked()
	return msgs
}

// deriveLocked folds the log into the message list plus the meter anchor.
func (s *Session) deriveLocked() ([]deepseek.Message, []int64, *deepseek.Usage, int) {
	summaryAt := make(map[int64]string)
	shadowed := make(map[int64]bool)
	for _, ev := range s.events {
		if ev.Type != EventCompaction {
			continue
		}
		var d compactionData
		if json.Unmarshal(ev.Data, &d) != nil {
			continue
		}
		summaryAt[d.FromSeq] = d.Summary
		for seq := d.FromSeq; seq <= d.ToSeq; seq++ {
			shadowed[seq] = true
		}
	}

	pruned := make(map[int64]bool)
	for _, ev := range s.events {
		if ev.Type != EventPrune {
			continue
		}
		var d pruneData
		if json.Unmarshal(ev.Data, &d) != nil {
			continue
		}
		for seq := d.FromSeq; seq <= d.ToSeq; seq++ {
			pruned[seq] = true
		}
	}

	var out []deepseek.Message
	var seqs []int64
	var anchor *deepseek.Usage
	anchorLen := 0
	emit := func(m deepseek.Message, seq int64) {
		out = append(out, m)
		seqs = append(seqs, seq)
	}

	for _, ev := range s.events {
		if summary, ok := summaryAt[ev.Seq]; ok {
			emit(deepseek.Message{Role: "user", Content: summary}, ev.Seq)
			continue
		}
		if shadowed[ev.Seq] {
			continue
		}
		switch ev.Type {
		case EventUser:
			var d messageData
			if json.Unmarshal(ev.Data, &d) == nil {
				emit(deepseek.Message{Role: "user", Content: d.Content}, ev.Seq)
			}
		case EventAssistant:
			var d assistantData
			if json.Unmarshal(ev.Data, &d) != nil {
				continue
			}
			if d.Content == "" && len(d.ToolCalls) == 0 {
				continue // nothing model-visible
			}
			emit(deepseek.Message{Role: "assistant", Content: d.Content, ToolCalls: d.ToolCalls}, ev.Seq)
			if d.Usage != nil {
				anchor = d.Usage
				anchorLen = len(out)
			}
		case EventToolResult:
			var d toolResultData
			if json.Unmarshal(ev.Data, &d) == nil {
				content := d.Content
				if pruned[ev.Seq] {
					content = ShortenToolResult(content)
				}
				emit(deepseek.Message{Role: "tool", ToolCallID: d.ToolCallID, Content: content}, ev.Seq)
			}
		case EventCompaction, EventPrune:
			// The reported usage no longer describes the surface, so the meter
			// must fall back to estimating until the next answered request.
			anchor = nil
			anchorLen = 0
		}
	}
	return out, seqs, anchor, anchorLen
}

// LastUsage returns the usage of the most recent assistant event.
func (s *Session) LastUsage() (deepseek.Usage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		ev := s.events[i]
		if ev.Type != EventAssistant {
			continue
		}
		var d assistantData
		if json.Unmarshal(ev.Data, &d) == nil && d.Usage != nil {
			return *d.Usage, true
		}
	}
	return deepseek.Usage{}, false
}

// Close flushes and closes the log.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	if err := s.file.Sync(); err != nil {
		s.file.Close()
		return err
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// List returns the sessions in dir, newest first.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		path := filepath.Join(dir, e.Name())
		events, meta, err := load(path)
		if err != nil {
			continue
		}
		info := Info{ID: id, Path: path, Created: meta.Created, Events: len(events)}
		if fi, err := e.Info(); err == nil {
			info.Updated = fi.ModTime()
		}
		for _, ev := range events {
			switch ev.Type {
			case EventUser:
				var d messageData
				if info.Title == "" && json.Unmarshal(ev.Data, &d) == nil {
					info.Title = shortTitle(d.Content)
				}
			case EventTurnEnd:
				info.Turns++
			}
		}
		if info.Title == "" {
			info.Title = "(sin mensajes)"
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

// Latest returns the id of the most recently updated session.
func Latest(dir string) (string, error) {
	list, err := List(dir)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", ErrNotFound
	}
	return list[0].ID, nil
}

// Delete removes a session log.
func Delete(dir, id string) error {
	path := filepath.Join(dir, id+".jsonl")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// shortTitle trims a first message into a listing title.
func shortTitle(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len([]rune(s)) > 60 {
		return string([]rune(s)[:60]) + "…"
	}
	return s
}

// newID builds a sortable, unique session id.
func newID() string {
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano()%1000)
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(buf[:])
}

// pruneKeep is how many bytes are kept at each end of a pruned tool output.
const pruneKeep = 200

// ShortenToolResult keeps the head and the tail of a tool output and replaces
// the middle with a marker.
//
// The head says what the command printed and the tail usually holds the error,
// so a pruned result still tells the model what happened without costing the
// full transcript.
func ShortenToolResult(s string) string {
	if len(s) <= pruneKeep*2+64 {
		return s
	}
	head := trimRuneBoundary(s[:pruneKeep], false)
	tail := trimRuneBoundary(s[len(s)-pruneKeep:], true)
	omitted := len(s) - len(head) - len(tail)
	return head + fmt.Sprintf("\n[…tool output pruned: %d bytes omitted…]\n", omitted) + tail
}

// trimRuneBoundary cuts b at a valid UTF-8 boundary.
func trimRuneBoundary(b string, fromStart bool) string {
	if fromStart {
		for len(b) > 0 && b[0]&0xC0 == 0x80 {
			b = b[1:]
		}
		return b
	}
	for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 {
		b = b[:len(b)-1]
	}
	return b
}
