package fstools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSet builds a tool set rooted at a fresh temporary directory.
func newSet(t *testing.T) (*Set, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, dir
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPolicyRejectsPathsOutsideTheRoot(t *testing.T) {
	s, _ := newSet(t)

	for _, bad := range []string{"../outside.txt", "../../etc/passwd"} {
		if _, err := s.policy.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) should fail", bad)
		} else {
			var outside *OutsideError
			if !errors.As(err, &outside) {
				t.Errorf("Resolve(%q) error = %v, want OutsideError", bad, err)
			}
		}
	}

	// An absolute path outside the root is refused too.
	if _, err := s.policy.Resolve(filepath.Join(os.TempDir(), "x.txt")); err == nil {
		t.Error("absolute path outside the root should fail")
	}

	// A relative path is resolved against the root, not the process cwd.
	got, err := s.policy.Resolve("sub/a.txt")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(s.Root(), "sub", "a.txt"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestReadNumbersLines(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.txt", "uno\ndos\ntres\n")

	got, err := s.Read("a.txt", 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, want := range []string{"1\tuno", "2\tdos", "3\ttres"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
	// No phantom fourth line for the trailing newline.
	if strings.Contains(got, "4\t") {
		t.Errorf("phantom trailing line:\n%s", got)
	}
}

func TestReadHonoursOffsetAndLimit(t *testing.T) {
	s, dir := newSet(t)
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, "linea "+string(rune('0'+i%10)))
	}
	writeTestFile(t, dir, "many.txt", strings.Join(lines, "\n")+"\n")

	got, err := s.Read("many.txt", 10, 5)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got, "10\t") {
		t.Errorf("offset not applied:\n%s", got)
	}
	if strings.Contains(got, "16\t") {
		t.Errorf("limit not applied:\n%s", got)
	}
	if !strings.Contains(got, "more lines") {
		t.Errorf("missing continuation footer:\n%s", got)
	}
}

func TestReadRejectsBinaryAndDirectories(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "bin.dat", "MZ\x00\x01\x02")
	if _, err := s.Read("bin.dat", 0, 0); !errors.Is(err, ErrBinary) {
		t.Errorf("binary read error = %v, want ErrBinary", err)
	}
	if _, err := s.Read(".", 0, 0); err == nil {
		t.Error("reading a directory should fail")
	}
}

func TestWriteRequiresReadingAnExistingFile(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.txt", "original\n")

	if _, err := s.Write("a.txt", "nuevo\n"); !errors.Is(err, ErrNotObserved) {
		t.Fatalf("write without reading error = %v, want ErrNotObserved", err)
	}
	if _, err := s.Read("a.txt", 0, 0); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := s.Write("a.txt", "nuevo\n"); err != nil {
		t.Fatalf("write after reading: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "nuevo\n" {
		t.Errorf("content = %q", string(data))
	}
}

func TestWriteCreatesNewFilesWithoutReading(t *testing.T) {
	s, dir := newSet(t)
	out, err := s.Write("sub/nuevo.txt", "hola\n")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("report = %q, want created", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "nuevo.txt")); err != nil {
		t.Errorf("file was not created: %v", err)
	}
}

