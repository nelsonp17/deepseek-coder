package fstools

import (
	"fmt"
	"os"
	"strings"
)

// Read returns a file as numbered lines.
//
// offset is 1-based; limit caps how many lines come back. The output is bounded
// by maxReadBytes as well, so a minified file cannot flood the context.
func (s *Set) Read(path string, offset, limit int) (string, error) {
	p := s.pol()
	abs, err := p.Resolve(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	rel := p.Rel(abs)
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file", rel)
	}
	if info.Size() > maxReadFile {
		return "", fmt.Errorf("%w: %s is %s (limit %s)", ErrTooLarge, rel, humanBytes(info.Size()), humanBytes(maxReadFile))
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if isBinary(data) {
		return "", fmt.Errorf("%w: %s", ErrBinary, rel)
	}
	// The whole file was read, so the observation covers all of it.
	s.obs.Record(abs, info)

	lines := splitLines(string(data))
	total := len(lines)
	if total == 0 {
		return fmt.Sprintf("%s: empty file", rel), nil
	}
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}
	start := offset - 1
	if start >= total {
		return fmt.Sprintf("%s has %d lines; offset %d is past the end", rel, total, offset), nil
	}
	end := start + limit
	if end > total {
		end = total
	}

	var b strings.Builder
	budget := maxReadBytes
	shown := 0
	for i := start; i < end; i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		if len(line) > maxLineLength {
			line = truncateRunes(line, maxLineLength) + " …"
		}
		entry := fmt.Sprintf("%6d\t%s\n", i+1, line)
		if len(entry) > budget {
			break
		}
		budget -= len(entry)
		b.WriteString(entry)
		shown++
	}
	if shown == 0 {
		return fmt.Sprintf("%s has %d lines; use offset=1", rel, total), nil
	}
	if start+shown < total {
		fmt.Fprintf(&b, "… %d more lines (file has %d); continue with offset=%d\n", total-(start+shown), total, start+shown+1)
	}
	return b.String(), nil
}

// splitLines splits content into lines, ignoring one trailing newline so a file
// ending in "\n" does not report a phantom last line.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// humanBytes formats a size for a tool message.
func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
