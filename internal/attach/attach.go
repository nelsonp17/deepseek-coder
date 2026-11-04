// Package attach turns @path references in a user message into file contents.
//
// The message the user typed is never rewritten: the file contents are
// appended in their own section, so the instructions stay readable and the
// reference stays visible to the model. Resolve returns the enriched payload
// only; what the chat shows is up to the caller, so a 30 KB file does not end
// up in the transcript or in the SQLite history.
package attach

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Limits applied to one resolution pass.
const (
	// DefaultMaxFileBytes is the size above which a file is not attached.
	DefaultMaxFileBytes = 32 * 1024
	// DefaultMaxTotalBytes is the budget shared by every file in one message.
	DefaultMaxTotalBytes = 64 * 1024
	// binarySniff is how many bytes are inspected for NUL to spot a binary.
	binarySniff = 8 * 1024
)

// Reasons an attachment was skipped.
var (
	ErrNotFound  = errors.New("not found")
	ErrDirectory = errors.New("it is a directory")
	ErrTooLarge  = errors.New("too large")
	ErrBinary    = errors.New("binary file")
	ErrBudget    = errors.New("attachment budget exhausted")
	ErrRead      = errors.New("could not read")
)

// Attachment is one reference, resolved or skipped.
type Attachment struct {
	// Path is the path as the user wrote it.
	Path string
	// Bytes is how many bytes of the file were attached.
	Bytes int
	// Lines is the line count of the attached content.
	Lines int
	// Truncated reports whether the content had to be cut to fit the budget.
	Truncated bool
	// Err is non-nil when the file was not attached.
	Err error
	// Content is the attached text; empty when Err is set.
	Content string
	// Language is the fence hint derived from the extension.
	Language string
}

// Options configures Resolve.
type Options struct {
	MaxFileBytes  int
	MaxTotalBytes int
	// Header opens the attachments section. Receives the total size attached.
	Header func(size string) string
	// FileTag wraps the path that precedes each file block.
	FileTag func(path string) string
}

// Resolve finds the @path references in msg and returns the payload to send to
// the model: the original message followed by an attachments section. It also
// returns one Attachment per reference, in order, including the skipped ones.
func Resolve(msg string, opts Options) (string, []Attachment) {
	refs := References(msg)
	if len(refs) == 0 {
		return msg, nil
	}
	maxFile := opts.MaxFileBytes
	if maxFile <= 0 {
		maxFile = DefaultMaxFileBytes
	}
	maxTotal := opts.MaxTotalBytes
	if maxTotal <= 0 {
		maxTotal = DefaultMaxTotalBytes
	}
	header := opts.Header
	if header == nil {
		header = func(size string) string { return fmt.Sprintf("Attached files (%s):", size) }
	}
	fileTag := opts.FileTag
	if fileTag == nil {
		fileTag = func(path string) string { return fmt.Sprintf("<file: %s>", path) }
	}

	atts := make([]Attachment, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	total := 0

	for _, ref := range refs {
		key := dedupeKey(ref.Path)
		if seen[key] {
			continue
		}
		seen[key] = true

		remaining := maxTotal - total
		if remaining <= 0 {
			atts = append(atts, Attachment{Path: ref.Path, Err: ErrBudget})
			continue
		}
		a := load(ref.Path, maxFile, remaining)
		if a.Err == nil {
			total += a.Bytes
		}
		atts = append(atts, a)
	}

	var b strings.Builder
	b.WriteString(msg)
	b.WriteString("\n\n")
	b.WriteString(header(HumanBytes(total)))
	b.WriteString("\n")
	for _, a := range atts {
		if a.Err != nil {
			continue // failures are reported to the user, not to the model
		}
		fence := fenceFor(a.Content)
		b.WriteString("\n")
		b.WriteString(fileTag(a.Path))
		b.WriteString("\n")
		b.WriteString(fence)
		b.WriteString(a.Language)
		b.WriteString("\n")
		b.WriteString(a.Content)
		if !strings.HasSuffix(a.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(fence)
		b.WriteString("\n")
	}
	return b.String(), atts
}

// load reads one file, applying the size, binary and budget rules.
func load(path string, maxFile, remaining int) Attachment {
	a := Attachment{Path: path, Language: Language(path)}

	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			a.Err = ErrNotFound
		} else {
			a.Err = fmt.Errorf("%w: %v", ErrRead, err)
		}
		return a
	}
	if info.IsDir() {
		a.Err = ErrDirectory
		return a
	}
	// Check the size before reading: this is what keeps a 20 MB executable out
	// of memory.
	if info.Size() > int64(maxFile) {
		a.Bytes = int(info.Size())
		a.Err = ErrTooLarge
		return a
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		a.Err = fmt.Errorf("%w: %v", ErrRead, err)
		return a
	}
	if isBinary(data) {
		a.Bytes = len(data)
		a.Err = ErrBinary
		return a
	}
	if len(data) > maxFile {
		data = truncateRunes(data, maxFile)
		a.Truncated = true
	}
	if len(data) > remaining {
		data = truncateRunes(data, remaining)
		a.Truncated = true
	}

	a.Bytes = len(data)
	a.Lines = bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		a.Lines++
	}
	a.Content = string(data)
	return a
}

