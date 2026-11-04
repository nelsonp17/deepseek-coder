package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepseek-coder/internal/deepseek"
)

func newSession(t *testing.T) (*Session, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Create(dir, Meta{CWD: dir, Model: "deepseek-chat"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestCreateWritesAHeader(t *testing.T) {
	s, dir := newSession(t)
	if s.ID() == "" {
		t.Fatal("empty session id")
	}
	data, err := os.ReadFile(filepath.Join(dir, s.ID()+".jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), EventStart) {
		t.Errorf("log has no header:\n%s", string(data))
	}
}

func TestMessagesRoundTrip(t *testing.T) {
	s, dir := newSession(t)
	call := deepseek.ToolCall{ID: "c1", Type: "function"}
	call.Function.Name = "read_file"
	call.Function.Arguments = `{"path":"a.txt"}`

	if err := s.AppendUser("hola"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAssistant("", []deepseek.ToolCall{call}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendToolResult("c1", "contenido", false); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAssistant("listo", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTurnEnd("completed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must derive exactly the same history.
	reopened, err := Open(dir, s.ID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reopened.Close()

	msgs := reopened.Messages()
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hola" {
		t.Errorf("first message = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "c1" {
		t.Errorf("tool call message = %+v", msgs[1])
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "c1" || msgs[2].Content != "contenido" {
		t.Errorf("tool result = %+v", msgs[2])
	}
	if msgs[3].Role != "assistant" || msgs[3].Content != "listo" {
		t.Errorf("final message = %+v", msgs[3])
	}
}

func TestEmptyAssistantIsNotModelVisible(t *testing.T) {
	s, _ := newSession(t)
	s.AppendUser("hola")
	s.AppendAssistant("", nil, nil) // e.g. a max_tokens finish with no content
	if msgs := s.Messages(); len(msgs) != 1 {
		t.Errorf("got %d messages, want 1: %+v", len(msgs), msgs)
	}
}

// A process killed mid-batch leaves tool calls without results and a turn
// without its end marker. Opening must repair both, not hand a broken history
// to the API.
func TestOpenRepairsAnInterruptedTurn(t *testing.T) {
	dir := t.TempDir()
	s, err := Create(dir, Meta{CWD: dir, Model: "deepseek-chat"})
	if err != nil {
		t.Fatal(err)
	}
	call := deepseek.ToolCall{ID: "c1"}
	call.Function.Name = "read_file"
	s.AppendUser("lee el archivo")
	s.AppendAssistant("", []deepseek.ToolCall{call}, nil)
	// Crash here: no tool result, no turn/end.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir, s.ID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reopened.Close()

	msgs := reopened.Messages()
	found := false
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "c1" {
			found = true
			if !strings.Contains(m.Content, "interrupted") {
				t.Errorf("synthetic result = %q", m.Content)
			}
		}
	}
	if !found {
		t.Errorf("missing tool result was not repaired: %+v", msgs)
	}

	// The repaired file must contain the fix, not just memory.
	if _, err := os.Stat(filepath.Join(dir, s.ID()+".jsonl")); err != nil {
		t.Fatal(err)
	}
	info := reopenInfo(t, dir, s.ID())
	if info.Turns != 1 {
		t.Errorf("turn/end was not recorded: %d turns", info.Turns)
	}
}

func reopenInfo(t *testing.T, dir, id string) Info {
	t.Helper()
	s, err := Open(dir, id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	list, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, info := range list {
		if info.ID == id {
			return info
		}
	}
	t.Fatalf("session %q not listed", id)
	return Info{}
}

// A compaction hides a span without deleting it: the summary takes the place of
// the first hidden event and the rest of the conversation keeps its order.
func TestMessagesApplyCompaction(t *testing.T) {
	s, _ := newSession(t)
	s.AppendUser("uno")               // seq 1
	s.AppendAssistant("a1", nil, nil) // seq 2
	s.AppendUser("dos")               // seq 3
	s.AppendAssistant("a2", nil, nil) // seq 4
	s.AppendUser("tres")              // seq 5

	// Hide seq 1..2 and replace them with a summary.
	if err := s.AppendCompaction("[resumen de los dos primeros]", 1, 2, 42); err != nil {
		t.Fatal(err)
	}

	msgs := s.Messages()
	var contents []string
	for _, m := range msgs {
		contents = append(contents, m.Content)
	}
	joined := strings.Join(contents, "|")
	if strings.Contains(joined, "uno") || strings.Contains(joined, "a1") {
		t.Errorf("hidden events leaked into the history: %s", joined)
	}
	if !strings.Contains(joined, "resumen") {
		t.Errorf("summary missing: %s", joined)
	}
	// The summary comes first, then the surviving messages in order.
	want := "resumen|dos|a2|tres"
	norm := strings.NewReplacer("[", "", "]", "", " de los dos primeros", "").Replace(joined)
	if !strings.HasPrefix(norm, "resumen") || !strings.HasSuffix(norm, "tres") {
		t.Errorf("order is wrong: %s (normalized %s, want %s)", joined, norm, want)
	}
	order := strings.Index(joined, "resumen")
	for _, later := range []string{"dos", "a2", "tres"} {
		if idx := strings.Index(joined, later); idx < order {
			t.Errorf("%q appears before the summary: %s", later, joined)
		}
	}
}

func TestListAndLatest(t *testing.T) {
	dir := t.TempDir()
	first, err := Create(dir, Meta{Model: "deepseek-chat"})
	if err != nil {
		t.Fatal(err)
	}
	first.AppendUser("primera conversación")
	first.AppendTurnEnd("completed")
	first.Close()

	// Make sure the second session sorts later.
	time.Sleep(10 * time.Millisecond)

	second, err := Create(dir, Meta{Model: "deepseek-chat"})
	if err != nil {
		t.Fatal(err)
	}
	second.AppendUser("segunda conversación")
	second.AppendTurnEnd("completed")
	second.Close()

	list, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sessions, want 2", len(list))
	}
	if list[0].ID != second.ID() {
		t.Errorf("newest first: got %q, want %q", list[0].ID, second.ID())
	}
	if !strings.Contains(list[0].Title, "segunda") {
		t.Errorf("title = %q", list[0].Title)
	}
	if list[0].Turns != 1 {
		t.Errorf("turns = %d, want 1", list[0].Turns)
	}

	latest, err := Latest(dir)
	if err != nil || latest != second.ID() {
		t.Errorf("Latest = %q, %v; want %q", latest, err, second.ID())
	}
}

func TestDelete(t *testing.T) {
	s, dir := newSession(t)
	id := s.ID()
	s.Close()

	if err := Delete(dir, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Open(dir, id); !os.IsNotExist(err) && err != ErrNotFound {
		t.Errorf("Open after delete = %v, want not found", err)
	}
	if err := Delete(dir, id); err != ErrNotFound {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

func TestOpenMissingSession(t *testing.T) {
	if _, err := Open(t.TempDir(), "no-existe"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLastUsage(t *testing.T) {
	s, _ := newSession(t)
	s.AppendUser("hola")
	s.AppendAssistant("ok", nil, &deepseek.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12})

	usage, ok := s.LastUsage()
	if !ok || usage.TotalTokens != 12 {
		t.Errorf("LastUsage = %+v, %v", usage, ok)
	}
}

// A torn final line (crash mid-write) must not make the session unreadable.
// A compaction changes the surface, so the provider count of the last request
// no longer describes it: the meter must fall back to estimating.
func TestCompactionInvalidatesTheUsageAnchor(t *testing.T) {
	s, _ := newSession(t)
	s.AppendUser("hola")
	s.AppendAssistant("respuesta", nil, &deepseek.Usage{PromptTokens: 500, CompletionTokens: 50, TotalTokens: 550})

	snap := s.Snapshot()
	if snap.AnchorUsage == nil {
		t.Fatal("expected an anchor before compacting")
	}
	if snap.AnchorLen != 2 {
		t.Errorf("AnchorLen = %d, want 2", snap.AnchorLen)
	}

	if err := s.AppendCompaction("[resumen]", snap.Seqs[0], snap.Seqs[1], 400); err != nil {
		t.Fatal(err)
	}
	if snap = s.Snapshot(); snap.AnchorUsage != nil {
		t.Error("a compaction must invalidate the usage anchor")
	}
}

// Pruning shortens old tool output without touching the rest of the history.
func TestPruneShortensOnlyToolResults(t *testing.T) {
	s, _ := newSession(t)
	big := strings.Repeat("x", 4000)
	call := deepseek.ToolCall{ID: "c1"}
	call.Function.Name = "run_shell"

	s.AppendUser("ejecuta algo")
	s.AppendAssistant("", []deepseek.ToolCall{call}, nil)
	s.AppendToolResult("c1", big, false)
	s.AppendAssistant("ya está", nil, nil)

	snap := s.Snapshot()
	if err := s.AppendPrune(snap.Seqs[0], snap.Seqs[len(snap.Seqs)-1], 0); err != nil {
		t.Fatal(err)
	}

	msgs := s.Messages()
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want the same 4", len(msgs))
	}
	// The conversation and the tool call stay intact.
	if msgs[0].Content != "ejecuta algo" || len(msgs[1].ToolCalls) != 1 || msgs[3].Content != "ya está" {
		t.Errorf("pruning altered non-tool messages: %+v", msgs)
	}
	// The tool result is still a tool result for the same call, but short.
	tool := msgs[2]
	if tool.Role != "tool" || tool.ToolCallID != "c1" {
		t.Errorf("pruned result lost its identity: %+v", tool)
	}
	if len(tool.Content) >= len(big) {
		t.Error("tool output was not shortened")
	}
	if !strings.Contains(tool.Content, "pruned") {
		t.Errorf("shortened output has no marker: %q", tool.Content)
	}
	if !strings.HasPrefix(tool.Content, "xxx") || !strings.HasSuffix(tool.Content, "xxx") {
		t.Error("head and tail were not preserved")
	}
}

func TestShortenToolResultKeepsSmallOutput(t *testing.T) {
	small := strings.Repeat("y", 100)
	if got := ShortenToolResult(small); got != small {
		t.Errorf("small output was altered: %q", got)
	}
}

func TestPruneInvalidatesTheAnchor(t *testing.T) {
	s, _ := newSession(t)
	s.AppendUser("hola")
	s.AppendAssistant("respuesta", nil, &deepseek.Usage{PromptTokens: 300, CompletionTokens: 10})
	snap := s.Snapshot()
	if snap.AnchorUsage == nil {
		t.Fatal("expected an anchor")
	}
	if err := s.AppendPrune(snap.Seqs[0], snap.Seqs[1], 100); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().AnchorUsage != nil {
		t.Error("pruning must invalidate the usage anchor")
	}
}
func TestTornLineIsIgnored(t *testing.T) {
	dir := t.TempDir()
	s, err := Create(dir, Meta{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	s.AppendUser("hola")
	s.AppendTurnEnd("completed")
	s.Close()

	path := filepath.Join(dir, s.ID()+".jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"seq":99,"type":"user/message","data":{"cont`)
	f.Close()

	reopened, err := Open(dir, s.ID())
	if err != nil {
		t.Fatalf("Open with a torn line: %v", err)
	}
	defer reopened.Close()
	if msgs := reopened.Messages(); len(msgs) != 1 {
		t.Errorf("got %d messages, want 1", len(msgs))
	}
}

func TestEventJSONIsStable(t *testing.T) {
	// The on-disk format is a contract: keep the field names.
	ev := Event{Seq: 3, Type: EventToolResult, Data: json.RawMessage(`{"tool_call_id":"c1"}`)}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"seq":3`, `"type":"tool/result"`, `"time":"`, `"data":`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("event JSON %s is missing %s", raw, want)
		}
	}
}

// The sessions written before the rename must survive it.
func TestDirMigratesTheLegacySessions(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("XDG_DATA_HOME", base)

	legacy := filepath.Join(base, "deekseep-coder", "sessions")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "vieja.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := Dir()
	if !strings.HasPrefix(dir, filepath.Join(base, "deepseek-coder")) {
		t.Fatalf("Dir() = %q, want it under deepseek-coder", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "vieja.jsonl")); err != nil {
		t.Errorf("the old sessions were not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "deekseep-coder")); !os.IsNotExist(err) {
		t.Error("the legacy directory is still there")
	}
}
