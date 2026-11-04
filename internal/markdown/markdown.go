// Package markdown renders a practical subset of Markdown as ANSI text for a
// terminal: headings, emphasis, inline code, fenced code blocks, bullet and
// numbered lists, quotes, tables, links and horizontal rules.
//
// It is deliberately small and dependency-free: the project ships a single
// static binary, so pulling in a full CommonMark implementation plus a syntax
// highlighter is not worth it. The goal is that a model answer reads well in
// the chat log, not that it is pixel-perfect CommonMark.
package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MinWidth is the narrowest layout the renderer will produce.
const MinWidth = 20

// Styles used for the output. Colours are 256-colour codes, which every
// terminal the TUI supports can display.
var (
	styleH1       = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("39"))
	styleH2       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleH3       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	styleH4       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleItalic   = lipgloss.NewStyle().Italic(true)
	styleStrike   = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("245"))
	styleCode     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleLink     = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("33"))
	styleURL      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleQuote    = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("245"))
	styleQuoteBar = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleBullet   = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	styleRule     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleTableHd  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleTableBar = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleCodeBox  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleCodeTxt  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleLang     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// Render converts Markdown source into terminal text wrapped to width.
func Render(src string, width int) string {
	if width < MinWidth {
		width = MinWidth
	}
	r := renderer{width: width}
	return r.render(strings.ReplaceAll(src, "\r\n", "\n"))
}

// Plain strips the inline Markdown markers, for places that only want text.
func Plain(src string) string {
	var b strings.Builder
	for _, sp := range parseInline(src) {
		b.WriteString(sp.text)
	}
	return b.String()
}

type renderer struct {
	width int
}

func (r renderer) render(src string) string {
	lines := strings.Split(src, "\n")
	var out []string

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])

		switch {
		case trimmed == "":
			// Never emit two blank lines in a row.
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			i++

		case isFence(trimmed):
			block, next := r.fence(lines, i)
			out = append(out, block...)
			i = next

		case isRule(trimmed):
			out = append(out, styleRule.Render(strings.Repeat("─", r.width)))
			i++

		case isHeading(trimmed):
			block := r.heading(trimmed)
			out = append(out, block...)
			i++

		case strings.HasPrefix(trimmed, ">"):
			block, next := r.quote(lines, i)
			out = append(out, block...)
			i = next

		case isListItem(trimmed):
			block, next := r.list(lines, i)
			out = append(out, block...)
			i = next

		case i+1 < len(lines) && isTableRow(trimmed) && isTableSep(strings.TrimSpace(lines[i+1])):
			block, next := r.table(lines, i)
			out = append(out, block...)
			i = next

		default:
			block, next := r.paragraph(lines, i)
			out = append(out, block...)
			i = next
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// paragraph joins consecutive text lines and wraps them.
func (r renderer) paragraph(lines []string, i int) ([]string, int) {
	var parts []string
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || isFence(trimmed) || isRule(trimmed) || isHeading(trimmed) ||
			strings.HasPrefix(trimmed, ">") || isListItem(trimmed) {
			break
		}
		if i+1 < len(lines) && isTableRow(trimmed) && isTableSep(strings.TrimSpace(lines[i+1])) {
			break
		}
		parts = append(parts, trimmed)
		i++
	}
	// Always consume at least one line so the caller cannot loop forever.
	if len(parts) == 0 {
		parts = append(parts, strings.TrimSpace(lines[i]))
		i++
	}
	return wrapSpans(parseInline(strings.Join(parts, " ")), r.width), i
}

// heading renders an ATX heading. Inline markers are dropped so the heading
// style is not fighting with bold or italic inside it.
func (r renderer) heading(line string) []string {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	text := strings.TrimSpace(strings.TrimLeft(line[level:], "#"))

	style := styleH4
	switch level {
	case 1:
		style = styleH1
	case 2:
		style = styleH2
	case 3:
		style = styleH3
	}
	plain := Plain(text)
	if level == 1 {
		// Setext-ish look for the top level: underlined title.
		out := []string{style.Render(plain)}
		out = append(out, styleRule.Render(strings.Repeat("─", min(len([]rune(plain)), r.width))))
		return out
	}
	wrapped := wrapSpans([]span{{text: plain}}, r.width)
	for k := range wrapped {
		wrapped[k] = style.Render(stripANSI(wrapped[k]))
	}
	return wrapped
}

func (r renderer) fence(lines []string, i int) ([]string, int) {
	info := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i]), "`~"))
	var out []string
	if info != "" {
		out = append(out, styleCodeBox.Render("┌ ")+styleLang.Render(info))
	} else {
		out = append(out, styleCodeBox.Render("┌"))
	}
	i++

	var body []string
	for i < len(lines) && !isCloseFence(strings.TrimSpace(lines[i])) {
		body = append(body, lines[i])
		i++
	}
	if i < len(lines) {
		i++ // consume the closing fence
	}
	body = dedent(body)
	for _, ln := range body {
		out = append(out, styleCodeBox.Render("│ ")+styleCodeTxt.Render(ln))
	}
	out = append(out, styleCodeBox.Render("└"))
	return out, i
}

func (r renderer) quote(lines []string, i int) ([]string, int) {
	var parts []string
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, ">") {
			break
		}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
		i++
	}
	text := Plain(strings.Join(parts, " "))
	wrapped := wrapSpans([]span{{text: text}}, r.width-2)
	out := make([]string, 0, len(wrapped))
	for _, ln := range wrapped {
		out = append(out, styleQuoteBar.Render("│ ")+styleQuote.Render(ln))
	}
	return out, i
}