func TestWriteRejectsStaleFiles(t *testing.T) {
	s, dir := newSet(t)
	path := writeTestFile(t, dir, "a.txt", "uno\n")
	if _, err := s.Read("a.txt", 0, 0); err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Someone else edits the file behind our back.
	if err := os.WriteFile(path, []byte("cambiado por otro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("a.txt", "nuevo\n"); !errors.Is(err, ErrStale) {
		t.Errorf("stale write error = %v, want ErrStale", err)
	}
}

func TestEditReplacesUniqueText(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.go", "package a\n\nfunc f() { return 1 }\n")
	if _, err := s.Read("a.go", 0, 0); err != nil {
		t.Fatalf("Read: %v", err)
	}

	out, err := s.Edit("a.go", "return 1", "return 2", false)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if !strings.Contains(out, "1 replacement") {
		t.Errorf("report = %q", out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if !strings.Contains(string(data), "return 2") {
		t.Errorf("edit not applied: %q", string(data))
	}
}

func TestEditRejectsAmbiguousAndMissingText(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.txt", "x\nx\n")
	if _, err := s.Read("a.txt", 0, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Edit("a.txt", "x", "y", false); err == nil {
		t.Error("ambiguous edit should fail")
	} else if !strings.Contains(err.Error(), "2 times") {
		t.Errorf("error should say how many matches: %v", err)
	}

	if _, err := s.Edit("a.txt", "zzz", "y", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing text error = %v, want ErrNotFound", err)
	}

	// replace_all resolves the ambiguity on purpose.
	if _, err := s.Edit("a.txt", "x", "y", true); err != nil {
		t.Errorf("replace_all: %v", err)
	}
}

func TestEditRequiresReading(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.txt", "hola\n")
	if _, err := s.Edit("a.txt", "hola", "adios", false); !errors.Is(err, ErrNotObserved) {
		t.Errorf("edit without reading error = %v, want ErrNotObserved", err)
	}
}

func TestGlobMatchesTopLevelAndNested(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "main.go", "package main\n")
	writeTestFile(t, dir, "cli/app.go", "package main\n")
	writeTestFile(t, dir, "node_modules/pkg/ignored.go", "package x\n")
	writeTestFile(t, dir, "README.md", "# doc\n")

	got, err := s.Glob("**/*.go", 0)
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if !strings.Contains(got, "main.go") || !strings.Contains(got, "cli/app.go") {
		t.Errorf("missing expected files:\n%s", got)
	}
	if strings.Contains(got, "ignored.go") {
		t.Errorf("node_modules should be skipped:\n%s", got)
	}
	if strings.Contains(got, "README.md") {
		t.Errorf("non-matching files leaked:\n%s", got)
	}
}

func TestGlobRespectsTheLimit(t *testing.T) {
	s, dir := newSet(t)
	for i := 0; i < 10; i++ {
		writeTestFile(t, dir, filepath.Join("d", string(rune('a'+i))+".txt"), "x\n")
	}
	got, err := s.Glob("d/*.txt", 3)
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if n := strings.Count(got, ".txt"); n > 3 {
		t.Errorf("limit exceeded: %d entries\n%s", n, got)
	}
}

func TestGrepFindsLines(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a.go", "package a\n\nfunc Hello() {}\n")
	writeTestFile(t, dir, "b.go", "package b\n")

	got, err := s.Grep("func Hello", "", 0)
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "a.go:3:") {
		t.Errorf("match not reported with its line:\n%s", got)
	}
	if strings.Contains(got, "b.go") {
		t.Errorf("non-matching file leaked:\n%s", got)
	}
}

func TestGrepScopedToAPath(t *testing.T) {
	s, dir := newSet(t)
	writeTestFile(t, dir, "a/one.txt", "target\n")
	writeTestFile(t, dir, "b/two.txt", "target\n")

	got, err := s.Grep("target", "a", 0)
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "a/one.txt") || strings.Contains(got, "b/two.txt") {
		t.Errorf("scope not applied:\n%s", got)
	}
}

func TestGrepRejectsBadPattern(t *testing.T) {
	s, _ := newSet(t)
	if _, err := s.Grep("(", "", 0); err == nil {
		t.Error("invalid pattern should fail")
	}
}

func TestNoMatchesMessages(t *testing.T) {
	s, _ := newSet(t)
	if got, err := s.Grep("nada", "", 0); err != nil || !strings.Contains(got, "no matches") {
		t.Errorf("Grep = %q, %v", got, err)
	}
	if got, err := s.Glob("*.zzz", 0); err != nil || !strings.Contains(got, "no files match") {
		t.Errorf("Glob = %q, %v", got, err)
	}
}
