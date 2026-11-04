package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxFileLen = 6000
	maxDiffLen = 4000
)

// Skill is a discoverable agent skill.
type Skill struct {
	Name        string
	Description string
	// Path is the SKILL.md the skill was loaded from, so its body can be read
	// on demand. Listing the catalogue must not cost the body of every skill.
	Path string
}

// maxSkillBody caps how much of a skill body is injected into a message.
const maxSkillBody = 8 * 1024

// Options selects which parts of the project are loaded. Everything it turns
// off is context the model does not have to read (and pay for) on every turn.
type Options struct {
	// Readme includes README.md, usually the largest file in the repo.
	Readme bool
	// Diff includes the working tree diff, which grows with uncommitted work.
	Diff bool
}

// Full is the default: load everything.
var Full = Options{Readme: true, Diff: true}

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

	// Options records what was actually loaded, so callers can explain what is
	// missing instead of silently sending less context.
	Options Options
}

// Load reads the full project context from dir.
func Load(dir string) (*Project, error) { return LoadWith(dir, Full) }

// LoadWith reads the project context from dir, honouring opts.
func LoadWith(dir string, opts Options) (*Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	p := &Project{Root: abs, Options: opts}
	p.AGENTS = readFileAt(abs, "AGENTS.md")
	p.DESIGN = readFileAt(abs, "DESIGN.md")
	if opts.Readme {
		p.README = readFileAt(abs, "README.md")
	}
	p.loadSkills(abs)
	p.loadGit(abs, opts.Diff)
	return p, nil
}

// Part is one section of the project context.
type Part struct {
	// Name is a stable key: "header", "agents", "design", "readme", "skills"
	// or "git". Callers translate it.
	Name    string
	Content string
}

// Parts splits the context by section so callers can report where the tokens
// go.
func (p *Project) Parts() []Part {
	var parts []Part
	parts = append(parts, Part{
		Name:    "header",
		Content: fmt.Sprintf("## Project context\nWorking directory: %s\n", p.Root),
	})
	if p.AGENTS != "" {
		parts = append(parts, Part{Name: "agents", Content: fmt.Sprintf("\n### AGENTS.md (project instructions — follow them)\n%s\n", p.AGENTS)})
	}
	if p.DESIGN != "" {
		parts = append(parts, Part{Name: "design", Content: fmt.Sprintf("\n### DESIGN.md (design document)\n%s\n", p.DESIGN)})
	}
	if p.README != "" {
		parts = append(parts, Part{Name: "readme", Content: fmt.Sprintf("\n### README.md (project overview)\n%s\n", p.README)})
	}
	if len(p.Skills) > 0 {
		var b strings.Builder
		b.WriteString("\n### Skills available\n")
		for _, s := range p.Skills {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
		parts = append(parts, Part{Name: "skills", Content: b.String()})
	}
	if p.HasGit {
		var b strings.Builder
		b.WriteString("\n### Git\n")
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
		parts = append(parts, Part{Name: "git", Content: b.String()})
	}
	return parts
}

// Context renders the project context as a text block for the system prompt.
func (p *Project) Context() string {
	var b strings.Builder
	for _, part := range p.Parts() {
		b.WriteString(part.Content)
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
	return Skill{Name: name, Description: extractField(fm, "description"), Path: path}, true
}

// ReadSkillBody returns the instructions of a skill: its SKILL.md without the
// YAML frontmatter, truncated to maxSkillBody.
//
// Referencing a skill has to send its instructions; sending only its name
// leaves the model guessing what the skill does.
func ReadSkillBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if strings.HasPrefix(content, "---") {
		if end := strings.Index(content[3:], "---"); end >= 0 {
			content = content[3+end+3:]
		}
	}
	body := strings.TrimSpace(content)
	if body == "" {
		return "", errors.New("skill has no instructions")
	}
	if len(body) > maxSkillBody {
		cut := maxSkillBody
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		body = body[:cut] + "\n\n[…skill truncated…]"
	}
	return body, nil
}

// extractField reads a scalar field from a YAML frontmatter block.
//
// It understands the block scalars used by most SKILL.md files ("description: >"
// or "description: |"), where the real value sits on the indented lines that
// follow instead of on the same line.
func extractField(fm, key string) string {
	lines := strings.Split(fm, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, key+":") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
		indicator := strings.TrimRight(value, "-+ \t")
		if indicator != ">" && indicator != "|" {
			return strings.Trim(value, `"'`)
		}
		// Block scalar: collect the indented lines that follow.
		var body []string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				body = append(body, "")
				continue
			}
			if len(next) == len(strings.TrimLeft(next, " \t")) {
				break // back to a non-indented key
			}
			body = append(body, strings.TrimSpace(next))
		}
		if indicator == "|" {
			return strings.TrimSpace(strings.Join(body, "\n"))
		}
		return strings.TrimSpace(collapseSpaces(strings.Join(body, "\n")))
	}
	return ""
}

// collapseSpaces squeezes repeated spaces left behind by joining block lines.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (p *Project) loadGit(dir string, withDiff bool) {
	if runGit(dir, "rev-parse", "--is-inside-work-tree") != "true" {
		return
	}
	p.HasGit = true
	p.Branch = runGit(dir, "branch", "--show-current")
	p.Status = truncate(runGit(dir, "status", "--short"), maxDiffLen)
	if withDiff {
		p.Diff = truncate(runGit(dir, "diff"), maxDiffLen)
	}
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
