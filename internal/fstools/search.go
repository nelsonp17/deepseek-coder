package fstools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Glob finds files whose workspace-relative path matches pattern.
//
// The pattern uses the usual glob syntax plus "**" for any number of
// directories; results are ordered by modification time, newest first, because
// the file being worked on is usually the one that just changed.
func (s *Set) Glob(pattern string, limit int) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("empty pattern")
	}
	re, err := globToRegexp(pattern)
	if err != nil {
		return "", err
	}
	if limit <= 0 || limit > maxGlobResults {
		limit = maxGlobResults
	}

	type hit struct {
		rel   string
		mtime time.Time
	}
	var hits []hit
	visited := 0
	pol := s.pol()

	walkErr := filepath.WalkDir(s.pol().Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip it, keep walking
		}
		if d.IsDir() {
			if path != s.policy.Root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		visited++
		if visited > maxScanFiles*20 {
			return fs.SkipAll
		}
		rel := pol.Rel(path)
		if !re.MatchString(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		hits = append(hits, hit{rel: rel, mtime: info.ModTime()})
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(hits) == 0 {
		return fmt.Sprintf("no files match %q", pattern), nil
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].mtime.After(hits[j].mtime) })
	truncated := false
	if len(hits) > limit {
		hits = hits[:limit]
		truncated = true
	}

	var b strings.Builder
	for _, h := range hits {
		b.WriteString(h.rel)
		b.WriteString("\n")
	}
	if truncated {
		fmt.Fprintf(&b, "… stopped at %d files; narrow the pattern\n", limit)
	}
	return b.String(), nil
}

// globToRegexp translates a glob pattern into an anchored regular expression.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	p := filepath.ToSlash(strings.TrimSpace(pattern))
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); {
		switch {
		case strings.HasPrefix(p[i:], "**/"):
			// "**/" also matches no directory at all, so "**/*.go" finds
			// "main.go" as well as "cli/main.go".
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i += 2
		case p[i] == '*':
			b.WriteString("[^/]*")
			i++
		case p[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(p[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// Grep searches file contents for a regular expression.
//
// path scopes the search to a file or directory inside the workspace; empty
// means the whole workspace.
func (s *Set) Grep(pattern, path string, limit int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	pol := s.pol()
	root := pol.Root
	if strings.TrimSpace(path) != "" {
		root, err = s.policy.Resolve(path)
		if err != nil {
			return "", err
		}
	}
	if limit <= 0 || limit > maxGrepMatches {
		limit = maxGrepMatches
	}

	var b strings.Builder
	matches, files := 0, 0
	enough := errors.New("enough")

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if files >= maxScanFiles || matches >= limit {
			return enough
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxScanFile {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || isBinary(data) {
			return nil
		}
		files++
		rel := pol.Rel(p)
		for i, line := range splitLines(string(data)) {
			if matches >= limit {
				break
			}
			line = strings.TrimSuffix(line, "\r")
			if !re.MatchString(line) {
				continue
			}
			if len(line) > 300 {
				line = truncateRunes(line, 300) + " …"
			}
			fmt.Fprintf(&b, "%s:%d: %s\n", rel, i+1, line)
			matches++
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, enough) {
		return "", walkErr
	}
	if matches == 0 {
		return fmt.Sprintf("no matches for %q", pattern), nil
	}
	if matches >= limit {
		fmt.Fprintf(&b, "… stopped at %d matches; narrow the pattern or the path\n", limit)
	}
	return b.String(), nil
}
