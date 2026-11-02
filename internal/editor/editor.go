package editor

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

type keyKind int

const (
	keyRune keyKind = iota
	keyEnter
	keyAltEnter
	keyBackspace
	keyTab
	keyLeft
	keyRight
	keyUp
	keyDown
	keyHome
	keyEnd
	keyCtrlC
	keyCtrlD
)

type key struct {
	kind keyKind
	r    rune
}

// Completion is a set of candidates for the token at the cursor.
type Completion struct {
	// Prefix is the text to replace, from the trigger char to the cursor.
	Prefix string
	// Candidates are full replacement strings (they include the trigger char).
	Candidates []string
}

// Completer computes completions for the text before the cursor.
type Completer func(line string, pos int) Completion

const maxSuggestions = 8

// ReadMultiline reads a multiline input from the terminal. Enter sends the
// line, Alt+Enter inserts a newline, Tab completes using the completer.
// It returns io.EOF when the user presses Ctrl+C or Ctrl+D on an empty buffer.
func ReadMultiline(prompt string, completer Completer) (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, oldState)
	return readMultiline(os.Stdin, os.Stdout, prompt, completer)
}

// readMultiline is the testable core of ReadMultiline.
func readMultiline(in io.Reader, out io.Writer, prompt string, completer Completer) (string, error) {
	fmt.Fprint(out, prompt)
	var buf []rune
	pos := 0
	var suggestions []string
	for {
		k, err := parseKey(in)
		if err != nil {
			return "", err
		}
		switch k.kind {
		case keyEnter:
			fmt.Fprintln(out)
			return string(buf), nil
		case keyAltEnter:
			buf = insert(buf, pos, '\n')
			pos++
		case keyRune:
			buf = insert(buf, pos, k.r)
			pos++
		case keyBackspace:
			if pos > 0 {
				buf = remove(buf, pos-1)
				pos--
			}
		case keyTab:
			if completer != nil {
				comp := completer(string(buf), pos)
				switch {
				case len(comp.Candidates) == 1:
					buf = applyCompletion(buf, pos, comp.Prefix, comp.Candidates[0])
					pos = len(buf)
				case len(comp.Candidates) > 1:
					cp := commonPrefix(comp.Candidates)
					if len(cp) > len(comp.Prefix) {
						buf = applyCompletion(buf, pos, comp.Prefix, cp)
						pos = len(buf)
					}
				}
			}
		case keyLeft:
			if pos > 0 {
				pos--
			}
		case keyRight:
			if pos < len(buf) {
				pos++
			}
		case keyUp:
			pos = lineStart(buf, pos)
		case keyDown:
			pos = lineEnd(buf, pos)
		case keyHome:
			pos = 0
		case keyEnd:
			pos = len(buf)
		case keyCtrlC, keyCtrlD:
			if k.kind == keyCtrlD && len(buf) > 0 {
				continue // Ctrl+D with content: ignore
			}
			return "", io.EOF
		}

		suggestions = nil
		if completer != nil {
			comp := completer(string(buf), pos)
			suggestions = comp.Candidates
			if len(suggestions) > maxSuggestions {
				suggestions = suggestions[:maxSuggestions]
			}
		}
		render(out, prompt, buf, pos, suggestions)
	}
}

// render redraws the prompt, buffer and suggestions, placing the terminal
// cursor at pos.
func render(out io.Writer, prompt string, buf []rune, pos int, suggestions []string) {
	s := string(buf)
	totalLines := 1 + strings.Count(s, "\n")
	total := totalLines + len(suggestions)
	if total > 1 {
		fmt.Fprintf(out, "\x1b[%dA", total-1) // move up to the prompt line
	}
	fmt.Fprint(out, "\r\x1b[J") // clear from cursor to end of screen
	fmt.Fprint(out, prompt)
	fmt.Fprint(out, s)

	// Position the cursor at pos.
	before := s[:pos]
	line := strings.Count(before, "\n")
	col := len(before)
	if last := strings.LastIndex(before, "\n"); last >= 0 {
		col = len(before) - last - 1
	}
	if line == 0 {
		col += len(prompt)
	}
	endLine := strings.Count(s, "\n")
	if endLine > line {
		fmt.Fprintf(out, "\x1b[%dA", endLine-line)
	}
	if col > 0 {
		fmt.Fprintf(out, "\x1b[%dC", col)
	}

	// Show completion suggestions below the input.
	if len(suggestions) > 0 {
		fmt.Fprint(out, "\n")
		for i, sug := range suggestions {
			if i > 0 {
				fmt.Fprint(out, "\n")
			}
			fmt.Fprint(out, "\x1b[90m"+sug+"\x1b[0m")
		}
	}
}

