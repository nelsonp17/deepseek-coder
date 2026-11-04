package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/fstools"
)

// fileTestApp returns an app whose file tools are rooted at dir.
func fileTestApp(t *testing.T, dir string) *app {
	t.Helper()
	set, err := fstools.New(dir)
	if err != nil {
		t.Fatalf("fstools.New: %v", err)
	}
	a := testApp()
	a.fs = set
	return a
}

func TestAllToolsIncludesFileTools(t *testing.T) {
	a := fileTestApp(t, t.TempDir())
	names := map[string]bool{}
	for _, tool := range a.allTools() {
		names[tool.Function.Name] = true
	}
	for _, want := range []string{toolRead, toolWrite, toolEdit, toolGlob, toolGrep, shellToolName} {
		if !names[want] {
			t.Errorf("tool %q is not offered to the model", want)
		}
	}
	// Each tool needs a schema the model can fill.
	for _, tool := range a.allTools() {
		if tool.Function.Description == "" {
			t.Errorf("tool %q has no description", tool.Function.Name)
		}
		if tool.Function.Parameters == nil {
			t.Errorf("tool %q has no parameters schema", tool.Function.Name)
		}
	}
}

// Reads are not side effects: they must not prompt. Writes and shell do.
func TestToolApprovalPolicy(t *testing.T) {
	a := testApp()
	for _, name := range []string{toolRead, toolGlob, toolGrep} {
		if a.toolNeedsApproval(name) {
			t.Errorf("%q should not require approval", name)
		}
	}
	for _, name := range []string{toolWrite, toolEdit, shellToolName, "mcp_whatever"} {
		if !a.toolNeedsApproval(name) {
			t.Errorf("%q must require approval", name)
		}
	}
}

func TestExecuteFileToolRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := fileTestApp(t, dir)

	// write a new file
	out, err := a.executeTool(t.Context(), call(toolWrite, `{"path":"a.txt","content":"uno\ndos\n"}`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("write report = %q", out)
	}

	// read it back
	out, err = a.executeTool(t.Context(), call(toolRead, `{"path":"a.txt"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "1\tuno") || !strings.Contains(out, "2\tdos") {
		t.Errorf("read output = %q", out)
	}

	// edit it
	out, err = a.executeTool(t.Context(), call(toolEdit, `{"path":"a.txt","old_string":"dos","new_string":"tres"}`))
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "1 replacement") {
		t.Errorf("edit report = %q", out)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "uno\ntres\n" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestExecuteFileToolUnknownIsNotHandled(t *testing.T) {
	a := fileTestApp(t, t.TempDir())
	if _, handled, _ := a.executeFileTool(call("otra_cosa", "{}")); handled {
		t.Error("an unknown tool must not be claimed by the file tools")
	}
}

func TestExecuteFileToolRejectsBadArguments(t *testing.T) {
	a := fileTestApp(t, t.TempDir())
	if _, err := a.executeTool(t.Context(), call(toolRead, `{"path":`)); err == nil {
		t.Error("malformed JSON should fail")
	}
}

// The permission prompt must show the change, not a JSON blob.
func TestToolDetailPreviewsTheChange(t *testing.T) {
	write := call(toolWrite, `{"path":"a.txt","content":"hola\nmundo\n"}`)
	got := toolDetail(write)
	if strings.Contains(got, `"content"`) {
		t.Errorf("preview leaked raw JSON:\n%s", got)
	}
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "+ hola") {
		t.Errorf("write preview is not readable:\n%s", got)
	}

	edit := call(toolEdit, `{"path":"a.txt","old_string":"hola","new_string":"adios"}`)
	got = toolDetail(edit)
	if !strings.Contains(got, "- hola") || !strings.Contains(got, "+ adios") {
		t.Errorf("edit preview does not show both sides:\n%s", got)
	}

	// The shell keeps its own preview.
	shellCall := call(shellToolName, `{"command":"go test ./..."}`)
	if got := toolDetail(shellCall); !strings.Contains(got, "$ go test") {
		t.Errorf("shell preview = %q", got)
	}
}

func TestPreviewLinesIsBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("linea\n")
	}
	got := previewLines(b.String(), "+ ")
	if strings.Count(got, "\n") > 13 {
		t.Errorf("preview is not bounded:\n%s", got)
	}
	if !strings.Contains(got, "más") {
		t.Errorf("preview does not say it was truncated:\n%s", got)
	}
}

// call builds a ToolCall the way the API delivers one.
func call(name, args string) deepseek.ToolCall {
	tc := deepseek.ToolCall{ID: "call_" + name, Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}
