package fstools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Write creates or replaces a file.
//
// An existing file must have been read first and must be unchanged, so the
// model cannot clobber content it never saw. A new file needs no read.
func (s *Set) Write(path, content string) (string, error) {
	p := s.pol()
	abs, err := p.Resolve(path)
	if err != nil {
		return "", err
	}
	rel := p.Rel(abs)

	info, statErr := os.Stat(abs)
	exists := statErr == nil
	if exists {
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", rel)
		}
		if err := s.obs.Check(abs, info); err != nil {
			return "", fmt.Errorf("%s: %w", rel, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(abs, []byte(content)); err != nil {
		return "", err
	}
	if err := s.obs.RecordPath(abs); err != nil {
		return "", err
	}

	verb := "created"
	if exists {
		verb = "replaced"
	}
	return fmt.Sprintf("%s %s (%s, %d lines)", verb, rel, humanBytes(int64(len(content))), countLines(content)), nil
}

// Edit replaces a literal snippet in an existing file.
//
// Without replaceAll the snippet must appear exactly once: an ambiguous edit is
// far more likely to be a mistake than an intention, and the error tells the
// model how many matches it found.
func (s *Set) Edit(path, oldText, newText string, replaceAll bool) (string, error) {
	if oldText == "" {
		return "", fmt.Errorf("old text is empty")
	}
	p := s.pol()
	abs, err := p.Resolve(path)
	if err != nil {
		return "", err
	}
	rel := p.Rel(abs)

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", rel)
	}
	if info.Size() > maxReadFile {
		return "", fmt.Errorf("%w: %s is %s", ErrTooLarge, rel, humanBytes(info.Size()))
	}
	if err := s.obs.Check(abs, info); err != nil {
		return "", fmt.Errorf("%s: %w", rel, err)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, oldText)
	switch {
	case count == 0:
		return "", fmt.Errorf("%s: %w", rel, ErrNotFound)
	case count > 1 && !replaceAll:
		return "", fmt.Errorf("%s: the text appears %d times; add more context or set replace_all", rel, count)
	}

	updated := strings.Replace(content, oldText, newText, 1)
	if replaceAll {
		updated = strings.ReplaceAll(content, oldText, newText)
	}
	if err := writeFileAtomic(abs, []byte(updated)); err != nil {
		return "", err
	}
	if err := s.obs.RecordPath(abs); err != nil {
		return "", err
	}

	replaced := 1
	if replaceAll {
		replaced = count
	}
	return fmt.Sprintf("edited %s (%d replacement(s), %d → %d lines)", rel, replaced, countLines(content), countLines(updated)), nil
}