// applyCompletion replaces the text from start (pos-len(prefix)) to pos with
// the candidate.
func applyCompletion(buf []rune, pos int, prefix, candidate string) []rune {
	start := pos - len(prefix)
	if start < 0 {
		start = 0
	}
	rest := append([]rune{}, buf[pos:]...)
	buf = append(buf[:start], []rune(candidate)...)
	return append(buf, rest...)
}

// commonPrefix returns the longest string that is a prefix of every candidate.
func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	p := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func lineStart(buf []rune, pos int) int {
	s := string(buf[:pos])
	if last := strings.LastIndex(s, "\n"); last >= 0 {
		return last + 1
	}
	return 0
}

func lineEnd(buf []rune, pos int) int {
	s := string(buf)
	rest := s[pos:]
	if next := strings.Index(rest, "\n"); next >= 0 {
		return pos + next
	}
	return len(buf)
}

func insert(buf []rune, pos int, r rune) []rune {
	buf = append(buf, 0)
	copy(buf[pos+1:], buf[pos:])
	buf[pos] = r
	return buf
}

func remove(buf []rune, pos int) []rune {
	return append(buf[:pos], buf[pos+1:]...)
}

// parseKey reads one key event from in, decoding terminal escape sequences.
func parseKey(in io.Reader) (key, error) {
	b, err := readByte(in)
	if err != nil {
		return key{}, err
	}
	switch b {
	case '\r', '\n':
		return key{kind: keyEnter}, nil
	case 0x09:
		return key{kind: keyTab}, nil
	case 0x1b: // ESC
		b2, err := readByte(in)
		if err != nil {
			return key{kind: keyRune, r: 0x1b}, nil // bare ESC
		}
		switch b2 {
		case '[': // CSI sequence
			var seq []byte
			for {
				c, err := readByte(in)
				if err != nil {
					break
				}
				seq = append(seq, c)
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
			switch string(seq) {
			case "A":
				return key{kind: keyUp}, nil
			case "B":
				return key{kind: keyDown}, nil
			case "C":
				return key{kind: keyRight}, nil
			case "D":
				return key{kind: keyLeft}, nil
			case "H":
				return key{kind: keyHome}, nil
			case "F":
				return key{kind: keyEnd}, nil
			}
			return key{kind: keyRune, r: 0x1b}, nil // unknown sequence
		case '\r', '\n':
			return key{kind: keyAltEnter}, nil
		default:
			return key{kind: keyRune, r: rune(b2)}, nil // Alt+char
		}
	case 0x7f, 0x08: // DEL / Backspace
		return key{kind: keyBackspace}, nil
	case 0x03:
		return key{kind: keyCtrlC}, nil
	case 0x04:
		return key{kind: keyCtrlD}, nil
	default:
		r, err := readRune(in, b)
		if err != nil {
			return key{}, err
		}
		return key{kind: keyRune, r: r}, nil
	}
}

func readByte(in io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(in, b[:])
	return b[0], err
}

func readRune(in io.Reader, first byte) (rune, error) {
	var buf [4]byte
	buf[0] = first
	size := 1
	switch {
	case first&0xE0 == 0xC0:
		size = 2
	case first&0xF0 == 0xE0:
		size = 3
	case first&0xF8 == 0xF0:
		size = 4
	}
	if size > 1 {
		if _, err := io.ReadFull(in, buf[1:size]); err != nil {
			return 0, err
		}
	}
	r, _ := utf8.DecodeRune(buf[:size])
	return r, nil
}
