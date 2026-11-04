// Package shell runs commands in the native shell of the operating system:
// PowerShell on Windows and the user's POSIX shell (bash/sh) on macOS and
// Linux.
//
// It is the execution engine behind the model's run_shell tool and the
// user-facing /run command. Every call is bounded by a timeout and by the
// amount of output it may return, and the whole process tree is killed when
// the timeout expires so a stuck command cannot leak children.
package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Limits applied to every command.
const (
	// DefaultTimeout is how long a command may run when the caller does not
	// ask for something else.
	DefaultTimeout = 120 * time.Second
	// MaxTimeout caps whatever the caller (or the model) requests.
	MaxTimeout = 600 * time.Second
	// DefaultMaxOutput is how many bytes of output are kept.
	DefaultMaxOutput = 32 * 1024
	// headRatio is the fraction of the output budget reserved for the start of
	// the output; the rest keeps the end, which is usually where the error is.
	headRatio = 0.6
)

// ErrEmptyCommand is returned when there is nothing to run.
var ErrEmptyCommand = errors.New("empty command")

// Name returns a short, human readable name for the detected shell, such as
// "pwsh", "powershell", "bash" or "sh".
func Name() string {
	path := Detect()
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSuffix(path, ".exe")
}

// Options configures one command execution.
type Options struct {
	// Command is the command line, passed to the shell verbatim.
	Command string
	// Dir is the working directory; empty means the current one.
	Dir string
	// Timeout overrides DefaultTimeout. Values above MaxTimeout are clamped.
	Timeout time.Duration
	// MaxOutput overrides DefaultMaxOutput.
	MaxOutput int
}

// Result is the outcome of a command execution.
type Result struct {
	// Shell is the executable that ran the command.
	Shell string
	// Command is the command line that was run.
	Command string
	// Output is the combined stdout and stderr.
	Output string
	// ExitCode is the process exit status (-1 when it timed out).
	ExitCode int
	// TimedOut reports whether the command was killed for exceeding Timeout.
	TimedOut bool
	// Truncated reports whether Output had to be cut down to MaxOutput.
	Truncated bool
	// Duration is how long the command ran.
	Duration time.Duration
}

// Run executes opts.Command in the platform shell.
//
// It never returns an error for a command that fails: the failure is reported
// through Result.ExitCode so the caller can hand it back to the model. An
// error is returned only when the command could not be started at all.
func Run(ctx context.Context, opts Options) (Result, error) {
	cmdline := strings.TrimSpace(opts.Command)
	if cmdline == "" {
		return Result{}, ErrEmptyCommand
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	maxOutput := opts.MaxOutput
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	executable, args := Invocation(cmdline)
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Dir = opts.Dir
	// No stdin: an interactive command hits EOF instead of hanging forever.
	cmd.Stdin = nil
	buf := newLimitedBuffer(maxOutput)
	cmd.Stdout = buf
	cmd.Stderr = buf
	configureProcess(cmd)
	// Kill the whole tree, not just the shell, when the deadline expires.
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Shell:     executable,
		Command:   cmdline,
		Output:    buf.String(),
		Truncated: buf.Truncated(),
		Duration:  time.Since(start),
	}

	if runCtx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.ExitCode = 0
	return res, nil
}

// ExitError reports whether the result represents a failed command.
func (r Result) ExitError() bool { return r.ExitCode != 0 || r.TimedOut }

// Summary renders a compact, model-friendly report of the execution.
func (r Result) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", r.Command)
	if r.TimedOut {
		fmt.Fprintf(&b, "[timed out after %s]\n", r.Duration.Round(time.Millisecond))
	} else {
		fmt.Fprintf(&b, "[exit code %d in %s]\n", r.ExitCode, r.Duration.Round(time.Millisecond))
	}
	if r.Truncated {
		b.WriteString("[output truncated]\n")
	}
	out := strings.TrimRight(r.Output, " \t\r\n")
	if out == "" {
		b.WriteString("(no output)")
	} else {
		b.WriteString(out)
	}
	b.WriteString("\n")
	return b.String()
}

// limitedBuffer is an io.Writer that keeps at most max bytes, preserving the
// beginning and the end of the output and dropping the middle.
type limitedBuffer struct {
	mu        sync.Mutex
	max       int
	head      []byte
	tail      []byte
	written   int
	truncated bool
}

func newLimitedBuffer(max int) *limitedBuffer {
	return &limitedBuffer{max: max}
}

func (b *limitedBuffer) headCap() int {
	n := int(float64(b.max) * headRatio)
	if n < 1 {
		n = 1
	}
	return n
}

func (b *limitedBuffer) tailCap() int {
	n := b.max - b.headCap()
	if n < 0 {
		n = 0
	}
	return n
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Write must report how many bytes of p it consumed, not the running total,
	// or io copies fail with "invalid write result".
	consumed := len(p)
	b.written += consumed
	if len(b.head) < b.headCap() {
		room := b.headCap() - len(b.head)
		n := len(p)
		if n > room {
			n = room
		}
		b.head = append(b.head, p[:n]...)
		p = p[n:]
	}
	if len(p) == 0 {
		return consumed, nil
	}
	b.truncated = true
	b.tail = append(b.tail, p...)
	if over := len(b.tail) - b.tailCap(); over > 0 {
		b.tail = b.tail[over:]
	}
	return consumed, nil
}

// Truncated reports whether any output was dropped.
func (b *limitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// String returns the kept output, joining the head and the tail with a marker
// when the middle was dropped.
func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.truncated {
		return string(b.head)
	}
	head := trimToRuneBoundary(b.head, false)
	tail := trimToRuneBoundary(b.tail, true)
	dropped := b.written - len(head) - len(tail)
	if dropped < 0 {
		dropped = 0
	}
	return string(head) +
		fmt.Sprintf("\n... [%d bytes omitted] ...\n", dropped) +
		string(tail)
}

// trimToRuneBoundary cuts b to a valid UTF-8 prefix or suffix.
func trimToRuneBoundary(b []byte, fromStart bool) []byte {
	if fromStart {
		for len(b) > 0 && !isRuneStart(b[0]) {
			b = b[1:]
		}
		return b
	}
	for len(b) > 0 && !isRuneStart(b[len(b)-1]) {
		b = b[:len(b)-1]
	}
	return b
}

func isRuneStart(c byte) bool { return c&0xC0 != 0x80 }
