package session

import (
	"os"
	"strings"
	"testing"
	"time"
)

func seedSession(t *testing.T, dir, id, content string) {
	t.Helper()
	s, err := Create(dir, Meta{Model: "m"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendUser(content); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAssistant("respuesta a "+content, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTurnEnd("completed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_ = id
}

func TestSearchFindsContent(t *testing.T) {
	dir := t.TempDir()
	seedSession(t, dir, "", "arreglé el endpoint /health del servidor")

	hits, err := Search(dir, "health", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits for a word that is in the log")
	}
	if !strings.Contains(hits[0].Snippet, "health") {
		t.Errorf("snippet = %q", hits[0].Snippet)
	}
	if hits[0].Role != "user" && hits[0].Role != "assistant" {
		t.Errorf("role = %q", hits[0].Role)
	}
	if hits[0].SessionID == "" {
		t.Error("hit has no session id")
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	seedSession(t, dir, "", "El Endpoint /HEALTH responde 200")

	hits, err := Search(dir, "endpoint", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Error("the search is case sensitive")
	}
}

func TestSearchReturnsNothingForUnknown(t *testing.T) {
	dir := t.TempDir()
	seedSession(t, dir, "", "hola")
	hits, err := Search(dir, "palabra-que-no-existe", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("got %d hits, want 0", len(hits))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	seedSession(t, dir, "", "hola")
	hits, err := Search(dir, "   ", 10)
	if err != nil || hits != nil {
		t.Errorf("empty query = %v, %v; want nothing", hits, err)
	}
}

func TestSearchRespectsTheLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		seedSession(t, dir, "", "repetido repetido repetido")
	}
	hits, err := Search(dir, "repetido", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 2 {
		t.Errorf("got %d hits, want at most 2", len(hits))
	}
}

func TestSearchOrdersNewestFirst(t *testing.T) {
	dir := t.TempDir()
	old, err := Create(dir, Meta{})
	if err != nil {
		t.Fatal(err)
	}
	old.AppendUser("marca vieja")
	old.Close()
	time.Sleep(15 * time.Millisecond)
	seedSession(t, dir, "", "marca nueva")

	hits, err := Search(dir, "marca", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].Time.Before(hits[1].Time) {
		t.Error("hits are not newest first")
	}
}

func TestSnippetAround(t *testing.T) {
	text := strings.Repeat("a", 500) + "AGUJA" + strings.Repeat("b", 500)
	snip := snippetAround(text, 500, 5)
	if !strings.Contains(snip, "AGUJA") {
		t.Errorf("snippet misses the match: %q", snip)
	}
	if len(snip) > searchSnippet*2 {
		t.Errorf("snippet is %d bytes, want it bounded", len(snip))
	}
	if !strings.HasPrefix(snip, "…") || !strings.HasSuffix(snip, "…") {
		t.Errorf("snippet does not mark the cut: %q", snip)
	}
}

// A torn line in the middle of a log must not break the search.
func TestSearchSkipsBrokenLines(t *testing.T) {
	dir := t.TempDir()
	seedSession(t, dir, "", "contenido bueno")
	infos, err := List(dir)
	if err != nil || len(infos) != 1 {
		t.Fatalf("List = %+v, %v", infos, err)
	}
	f, err := os.OpenFile(infos[0].Path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{roto\n")
	f.Close()

	hits, err := Search(dir, "bueno", 10)
	if err != nil {
		t.Fatalf("Search with a broken line: %v", err)
	}
	if len(hits) == 0 {
		t.Error("the good content was not found")
	}
}
