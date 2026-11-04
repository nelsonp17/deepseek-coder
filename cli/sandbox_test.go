package main

import (
	"path/filepath"
	"strings"
	"testing"

	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/fstools"
)

func toolCall(name, args string) deepseek.ToolCall {
	var tc deepseek.ToolCall
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

func TestSandboxDenialInReadOnly(t *testing.T) {
	a := testApp()
	if err := a.setSandbox("read-only"); err != nil {
		t.Fatalf("setSandbox: %v", err)
	}

	for _, name := range []string{toolWrite, toolEdit, shellToolName} {
		denial, blocked := a.sandboxDenial(toolCall(name, "{}"))
		if !blocked {
			t.Errorf("%s should be denied in read-only mode", name)
			continue
		}
		if !strings.Contains(denial, "sandbox:") || !strings.Contains(denial, "read-only") {
			t.Errorf("denial for %s lacks the marker: %q", name, denial)
		}
	}

	// Reading is the whole point of read-only mode.
	for _, name := range []string{toolRead, toolGlob, toolGrep} {
		if _, blocked := a.sandboxDenial(toolCall(name, "{}")); blocked {
			t.Errorf("%s must be allowed in read-only mode", name)
		}
	}
}

func TestSandboxDenialOffWhenNotReadOnly(t *testing.T) {
	a := testApp()
	for _, mode := range []string{"workspace-write", "full"} {
		if err := a.setSandbox(mode); err != nil {
			t.Fatalf("setSandbox(%q): %v", mode, err)
		}
		for _, name := range []string{toolWrite, toolEdit, shellToolName} {
			if _, blocked := a.sandboxDenial(toolCall(name, "{}")); blocked {
				t.Errorf("%s must not be denied in %s mode", name, mode)
			}
		}
	}
}

func TestCallNeedsApproval(t *testing.T) {
	a := testApp()

	// Asking mode: writes and shell ask, reads do not.
	a.approvalAuto = false
	if !a.callNeedsApproval(toolCall(toolWrite, "{}")) {
		t.Error("write must ask when approval is on")
	}
	if a.callNeedsApproval(toolCall(toolRead, "{}")) {
		t.Error("read must not ask")
	}
	if !a.callNeedsApproval(toolCall(shellToolName, `{"command":"go test ./..."}`)) {
		t.Error("shell must ask when approval is on")
	}

	// Auto mode: nothing asks, except a destructive command.
	a.approvalAuto = true
	if a.callNeedsApproval(toolCall(toolWrite, "{}")) {
		t.Error("write must not ask in auto mode")
	}
	if a.callNeedsApproval(toolCall(shellToolName, `{"command":"go test ./..."}`)) {
		t.Error("a safe command must not ask in auto mode")
	}
	if !a.callNeedsApproval(toolCall(shellToolName, `{"command":"rm -rf /"}`)) {
		t.Error("a destructive command must always ask, even in auto mode")
	}
}

func TestSetSandboxValidatesAndApplies(t *testing.T) {
	a := testApp()
	if err := a.setSandbox("no-existe"); err == nil {
		t.Error("an unknown mode should fail")
	}
	if err := a.setSandbox("read-only"); err != nil {
		t.Fatal(err)
	}
	if a.sandboxMode != "read-only" {
		t.Errorf("mode = %q", a.sandboxMode)
	}
	// "full" must lift the path confinement of the file tools.
	if err := a.setSandbox("full"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.sandboxString(), "full:") {
		t.Errorf("sandboxString does not report the new mode: %q", a.sandboxString())
	}
}

// "full" really lifts the path confinement: a write outside the workspace works.
func TestFullModeLiftsPathConfinement(t *testing.T) {
	a := testApp()
	set, err := fstools.New(".")
	if err != nil {
		t.Fatalf("fstools.New: %v", err)
	}
	a.fs = set

	outside := filepath.Join(t.TempDir(), "fuera.txt")

	// Confined (default): refused.
	if _, err := a.fs.Write(outside, "x"); err == nil {
		t.Error("a confined set must refuse a write outside the workspace")
	}

	// Full: allowed.
	if err := a.setSandbox("full"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.fs.Write(outside, "x"); err != nil {
		t.Errorf("full mode should allow the write: %v", err)
	}

	// Back to the default: refused again.
	if err := a.setSandbox("workspace-write"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.fs.Write(outside, "y"); err == nil {
		t.Error("returning to workspace-write must confine paths again")
	}
}
func TestConfigModes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "workspace-write"},
		{"read-only", "read-only"},
		{"ro", "read-only"},
		{"full", "full"},
		{"danger-full-access", "full"},
		{"workspace-write", "workspace-write"},
		{"basura", "workspace-write"},
	}
	for _, c := range cases {
		cfg := &config.Config{Sandbox: c.in}
		if got := cfg.SandboxMode(); got != c.want {
			t.Errorf("SandboxMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, in := range []string{"auto", "never", "off"} {
		if !(&config.Config{Approval: in}).ApprovalAuto() {
			t.Errorf("ApprovalAuto(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "ask", "siempre"} {
		if (&config.Config{Approval: in}).ApprovalAuto() {
			t.Errorf("ApprovalAuto(%q) = true, want false", in)
		}
	}
}