// Reference is one @path found in a message.
type Reference struct {
	// Path is the path with sentence punctuation stripped.
	Path string
	// Quoted reports whether it was written as @"..." or @'...'.
	Quoted bool
}

// References extracts every @path reference from a message.
//
// A reference must open a token: "@main.go" in "revisa @main.go" counts, but
// the "@" in "user@example.com" does not. Quoted references (@"/my file.go" or
// @'/my file.go') are supported for paths with spaces.
func References(msg string) []Reference {
	var out []Reference
	for i := 0; i < len(msg); i++ {
		if msg[i] != '@' {
			continue
		}
		if i > 0 && !isSpace(msg[i-1]) {
			continue
		}
		if i+1 >= len(msg) {
			continue
		}
		if q := msg[i+1]; q == '"' || q == '\'' {
			if end := strings.IndexByte(msg[i+2:], q); end >= 0 {
				if p := msg[i+2 : i+2+end]; p != "" {
					out = append(out, Reference{Path: p, Quoted: true})
				}
				i += end + 2
			}
			continue
		}
		j := i + 1
		for j < len(msg) && !isSpace(msg[j]) {
			j++
		}
		if p := trimToken(msg[i+1 : j]); p != "" {
			out = append(out, Reference{Path: p})
		}
		i = j
	}
	return out
}

// trimToken drops the sentence punctuation that follows a path in prose, so
// "@main.go." still resolves to main.go.
func trimToken(token string) string {
	token = strings.TrimRight(token, ".,;:!?")
	return strings.TrimSpace(token)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// dedupeKey makes "./a.go" and "a.go" the same reference.
func dedupeKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// isBinary reports whether data looks like binary content.
func isBinary(data []byte) bool {
	n := len(data)
	if n > binarySniff {
		n = binarySniff
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

// truncateRunes cuts data to at most max bytes without splitting a rune.
func truncateRunes(data []byte, max int) []byte {
	if len(data) <= max {
		return data
	}
	cut := data[:max]
	for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	// The last rune may be incomplete even though its first byte is a start
	// byte; drop it so the result is valid UTF-8.
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// fenceFor returns a backtick run longer than any run inside content, so a file
// that itself contains ``` cannot close the block early.
func fenceFor(content string) string {
	longest, cur := 0, 0
	for _, r := range content {
		if r == '`' {
			cur++
			if cur > longest {
				longest = cur
			}
			continue
		}
		cur = 0
	}
	n := longest + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}

// HumanBytes formats a byte count for humans.
func HumanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

var langByExt = map[string]string{
	".go": "go", ".md": "markdown", ".json": "json", ".yml": "yaml", ".yaml": "yaml",
	".js": "javascript", ".jsx": "javascript", ".ts": "typescript", ".tsx": "typescript",
	".py": "python", ".rb": "ruby", ".rs": "rust", ".java": "java", ".kt": "kotlin",
	".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp", ".cs": "csharp",
	".sh": "bash", ".bash": "bash", ".zsh": "bash", ".ps1": "powershell", ".psm1": "powershell",
	".sql": "sql", ".html": "html", ".htm": "html", ".css": "css", ".scss": "scss",
	".toml": "toml", ".ini": "ini", ".xml": "xml", ".lua": "lua", ".php": "php",
	".swift": "swift", ".dart": "dart", ".ex": "elixir", ".exs": "elixir", ".r": "r",
	".dockerfile": "dockerfile", ".mk": "makefile",
}

// Language returns the fence language hint for a path.
func Language(path string) string {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	}
	return langByExt[strings.ToLower(filepath.Ext(path))]
}
