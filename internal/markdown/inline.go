package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// span is a run of text sharing one style.
type span struct {
	text  string
	style lipgloss.Style
}

// parseInline splits a line into styled spans, understanding `code`, **bold**,
// __bold__, *italic*, _italic_, ~~strike~~ and [links](url).
func parseInline(s string) []span {
	var out []span
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, span{text: buf.String()})
			buf.Reset()
		}
	}

	for i := 0; i < len(s); {
		switch {
		case s[i] == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				flush()
				out = append(out, span{text: s[i+1 : i+1+end], style: styleCode})
				i += end + 2
				continue
			}
			buf.WriteByte(s[i])
			i++

		case strings.HasPrefix(s[i:], "**") || strings.HasPrefix(s[i:], "__"):
			mark := s[i : i+2]
			if end := strings.Index(s[i+2:], mark); end >= 0 {
				flush()
				out = append(out, span{text: s[i+2 : i+2+end], style: styleBold})
				i += end + 4
				continue
			}
			buf.WriteString(mark)
			i += 2

		case strings.HasPrefix(s[i:], "~~"):
			if end := strings.Index(s[i+2:], "~~"); end >= 0 {
				flush()
				out = append(out, span{text: s[i+2 : i+2+end], style: styleStrike})
				i += end + 4
				continue
			}
			buf.WriteString("~~")
			i += 2

		case s[i] == '*' || s[i] == '_':
			mark := s[i]
			if end := strings.IndexByte(s[i+1:], mark); end >= 0 && end > 0 {
				flush()
				out = append(out, span{text: s[i+1 : i+1+end], style: styleItalic})
				i += end + 2
				continue
			}
			buf.WriteByte(s[i])
			i++

		case s[i] == '[':
			if text, url, n, ok := parseLink(s[i:]); ok {
				flush()
				out = append(out, span{text: text, style: styleLink})
				if url != "" && url != text {
					out = append(out, span{text: " (" + url + ")", style: styleURL})
				}
				i += n
				continue
			}
			buf.WriteByte(s[i])
			i++

		default:
			j := i + 1
			for j < len(s) && !isSpecial(s[j]) {
				j++
			}
			buf.WriteString(s[i:j])
			i = j
		}
	}
	flush()
	return out
}

func isSpecial(c byte) bool {
	switch c {
	case '`', '*', '_', '~', '[':
		return true
	}
	return false
}

// parseLink parses "[text](url)" at the start of s.
func parseLink(s string) (text, url string, consumed int, ok bool) {
	if len(s) < 4 || s[0] != '[' {
		return "", "", 0, false
	}
	close := strings.IndexByte(s, ']')
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return "", "", 0, false
	}
	end := strings.IndexByte(s[close+2:], ')')
	if end < 0 {
		return "", "", 0, false
	}
	return s[1:close], s[close+2 : close+2+end], close + 2 + end + 1, true
}

// token is one word (or an explicit line break) with the style it carries and
// whether the source had whitespace right before it.
type token struct {
	text    string
	style   lipgloss.Style
	space   bool // whitespace preceded this word
	newline bool
}

// wrapSpans lays the spans out over lines of at most width cells, splitting on
// spaces. Styles survive a line break because the words carry them.
func wrapSpans(spans []span, width int) []string {
	if width < 10 {
		width = 10
	}
	tokens := tokenize(spans)

	var lines []string
	var cur strings.Builder
	curWidth := 0

	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curWidth = 0
	}

	for _, tok := range tokens {
		if tok.newline {
			if curWidth > 0 {
				flush()
			}
			continue
		}
		wl := lipgloss.Width(tok.text)

		// A single word wider than the line has to be broken.
		if wl > width && curWidth == 0 {
			for _, chunk := range chunkWord(tok.text, width) {
				cur.WriteString(tok.style.Render(chunk))
				curWidth = lipgloss.Width(chunk)
				flush()
			}
			continue
		}
		// Only break at a real word boundary: punctuation glued to the previous
		// word ("**code**.") must not start a new line.
		if curWidth > 0 && tok.space && curWidth+1+wl > width {
			flush()
		}
		if curWidth > 0 && tok.space {
			cur.WriteByte(' ')
			curWidth++
		}
		cur.WriteString(tok.style.Render(tok.text))
		curWidth += wl
	}
	if curWidth > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// tokenize splits spans into words, remembering where the source had spaces.
// Without the space flag, splitting on span boundaries would insert spaces that
// were never there ("**code**." would render as "code .").
func tokenize(spans []span) []token {
	var out []token
	var word strings.Builder
	var wordStyle lipgloss.Style
	spacePending := false

	flush := func() {
		if word.Len() > 0 {
			out = append(out, token{text: word.String(), style: wordStyle, space: spacePending})
			word.Reset()
			spacePending = false
		}
	}

	for _, sp := range spans {
		for _, r := range sp.text {
			switch r {
			case ' ', '\t':
				flush()
				spacePending = true
			case '\n':
				flush()
				out = append(out, token{newline: true})
				spacePending = false
			default:
				if word.Len() == 0 {
					wordStyle = sp.style
				}
				word.WriteRune(r)
			}
		}
	}
	flush()
	return out
}

// chunkWord cuts a word into pieces of at most width runes.
func chunkWord(s string, width int) []string {
	runes := []rune(s)
	var out []string
	for len(runes) > 0 {
		n := width
		if n > len(runes) {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}
