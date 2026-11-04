package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// On Windows and macOS file names are case-insensitive, so "@rea" must offer
// README.md. Before this, the completion used a case-sensitive prefix and
// typing lowercase found nothing at all.
func TestFileCompletionIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"README.md", "readme.txt", "otro.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	for _, typed := range []string{"rea", "REA", "Rea"} {
		comp := fileCompletion(typed)
		if len(comp.Candidates) != 2 {
			t.Fatalf("typed %q: got %d candidates %v, want 2", typed, len(comp.Candidates), comp.Candidates)
		}
		joined := strings.Join(comp.Candidates, " ")
		if !strings.Contains(joined, "README.md") || !strings.Contains(joined, "readme.txt") {
			t.Errorf("typed %q: candidates = %v", typed, comp.Candidates)
		}
		if strings.Contains(joined, "otro.txt") {
			t.Errorf("typed %q: matched an unrelated file: %v", typed, comp.Candidates)
		}
	}
}

// The candidate keeps the file's real case, so the inserted path works.
func TestFileCompletionKeepsRealCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	comp := fileCompletion("rea")
	if len(comp.Candidates) != 1 {
		t.Fatalf("candidates = %v", comp.Candidates)
	}
	if comp.Candidates[0] != "@README.md" {
		t.Errorf("candidate = %q, want @README.md", comp.Candidates[0])
	}
}

func TestHasPrefixFold(t *testing.T) {
	cases := []struct {
		s, prefix string
		want      bool
	}{
		{"README.md", "rea", true},
		{"README.md", "REA", true},
		{"README.md", "read", true},
		{"README.md", "readme.md", true},
		{"README.md", "readme.markdown", false},
		{"otro.txt", "rea", false},
		{"", "a", false},
		{"ñandú", "ÑAN", true},
	}
	for _, c := range cases {
		if got := hasPrefixFold(c.s, c.prefix); got != c.want {
			t.Errorf("hasPrefixFold(%q, %q) = %v, want %v", c.s, c.prefix, got, c.want)
		}
	}
}

// The reported case: typing a lowercase "@rea" in a sentence must surface the
// suggestion in the TUI, not just in the completer function.
func TestTypingAtShowsTheSuggestion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	m := newTUIModel(testApp(), nil, "deepseek-chat")
	m.textarea.SetValue("pues leer el archivo @rea")

	// Any key press refreshes the suggestions from the textarea contents.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := next.(tuiModel)

	if len(got.suggestions) == 0 {
		t.Fatal("no suggestions shown for @rea")
	}
	joined := strings.Join(got.suggestions, " ")
	if !strings.Contains(joined, "README.md") {
		t.Errorf("suggestions = %v, want README.md", got.suggestions)
	}
}
