package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxFileLen = 6000
	maxDiffLen = 4000
)

// Skill is a discoverable agent skill.
type Skill struct {
	Name        string
	Description string
}

// Project holds the context of the working directory.
type Project struct {
	Root   string
	AGENTS string
	DESIGN string
	README string
	Skills []Skill

	HasGit        bool
	Branch        string
	Status        string
	Diff          string
	RecentCommits string
}

// Load reads the project context from dir.
func Load(dir string) (*Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	p := &Project{Root: abs}
	p.AGENTS = readFileAt(abs, "AGENTS.md")
	p.DESIGN = readFileAt(abs, "DESIGN.md")
	p.README = readFileAt(abs, "README.md")
	p.loadSkills(abs)
	p.loadGit(abs)
	return p, nil
}

// Context renders the project context as a text block for the system prompt.
func (p *Project) Context() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project context\n")
	fmt.Fprintf(&b, "Working directory: %s\n", p.Root)
	if p.AGENTS != "" {
		fmt.Fprintf(&b, "\n### AGENTS.md (project instructions — follow them)\n%s\n", p.AGENTS)
	}
	if p.DESIGN != "" {
		fmt.Fprintf(&b, "\n### DESIGN.md (design document)\n%s\n", p.DESIGN)
	}
	if p.README != "" {
		fmt.Fprintf(&b, "\n### README.md (project overview)\n%s\n", p.README)
	}
	if len(p.Skills) > 0 {
		fmt.Fprintf(&b, "\n### Skills available\n")
		for _, s := range p.Skills {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
	}
	if p.HasGit {
		fmt.Fprintf(&b, "\n### Git\n")
		if p.Branch != "" {
			fmt.Fprintf(&b, "Branch: %s\n", p.Branch)
		}
		if p.Status != "" {
			fmt.Fprintf(&b, "Status:\n%s\n", p.Status)
		}
		if p.Diff != "" {
			fmt.Fprintf(&b, "Diff:\n%s\n", p.Diff)
		}
		if p.RecentCommits != "" {
			fmt.Fprintf(&b, "Recent commits:\n%s\n", p.RecentCommits)
		}
	}
	return b.String()
}

func readFileAt(dir, name string) string {
	path := findFile(dir, name)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return truncate(string(data), maxFileLen)
}

func findFile(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func (p *Project) loadSkills(dir string) {
	dirs := []string{filepath.Join(dir, ".agents", "skills")}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".agents", "skills"))
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sp := filepath.Join(d, e.Name(), "SKILL.md")
			if s, ok := parseSkill(sp); ok && !seen[s.Name] {
				seen[s.Name] = true
				p.Skills = append(p.Skills, s)
			}
		}
	}
}

func parseSkill(path string) (Skill, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return Skill{}, false
	}
	rest := content[3:]
	end := strings.Index(rest, "---")
	if end < 0 {
		return Skill{}, false
	}
	fm := rest[:end]
	name := extractField(fm, "name")
	if name == "" {
		return Skill{}, false
	}
	return Skill{Name: name, Description: extractField(fm, "description")}, true
}

func extractField(fm, key string) string {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			v := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func (p *Project) loadGit(dir string) {
	if runGit(dir, "rev-parse", "--is-inside-work-tree") != "true" {
		return
	}
	p.HasGit = true
	p.Branch = runGit(dir, "branch", "--show-current")
	p.Status = truncate(runGit(dir, "status", "--short"), maxDiffLen)
	p.Diff = truncate(runGit(dir, "diff"), maxDiffLen)
	p.RecentCommits = runGit(dir, "log", "--oneline", "-n", "5")
}

func runGit(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n… (truncated)"
}