// list renders a run of bullet or numbered items, including nesting and task
// list checkboxes.
func (r renderer) list(lines []string, i int) ([]string, int) {
	var out []string
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !isListItem(trimmed) {
			break
		}
		indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		level := indent / 2
		if level > 3 {
			level = 3
		}

		marker, text := splitListItem(trimmed)
		text = taskMarker(text)

		pad := strings.Repeat("  ", level)
		hang := pad + strings.Repeat(" ", lipgloss.Width(marker)+1)
		avail := r.width - len(hang)
		if avail < MinWidth/2 {
			avail = MinWidth / 2
		}

		wrapped := wrapSpans(parseInline(text), avail)
		for k, ln := range wrapped {
			if k == 0 {
				out = append(out, pad+styleBullet.Render(marker)+" "+ln)
			} else {
				out = append(out, hang+ln)
			}
		}
		i++
	}
	return out, i
}

// table renders a pipe table, shrinking columns so the result fits the width.
func (r renderer) table(lines []string, i int) ([]string, int) {
	var rows [][]string
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !isTableRow(trimmed) {
			break
		}
		i++
		if isTableSep(trimmed) {
			continue
		}
		rows = append(rows, splitRow(trimmed))
	}
	if len(rows) == 0 {
		return nil, i
	}

	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	widths := make([]int, cols)
	for _, row := range rows {
		for c, cell := range row {
			if w := lipgloss.Width(Plain(cell)); w > widths[c] {
				widths[c] = w
			}
		}
	}
	// Shrink the widest columns until the table fits (leaving room for the
	// "│ " and " │ " separators).
	fits := func() bool {
		total := 0
		for _, w := range widths {
			total += w
		}
		return total+3*cols+2 <= r.width
	}
	for !fits() {
		widest := 0
		for c := 1; c < cols; c++ {
			if widths[c] > widths[widest] {
				widest = c
			}
		}
		if widths[widest] <= 4 {
			break
		}
		widths[widest]--
	}

	renderRow := func(row []string, header bool) string {
		cells := make([]string, cols)
		for c := 0; c < cols; c++ {
			cell := ""
			if c < len(row) {
				cell = Plain(row[c])
			}
			cell = truncate(cell, widths[c])
			cell += strings.Repeat(" ", widths[c]-lipgloss.Width(cell))
			if header {
				cell = styleTableHd.Render(cell)
			}
			cells[c] = cell
		}
		return styleTableBar.Render("│ ") + strings.Join(cells, styleTableBar.Render(" │ ")) + styleTableBar.Render(" │")
	}

	out := []string{renderRow(rows[0], true)}
	sep := make([]string, cols)
	for c := range sep {
		sep[c] = styleTableBar.Render(strings.Repeat("─", widths[c]))
	}
	out = append(out, styleTableBar.Render("│ ")+strings.Join(sep, styleTableBar.Render(" │ "))+styleTableBar.Render(" │"))
	for _, row := range rows[1:] {
		out = append(out, renderRow(row, false))
	}
	return out, i
}

// --- block helpers -------------------------------------------------------

func isFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func isCloseFence(line string) bool {
	return line == "```" || line == "~~~" ||
		(strings.HasPrefix(line, "```") && strings.TrimSpace(strings.TrimLeft(line, "`")) == "") ||
		(strings.HasPrefix(line, "~~~") && strings.TrimSpace(strings.TrimLeft(line, "~")) == "")
}

func isRule(line string) bool {
	body := strings.ReplaceAll(line, " ", "")
	if len(body) < 3 {
		return false
	}
	for _, c := range []string{"-", "*", "_"} {
		if body == strings.Repeat(c, len(body)) {
			return true
		}
	}
	return false
}

func isHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return false
	}
	return n == len(line) || line[n] == ' '
}

func isListItem(line string) bool {
	if len(line) < 2 {
		return false
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	// Ordered: digits followed by '.' or ')' and a space.
	n := 0
	for n < len(line) && line[n] >= '0' && line[n] <= '9' {
		n++
	}
	if n == 0 || n >= len(line) {
		return false
	}
	if line[n] != '.' && line[n] != ')' {
		return false
	}
	return n+1 < len(line) && line[n+1] == ' '
}

func splitListItem(line string) (marker, text string) {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return "•", strings.TrimSpace(line[2:])
	}
	n := 0
	for n < len(line) && line[n] >= '0' && line[n] <= '9' {
		n++
	}
	marker = line[:n+1]
	text = strings.TrimSpace(line[n+2:])
	return marker, text
}

// taskMarker turns "- [ ] foo" / "- [x] foo" into a checkbox.
func taskMarker(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "[ ] "):
		return "☐ " + text[4:]
	case strings.HasPrefix(lower, "[x] "):
		return "☑ " + text[4:]
	}
	return text
}

func isTableRow(line string) bool {
	return strings.Contains(line, "|") && !isFence(line)
}

func isTableSep(line string) bool {
	if !strings.Contains(line, "-") || !isTableRow(line) {
		return false
	}
	for _, c := range line {
		switch c {
		case '-', ':', '|', ' ':
		default:
			return false
		}
	}
	return true
}

func splitRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for k := range parts {
		parts[k] = strings.TrimSpace(parts[k])
	}
	return parts
}

// dedent removes the common leading indentation of a code block.
func dedent(lines []string) []string {
	prefix := -1
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		n := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if prefix < 0 || n < prefix {
			prefix = n
		}
	}
	if prefix <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for k, ln := range lines {
		if len(ln) >= prefix {
			out[k] = ln[prefix:]
		} else {
			out[k] = strings.TrimLeft(ln, " \t")
		}
	}
	return out
}

func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// stripANSI removes escape sequences so text can be re-styled.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
