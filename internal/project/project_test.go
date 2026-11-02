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
