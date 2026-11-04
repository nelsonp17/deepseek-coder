package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadContextFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Instrucciones del proyecto")
	writeFile(t, filepath.Join(dir, "design.md"), "# Diseño") // lowercase variant
	writeFile(t, filepath.Join(dir, "README.md"), "# Proyecto")
	writeFile(t, filepath.Join(dir, ".agents", "skills", "test-skill", "SKILL.md"),
		"---\nname: test-skill\ndescription: Una skill de prueba\n---\n# Body\n")

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(p.AGENTS, "Instrucciones") {
		t.Error("AGENTS.md not loaded")
	}
	if !strings.Contains(p.DESIGN, "Diseño") {
		t.Error("DESIGN.md not loaded (case-insensitive)")
	}
	if !strings.Contains(p.README, "Proyecto") {
		t.Error("README.md not loaded")
	}
	found := false
	for _, s := range p.Skills {
		if s.Name == "test-skill" {
			found = true
			if s.Description != "Una skill de prueba" {
				t.Errorf("skill description = %q", s.Description)
			}
		}
	}
	if !found {
		t.Errorf("project skill test-skill not found in %+v", p.Skills)
	}

	ctx := p.Context()
	if !strings.Contains(ctx, "AGENTS.md") || !strings.Contains(ctx, "Skills available") {
		t.Error("Context() missing sections")
	}
}

// The light mode drops the README and the diff, which are the two big
// contributors to the prompt, and keeps what is cheap.
func TestLoadWithLightOptions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "README.md"), "# Un README enorme que no queremos en cada turno\n")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Instrucciones que sí importan\n")
	writeFile(t, filepath.Join(dir, "a.txt"), "hola\n")
	run("add", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "first commit")
	writeFile(t, filepath.Join(dir, "a.txt"), "cambio sin commitear\n")

	full, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if full.README == "" || full.Diff == "" {
		t.Fatal("full mode should load the README and the diff")
	}

	light, err := LoadWith(dir, Options{})
	if err != nil {
		t.Fatalf("LoadWith: %v", err)
	}
	if light.README != "" {
		t.Error("light mode still loads README.md")
	}
	if light.Diff != "" {
		t.Error("light mode still loads the git diff")
	}
	if light.AGENTS == "" {
		t.Error("light mode must keep AGENTS.md: those are the instructions")
	}
	if light.Branch != "main" || light.RecentCommits == "" {
		t.Errorf("light mode should keep branch and commits, got %q / %q", light.Branch, light.RecentCommits)
	}

	fullCtx, lightCtx := full.Context(), light.Context()
	if len(lightCtx) >= len(fullCtx) {
		t.Errorf("light context is not smaller: %d vs %d bytes", len(lightCtx), len(fullCtx))
	}
}

func TestPartsCoverTheContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Instrucciones\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# Proyecto\n")

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	parts := p.Parts()
	seen := map[string]bool{}
	total := 0
	for _, part := range parts {
		seen[part.Name] = true
		total += len(part.Content)
	}
	for _, want := range []string{"header", "agents", "readme"} {
		if !seen[want] {
			t.Errorf("missing part %q in %+v", want, seen)
		}
	}
	// Parts are the context, byte for byte.
	if total != len(p.Context()) {
		t.Errorf("parts total %d bytes, Context() is %d", total, len(p.Context()))
	}
}

func TestLoadGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "hola\n")
	run("add", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "first commit")
	writeFile(t, filepath.Join(dir, "a.txt"), "hola mundo\n")

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !p.HasGit {
		t.Fatal("expected HasGit")
	}
	if p.Branch != "main" {
		t.Errorf("branch = %q, want main", p.Branch)
	}
	if !strings.Contains(p.Status, "a.txt") {
		t.Errorf("status missing a.txt: %q", p.Status)
	}
	if !strings.Contains(p.Diff, "hola mundo") {
		t.Errorf("diff missing change: %q", p.Diff)
	}
	if !strings.Contains(p.RecentCommits, "first commit") {
		t.Errorf("commits missing: %q", p.RecentCommits)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	long := strings.Repeat("ñ", 100)
	got := truncate(long, 10)
	want := 10 + 1 + len([]rune("… (truncated)")) // content + \n + suffix
	if len([]rune(got)) != want {
		t.Errorf("truncate long length = %d runes, want %d", len([]rune(got)), want)
	}
}

// SKILL.md files usually write the description as a YAML block scalar. Reading
// only the first line would surface ">" as the description, which is what the
// help catalog used to show.
func TestExtractFieldBlockScalar(t *testing.T) {
	folded := "\nname: caveman\ndescription: >\n  Ultra-compressed mode.\n  Keeps accuracy.\nlicense: MIT\n"
	if got, want := extractField(folded, "description"), "Ultra-compressed mode. Keeps accuracy."; got != want {
		t.Errorf("extractField(folded) = %q, want %q", got, want)
	}

	literal := "\nname: x\ndescription: |\n  Primera línea.\n  Segunda línea.\n"
	if got, want := extractField(literal, "description"), "Primera línea.\nSegunda línea."; got != want {
		t.Errorf("extractField(literal) = %q, want %q", got, want)
	}

	plain := "\nname: x\ndescription: \"Una skill de prueba\"\n"
	if got, want := extractField(plain, "description"), "Una skill de prueba"; got != want {
		t.Errorf("extractField(plain) = %q, want %q", got, want)
	}
}
