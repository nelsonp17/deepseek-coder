// Package fstools implements the model-facing file tools: read, write, edit,
// glob and grep.
//
// Two policies make them safer than shelling out:
//
//   - Every path is resolved against a root and must stay inside it, so a
//     relative path cannot walk out of the workspace.
//   - write and edit require the file to have been read first and to be
//     unchanged since then, which stops the model from editing a file it has
//     never seen or overwriting someone else's work.
package fstools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Limits applied by the tools. They keep a single call from flooding the
// model's context or from loading a huge file into memory.
const (
	maxReadFile    = 5 << 20 // 5 MB: above this a file is not a source file
	maxReadLines   = 2000
	maxLineLength  = 2000
	maxReadBytes   = 50 << 10 // 50 KB of text returned by one read
	maxGlobResults = 100
	maxGrepMatches = 250
	maxScanFiles   = 5000
	maxScanFile    = 1 << 20 // 1 MB per file while scanning
	binarySniff    = 8 << 10
)

// skipDirs are never walked: they hold generated or vendored content that would
// drown the real results.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, "target": true, ".idea": true,
}

// Errors callers can compare against.
var (
	// ErrEmptyPath is returned when a tool gets a blank path.
	ErrEmptyPath = errors.New("empty path")
	// ErrNotObserved is returned when a write/edit targets a file that was
	// never read in this session.
	ErrNotObserved = errors.New("read the file before modifying it")
	// ErrStale is returned when the file changed after it was read.
	ErrStale = errors.New("the file changed since it was read")
	// ErrNotFound is returned when the search text is not in the file.
	ErrNotFound = errors.New("text not found in the file")
	// ErrBinary is returned when a file looks binary.
	ErrBinary = errors.New("binary file")
	// ErrTooLarge is returned when a file exceeds the per-call limit.
	ErrTooLarge = errors.New("file too large")
)

// OutsideError reports a path that escapes the workspace root.
type OutsideError struct {
	Path string
	Root string
}

func (e *OutsideError) Error() string {
	return fmt.Sprintf("path %q is outside the workspace %q", e.Path, e.Root)
}

// Policy confines every operation to one root directory.
type Policy struct {
	Root string
	// Confined keeps every operation inside Root. The full sandbox mode turns it
	// off on purpose, which is why it is a field and not a constant.
	Confined bool
}

// NewPolicy resolves root to an absolute path.
func NewPolicy(root string) (Policy, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Policy{}, err
	}
	// EvalSymlinks keeps the comparison honest when the workspace itself is
	// reached through a link.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return Policy{Root: abs, Confined: true}, nil
}

// Resolve turns a caller path into an absolute path inside the root.
//
// A relative path is relative to the workspace root, not to the process working
// directory: the tools must behave the same however the CLI was launched.
func (p Policy) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ErrEmptyPath
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.Root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if p.Confined && !within(p.Root, abs) {
		return "", &OutsideError{Path: path, Root: p.Root}
	}
	// Follow a symlink so a link inside the root cannot point outside it. When
	// the target does not exist yet, resolve the parent instead.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		if p.Confined && !within(p.Root, resolved) {
			return "", &OutsideError{Path: path, Root: p.Root}
		}
		return resolved, nil
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		if p.Confined && !within(p.Root, resolved) {
			return "", &OutsideError{Path: path, Root: p.Root}
		}
		return filepath.Join(resolved, filepath.Base(abs)), nil
	}
	return abs, nil
}

// Rel returns path relative to the root, for messages.
func (p Policy) Rel(path string) string {
	if rel, err := filepath.Rel(p.Root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// within reports whether target is root or lives under it.
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stamp is the identity of a file at the moment it was read.
type stamp struct {
	size  int64
	mtime time.Time
}

// Observations remembers which files the model has already read. It is scoped
// to one session and safe for concurrent use.
type Observations struct {
	mu   sync.Mutex
	seen map[string]stamp
}

// NewObservations creates an empty observation set.
func NewObservations() *Observations {
	return &Observations{seen: map[string]stamp{}}
}

// Record marks a file as read with the given metadata.
func (o *Observations) Record(path string, info os.FileInfo) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.seen[path] = stamp{size: info.Size(), mtime: info.ModTime()}
}

// RecordPath stats a path and records it.
func (o *Observations) RecordPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	o.Record(path, info)
	return nil
}

// Check reports whether path may be modified: it must have been read, and it
// must be unchanged since it was read.
func (o *Observations) Check(path string, info os.FileInfo) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	prev, ok := o.seen[path]
	if !ok {
		return ErrNotObserved
	}
	if prev.size != info.Size() || !prev.mtime.Equal(info.ModTime()) {
		return ErrStale
	}
	return nil
}

// Set owns the policy and the observations shared by the tools.
type Set struct {
	mu     sync.RWMutex
	policy Policy
	obs    *Observations
}

// pol returns the current policy.
func (s *Set) pol() Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

// SetConfined switches between confining paths to the root and allowing any
// path. The sandbox mode drives it.
func (s *Set) SetConfined(confined bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy.Confined = confined
}

// New creates a tool set rooted at dir.
func New(dir string) (*Set, error) {
	policy, err := NewPolicy(dir)
	if err != nil {
		return nil, err
	}
	return &Set{policy: policy, obs: NewObservations()}, nil
}

// Root returns the workspace root.
func (s *Set) Root() string { return s.pol().Root }

// isBinary reports whether data looks binary.
func isBinary(data []byte) bool {
	n := len(data)
	if n > binarySniff {
		n = binarySniff
	}
	return strings.IndexByte(string(data[:n]), 0) >= 0
}

// truncateRunes cuts s to at most max bytes without splitting a rune.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func isRuneStart(c byte) bool { return c&0xC0 != 0x80 }

// writeFileAtomic writes data to path through a temporary file in the same
// directory, so a crash cannot leave a half-written source file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dsh-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// countLines counts the lines of s, counting a trailing line without newline.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
