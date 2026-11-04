package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The transcript must never receive the file contents: only the API does.
func TestResolveUserMessageSplitsDisplayAndPayload(t *testing.T) {
	dir := t.TempDir()
	content := "package demo\n\nfunc Hello() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	um := testApp().resolveUserMessage("revisa @demo.go por favor")

	if !strings.HasPrefix(um.Display, "revisa @demo.go por favor") {
		t.Errorf("display lost the original message: %q", um.Display)
	}
	if strings.Contains(um.Display, "func Hello") {
		t.Errorf("file content leaked into the display:\n%s", um.Display)
	}
	if !strings.Contains(um.Display, "demo.go") {
		t.Errorf("display is missing the attachment note:\n%s", um.Display)
	}
	if !strings.Contains(um.Payload, "func Hello") {
		t.Errorf("payload is missing the content:\n%s", um.Payload)
	}
	if um.Note == "" {
		t.Error("note is empty for an attached file")
	}
}

func TestResolveUserMessageWithoutReferences(t *testing.T) {
	um := testApp().resolveUserMessage("hola, sin adjuntos")
	if um.Display != "hola, sin adjuntos" || um.Payload != "hola, sin adjuntos" {
		t.Errorf("message was altered: %+v", um)
	}
	if um.Note != "" {
		t.Errorf("note = %q, want empty", um.Note)
	}
}

// When every reference fails the payload must stay clean, but the user still
// needs to know why nothing was attached.
func TestResolveUserMessageAllSkipped(t *testing.T) {
	t.Chdir(t.TempDir())

	um := testApp().resolveUserMessage("mira @no-existe.go")
	if um.Payload != "mira @no-existe.go" {
		t.Errorf("payload = %q, want it untouched", um.Payload)
	}
	if !strings.Contains(um.Display, "omitido") {
		t.Errorf("display does not report the skip:\n%s", um.Display)
	}
	if !strings.Contains(um.Display, "no encontrado") {
		t.Errorf("display does not report the reason:\n%s", um.Display)
	}
}

func TestPipeQuestion(t *testing.T) {
	a := testApp()

	t.Run("prompt only", func(t *testing.T) {
		got, err := a.pipeQuestion([]string{"revisa", "esto"}, strings.NewReader(""), false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != "revisa esto" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("stdin only", func(t *testing.T) {
		got, err := a.pipeQuestion(nil, strings.NewReader("contenido del archivo"), false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != "contenido del archivo" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("both", func(t *testing.T) {
		got, err := a.pipeQuestion([]string{"explica esto"}, strings.NewReader("linea 1\nlinea 2"), false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !strings.HasPrefix(got, "explica esto") {
			t.Errorf("prompt missing: %q", got)
		}
		if !strings.Contains(got, "linea 1") || !strings.Contains(got, "linea 2") {
			t.Errorf("stdin content missing: %q", got)
		}
		if !strings.Contains(got, "entrada estándar") {
			t.Errorf("stdin block is not labelled: %q", got)
		}
	})

	t.Run("nothing at all", func(t *testing.T) {
		if _, err := a.pipeQuestion(nil, strings.NewReader("   \n"), false); err == nil {
			t.Fatal("expected a usage error")
		}
	})

	// With a terminal on stdin nothing is read, so the command cannot hang.
	t.Run("interactive does not read stdin", func(t *testing.T) {
		got, err := a.pipeQuestion([]string{"hola"}, strings.NewReader("ignorado"), true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != "hola" {
			t.Errorf("got %q", got)
		}
	})
}
