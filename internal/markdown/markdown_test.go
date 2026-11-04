package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// text strips the ANSI styling so assertions can look at the plain output.
func text(s string) string { return stripANSI(s) }

func TestHeadingsLoseTheirMarkers(t *testing.T) {
	got := text(Render("# Título\n## Sección\n### Menor", 60))
	for _, want := range []string{"Título", "Sección", "Menor"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "#") {
		t.Errorf("heading markers leaked into the output:\n%s", got)
	}
}

func TestInlineFormattingIsApplied(t *testing.T) {
	src := "Esto es **negrita**, *cursiva*, `código` y ~~tachado~~."
	raw := Render(src, 80)
	got := text(raw)

	for _, marker := range []string{"**", "`", "~~"} {
		if strings.Contains(got, marker) {
			t.Errorf("marker %q leaked into %q", marker, got)
		}
	}
	for _, want := range []string{"negrita", "cursiva", "código", "tachado"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

// Styling is only emitted when a colour profile is available; under `go test`
// lipgloss detects a non-TTY and disables colours, so this test forces one.
func TestStylingIsEmittedWithAColourProfile(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	raw := Render("**negrita** y `código`", 60)
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("no ANSI styling emitted: %q", raw)
	}
	if !strings.Contains(stripANSI(raw), "negrita") {
		t.Errorf("styling lost the text: %q", raw)
	}
}

func TestLinksKeepTextAndURL(t *testing.T) {
	got := text(Render("Mira [la documentación](https://example.com/docs) aquí.", 80))
	if strings.Contains(got, "](") {
		t.Errorf("link syntax leaked: %q", got)
	}
	if !strings.Contains(got, "la documentación") || !strings.Contains(got, "https://example.com/docs") {
		t.Errorf("link lost its parts: %q", got)
	}
}

func TestFencedCodeBlock(t *testing.T) {
	src := "Antes\n\n```go\nfunc main() {\n\tprintln(\"hola\")\n}\n```\n\nDespués"
	out := Render(src, 60)
	got := text(out)

	if !strings.Contains(got, "go") {
		t.Errorf("language label missing:\n%s", got)
	}
	if !strings.Contains(got, "func main() {") {
		t.Errorf("code body missing:\n%s", got)
	}
	if !strings.Contains(got, "┌") || !strings.Contains(got, "└") {
		t.Errorf("code block is not boxed:\n%s", got)
	}
	if strings.Contains(got, "```") {
		t.Errorf("fence leaked:\n%s", got)
	}
	if !strings.Contains(got, "Antes") || !strings.Contains(got, "Después") {
		t.Errorf("surrounding text lost:\n%s", got)
	}
}

// A streamed answer can end in the middle of a code block; rendering must not
// panic or drop what arrived so far.
func TestUnclosedFenceDuringStreaming(t *testing.T) {
	got := text(Render("Texto\n\n```go\nfmt.Println(\"a\")\n", 60))
	if !strings.Contains(got, `fmt.Println("a")`) {
		t.Errorf("partial code block lost its content:\n%s", got)
	}
}

func TestLists(t *testing.T) {
	src := "- uno\n- dos\n  - anidado\n1. primero\n2. segundo\n- [ ] pendiente\n- [x] hecho"
	got := text(Render(src, 60))

	for _, want := range []string{"• uno", "• dos", "anidado", "1. primero", "2. segundo", "☐ pendiente", "☑ hecho"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Nested items must be indented further than their parent.
	lines := strings.Split(got, "\n")
	for _, ln := range lines {
		if strings.Contains(ln, "anidado") {
			parent := ""
			for _, p := range lines {
				if strings.Contains(p, "• dos") {
					parent = p
				}
			}
			if len(ln)-len(strings.TrimLeft(ln, " ")) <= len(parent)-len(strings.TrimLeft(parent, " ")) {
				t.Errorf("nested item is not indented:\n%s", got)
			}
		}
	}
}

func TestTableFitsTheWidth(t *testing.T) {
	src := "| Comando | Descripción |\n|---|---|\n| `/copy` | copia **todo** |"
	out := Render(src, 50)
	got := text(out)

	if !strings.Contains(got, "Comando") || !strings.Contains(got, "Descripción") {
		t.Errorf("table header missing:\n%s", got)
	}
	if !strings.Contains(got, "copia todo") {
		t.Errorf("table cell not rendered:\n%s", got)
	}
	for _, ln := range strings.Split(got, "\n") {
		if w := lipgloss.Width(ln); w > 50 {
			t.Errorf("line is %d wide, over the 50 limit: %q", w, ln)
		}
	}
}

func TestQuoteAndRule(t *testing.T) {
	got := text(Render("> una cita\n> segunda línea\n\n---", 40))
	if !strings.Contains(got, "│ una cita segunda línea") {
		t.Errorf("quote not rendered:\n%s", got)
	}
	if !strings.Contains(got, "─") {
		t.Errorf("horizontal rule missing:\n%s", got)
	}
}

func TestParagraphsWrapToWidth(t *testing.T) {
	src := "palabra " + strings.Repeat("larga ", 40)
	for _, line := range strings.Split(text(Render(src, 40)), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line is %d wide, over the 40 limit: %q", w, line)
		}
	}
}

func TestLongWordIsBroken(t *testing.T) {
	got := text(Render(strings.Repeat("x", 120), 30))
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Errorf("long word not broken: %q", line)
		}
	}
}

func TestPlainStripsInlineMarkers(t *testing.T) {
	if got, want := Plain("**hola** y `mundo`"), "hola y mundo"; got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

// Styled spans must not inject a space where the source had none: punctuation
// glued to an emphasised word has to stay glued.
func TestPunctuationStaysAttached(t *testing.T) {
	got := text(Render("el proyecto **deepseek-coder**. Y la rama `main`.", 80))
	for _, bad := range []string{" .", " ,", " :", " )"} {
		if strings.Contains(got, bad) {
			t.Errorf("space inserted before punctuation %q in %q", bad, got)
		}
	}
	for _, want := range []string{"deepseek-coder.", "main."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

// Emoji and accents must be measured by display width, not bytes.
func TestUnicodeWidths(t *testing.T) {
	got := text(Render("¡Hola! 👋 **listo** para trabajar", 40))
	if !strings.Contains(got, "👋") {
		t.Errorf("emoji lost: %q", got)
	}
	if strings.Contains(got, "\ufffd") {
		t.Errorf("text was cut mid-rune: %q", got)
	}
}

func TestEmptyAndBlankInput(t *testing.T) {
	if got := Render("", 40); got != "" {
		t.Errorf("Render(\"\") = %q, want empty", got)
	}
	if got := Render("\n\n\n", 40); got != "" {
		t.Errorf("Render(blank) = %q, want empty", got)
	}
}
