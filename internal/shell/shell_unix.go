//go:build !windows

package shell

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
)

var (
	detectOnce sync.Once
	detected   string
)

// Detect returns the POSIX shell to use: $SHELL when it points at a real
// executable, then bash, then sh.
func Detect() string {
	detectOnce.Do(func() {
		if s := os.Getenv("SHELL"); s != "" {
			if info, err := os.Stat(s); err == nil && !info.IsDir() {
				detected = s
				return
			}
		}
		for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				detected = candidate
				return
			}
		}
		detected = "/bin/sh"
	})
	return detected
}

// Invocation returns the executable and arguments that run cmdline.
func Invocation(cmdline string) (string, []string) {
	return Detect(), []string{"-c", cmdline}
}

// configureProcess puts the command in its own process group so the whole tree
// can be signalled at once.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree kills the process group of the command.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Negative pid means "the whole process group".
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
