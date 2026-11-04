package shell

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

// echoCmd builds a command that prints text on the current platform.
func echoCmd(text string) string {
	if runtime.GOOS == "windows" {
		return `Write-Output "` + text + `"`
	}
	return `echo "` + text + `"`
}

func TestRunCapturesOutput(t *testing.T) {
	res, err := Run(context.Background(), Options{Command: echoCmd("hola mundo")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0 (output %q)", res.ExitCode, res.Output)
	}
	if !strings.Contains(res.Output, "hola mundo") {
		t.Errorf("output = %q, want it to contain 'hola mundo'", res.Output)
	}
	if res.TimedOut {
		t.Error("command unexpectedly timed out")
	}
	if res.Shell == "" {
		t.Error("result does not report the shell that ran the command")
	}
}

func TestRunReportsExitCode(t *testing.T) {
	res, err := Run(context.Background(), Options{Command: "exit 3"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if !res.ExitError() {
		t.Error("ExitError() = false for a failed command")
	}
}

func TestRunHonoursTimeout(t *testing.T) {
	cmd := "sleep 30"
	if runtime.GOOS == "windows" {
		cmd = "Start-Sleep -Seconds 30"
	}
	start := time.Now()
	res, err := Run(context.Background(), Options{Command: cmd, Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("command was not killed after the timeout (exit %d)", res.ExitCode)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("timeout took %s, expected it to be killed quickly", elapsed)
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1 for a timeout", res.ExitCode)
	}
}

func TestRunTruncatesOutput(t *testing.T) {
	cmd := "seq 1 5000"
	if runtime.GOOS == "windows" {
		cmd = "1..5000 | ForEach-Object { $_ }"
	}
	res, err := Run(context.Background(), Options{Command: cmd, MaxOutput: 200})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Truncated {
		t.Fatalf("output was not truncated (len %d)", len(res.Output))
	}
	if !strings.Contains(res.Output, "bytes omitted") {
		t.Errorf("truncated output is missing the omission marker: %q", res.Output)
	}
	// The beginning and the end must both survive.
	if !strings.Contains(res.Output, "1") {
		t.Errorf("truncated output lost its beginning: %q", res.Output)
	}
	if !strings.Contains(res.Output, "5000") {
		t.Errorf("truncated output lost its ending: %q", res.Output)
	}
}

func TestRunEmptyCommand(t *testing.T) {
	if _, err := Run(context.Background(), Options{Command: "   "}); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

func TestRunUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	cmd := "pwd"
	if runtime.GOOS == "windows" {
		cmd = "(Get-Location).Path"
	}
	res, err := Run(context.Background(), Options{Command: cmd, Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Output), strings.ToLower(dir)) {
		t.Errorf("output %q does not mention the working directory %q", res.Output, dir)
	}
}

func TestSummaryIncludesExitCode(t *testing.T) {
	res := Result{Command: "echo hi", Output: "hi\n", ExitCode: 0}
	got := res.Summary()
	if !strings.Contains(got, "$ echo hi") {
		t.Errorf("Summary = %q, want the command line", got)
	}
	if !strings.Contains(got, "exit code 0") {
		t.Errorf("Summary = %q, want the exit code", got)
	}
}
