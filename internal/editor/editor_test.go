package editor

import (
	"io"
	"strings"
	"testing"
)

func TestParseKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  keyKind
	}{
		{"enter", "\r", keyEnter},
		{"newline", "\n", keyEnter},
		{"tab", "\t", keyTab},
		{"alt-enter", "\x1b\r", keyAltEnter},
		{"alt-newline", "\x1b\n", keyAltEnter},
		{"backspace", "\x7f", keyBackspace},
		{"ctrl-c", "\x03", keyCtrlC},
		{"ctrl-d", "\x04", keyCtrlD},
		{"left", "\x1b[D", keyLeft},
		{"right", "\x1b[C", keyRight},
		{"up", "\x1b[A", keyUp},
		{"down", "\x1b[B", keyDown},
		{"home", "\x1b[H", keyHome},
		{"end", "\x1b[F", keyEnd},
		{"rune", "a", keyRune},
		{"utf8", "ñ", keyRune},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := parseKey(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("parseKey: %v", err)
			}
			if k.kind != tt.want {
				t.Errorf("parseKey(%q) = %v, want %v", tt.input, k.kind, tt.want)
			}
		})
	}
}

func TestParseKeyAltChar(t *testing.T) {
	k, err := parseKey(strings.NewReader("\x1ba"))
	if err != nil {
		t.Fatal(err)
	}
	if k.kind != keyRune || k.r != 'a' {
		t.Errorf("alt+char = %+v, want rune 'a'", k)
	}
}

func TestReadMultiline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "hola\r", "hola"},
		{"alt-enter newline", "hola\x1b\rmundo\r", "hola\nmundo"},
		{"backspace", "hola\x7f\r", "hol"},
		{"utf8", "código\r", "código"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			got, err := readMultiline(strings.NewReader(tt.input), &out, "> ", nil)
			if err != nil {
				t.Fatalf("readMultiline: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadMultilineCompletion(t *testing.T) {
	completer := func(line string, pos int) Completion {
		if strings.HasPrefix(line, "@") {
			return Completion{Prefix: line, Candidates: []string{"@src/main.go"}}
		}
		if strings.HasPrefix(line, "/") {
			return Completion{Prefix: line, Candidates: []string{"/login", "/list-models"}}
		}
		return Completion{}
	}

	t.Run("single candidate completes on tab", func(t *testing.T) {
		var out strings.Builder
		got, err := readMultiline(strings.NewReader("@src\t\r"), &out, "> ", completer)
		if err != nil {
			t.Fatalf("readMultiline: %v", err)
		}
		if got != "@src/main.go" {
			t.Errorf("got %q, want @src/main.go", got)
		}
	})

	t.Run("multiple candidates insert common prefix", func(t *testing.T) {
		var out strings.Builder
		got, err := readMultiline(strings.NewReader("/l\t\r"), &out, "> ", completer)
		if err != nil {
			t.Fatalf("readMultiline: %v", err)
		}
		// common prefix of /login and /list-models is /l
		if got != "/l" {
			t.Errorf("got %q, want /l", got)
		}
	})
}

func TestReadMultilineEOF(t *testing.T) {
	var out strings.Builder
	if _, err := readMultiline(strings.NewReader("\x03"), &out, "> ", nil); err != io.EOF {
		t.Errorf("ctrl-c: got %v, want io.EOF", err)
	}
	if _, err := readMultiline(strings.NewReader("\x04"), &out, "> ", nil); err != io.EOF {
		t.Errorf("ctrl-d: got %v, want io.EOF", err)
	}
}

func TestApplyCompletion(t *testing.T) {
	buf := []rune("hola @src")
	got := applyCompletion(buf, len(buf), "@src", "@src/main.go")
	if string(got) != "hola @src/main.go" {
		t.Errorf("applyCompletion = %q", string(got))
	}
}

func TestCommonPrefix(t *testing.T) {
	if got := commonPrefix([]string{"/login", "/list-models", "/logout"}); got != "/l" {
		t.Errorf("commonPrefix = %q, want /l", got)
	}
	if got := commonPrefix([]string{"/a", "/b"}); got != "/" {
		t.Errorf("commonPrefix = %q, want /", got)
	}
	if got := commonPrefix(nil); got != "" {
		t.Errorf("commonPrefix(nil) = %q, want empty", got)
	}
}

func TestInsertRemove(t *testing.T) {
	buf := insert(nil, 0, 'a')
	buf = insert(buf, 1, 'b')
	buf = insert(buf, 1, 'X')
	if got := string(buf); got != "aXb" {
		t.Errorf("insert = %q, want aXb", got)
	}
	buf = remove(buf, 1)
	if got := string(buf); got != "ab" {
		t.Errorf("remove = %q, want ab", got)
	}
}
