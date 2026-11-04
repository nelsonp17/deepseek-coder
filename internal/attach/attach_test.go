package attach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferences(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"none", "hola que tal", nil},
		{"single", "revisa @main.go por favor", []string{"main.go"}},
		{"multiple", "@a.go y @b/c.go", []string{"a.go", "b/c.go"}},
		{"start of message", "@main.go", []string{"main.go"}},
		{"trailing period", "mira @main.go.", []string{"main.go"}},
		{"trailing comma", "mira @main.go, listo", []string{"main.go"}},
		{"email is not a reference", "escribe a user@example.com", nil},
		{"quoted with spaces", `revisa @"mi archivo.go" ya`, []string{"mi archivo.go"}},
		{"single quoted", `revisa @'mi archivo.go'`, []string{"mi archivo.go"}},
		{"nested path", "@internal/shell/shell.go", []string{"internal/shell/shell.go"}},
		{"directory", "@internal/", []string{"internal/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs := References(tc.msg)
			var got []string
			for _, r := range refs {
				got = append(got, r.Path)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("References(%q) = %v, want %v", tc.msg, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("Reference[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestResolveAttachesContent(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	payload, atts := Resolve("revisa @main.go por favor", Options{})
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].Err != nil {
		t.Fatalf("attachment error: %v", atts[0].Err)
	}
	if atts[0].Language != "go" {
		t.Errorf("Language = %q, want go", atts[0].Language)
	}
	if atts[0].Bytes != len(content) {
		t.Errorf("Bytes = %d, want %d", atts[0].Bytes, len(content))
	}
	if atts[0].Lines != 3 {
		t.Errorf("Lines = %d, want 3", atts[0].Lines)
	}

	// The user's wording is preserved and the content is appended.
	if !strings.HasPrefix(payload, "revisa @main.go por favor") {
		t.Errorf("payload lost the original message:\n%s", payload)
	}
	if !strings.Contains(payload, "func main() {}") {
		t.Errorf("payload is missing the file content:\n%s", payload)
	}
	if !strings.Contains(payload, "```go") {
		t.Errorf("payload is missing the language fence:\n%s", payload)
	}
	if !strings.Contains(payload, "Attached files") {
		t.Errorf("payload is missing the section header:\n%s", payload)
	}
}

// A file that contains a Markdown fence must not break out of the block.
func TestResolveUsesLongerFence(t *testing.T) {
	dir := t.TempDir()
	content := "# Título\n\n```go\nfunc main() {}\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	payload, atts := Resolve("mira @README.md", Options{})
	if len(atts) != 1 || atts[0].Err != nil {
		t.Fatalf("unexpected attachments: %+v", atts)
	}
	// The inner ``` run is 3, so the wrapper must be at least 4.
	if !strings.Contains(payload, "````markdown") {
		t.Errorf("wrapper fence is not longer than the content's:\n%s", payload)
	}
	if strings.Contains(payload, "\n```markdown") {
		t.Errorf("wrapper fence collides with the content:\n%s", payload)
	}
}

func TestResolveSkipsBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.exe"), []byte{'M', 'Z', 0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	payload, atts := Resolve("mira @app.exe", Options{})
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].Err != ErrBinary {
		t.Fatalf("Err = %v, want ErrBinary", atts[0].Err)
	}
	if strings.Contains(payload, "MZ") {
		t.Errorf("binary content leaked into the payload:\n%s", payload)
	}
}

func TestResolveSkipsTooLarge(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("x", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, atts := Resolve("mira @big.txt", Options{MaxFileBytes: 100})
	if len(atts) != 1 || atts[0].Err != ErrTooLarge {
		t.Fatalf("attachments = %+v, want ErrTooLarge", atts)
	}
	if atts[0].Bytes != 500 {
		t.Errorf("Bytes = %d, want the real size 500", atts[0].Bytes)
	}
}

func TestResolveHonoursTotalBudget(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("y", 200)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	// 150 bytes of budget: the first file is truncated to fit, the second has
	// nothing left.
	_, atts := Resolve("mira @a.txt y @b.txt", Options{MaxFileBytes: 1000, MaxTotalBytes: 150})
	if len(atts) != 2 {
		t.Fatalf("got %d attachments, want 2", len(atts))
	}
	if atts[0].Err != nil || !atts[0].Truncated || atts[0].Bytes != 150 {
		t.Errorf("first = %+v, want 150 truncated bytes", atts[0])
	}
	if atts[1].Err != ErrBudget {
		t.Errorf("second = %v, want ErrBudget", atts[1].Err)
	}
}

func TestResolveSkipsMissingAndDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, atts := Resolve("mira @no-existe.go y @sub", Options{})
	if len(atts) != 2 {
		t.Fatalf("got %d attachments, want 2", len(atts))
	}
	if atts[0].Err != ErrNotFound {
		t.Errorf("first = %v, want ErrNotFound", atts[0].Err)
	}
	if atts[1].Err != ErrDirectory {
		t.Errorf("second = %v, want ErrDirectory", atts[1].Err)
	}
}

func TestResolveDeduplicates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	// The same file through two spellings.
	_, atts := Resolve("mira @a.go y @./a.go", Options{})
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1 (deduplicated)", len(atts))
	}
}

func TestResolveWithoutReferencesIsIdentity(t *testing.T) {
	msg := "hola, sin adjuntos"
	payload, atts := Resolve(msg, Options{})
	if payload != msg {
		t.Errorf("payload = %q, want it unchanged", payload)
	}
	if len(atts) != 0 {
		t.Errorf("atts = %v, want none", atts)
	}
}

func TestLanguage(t *testing.T) {
	cases := map[string]string{
		"a.go": "go", "a.MD": "markdown", "b.py": "python",
		"Dockerfile": "dockerfile", "desconocido.xyz": "",
	}
	for path, want := range cases {
		if got := Language(path); got != want {
			t.Errorf("Language(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int]string{
		0: "0 B", 512: "512 B", 2048: "2.0 KB", 3 * 1024 * 1024: "3.0 MB",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
