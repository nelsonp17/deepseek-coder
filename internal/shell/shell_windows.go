//go:build windows

package shell

import (
	"os/exec"
	"strconv"
	"sync"
	"syscall"
)

// createNewProcessGroup is CREATE_NEW_PROCESS_GROUP. It lets the whole tree be
// addressed when a command has to be killed.
const createNewProcessGroup = 0x00000200

var (
	detectOnce sync.Once
	detected   string
)

// Detect returns the PowerShell executable to use: pwsh (PowerShell 7+) when
// it is installed, Windows PowerShell otherwise.
func Detect() string {
	detectOnce.Do(func() {
		if p, err := exec.LookPath("pwsh"); err == nil {
			detected = p
			return
		}
		if p, err := exec.LookPath("powershell"); err == nil {
			detected = p
			return
		}
		detected = "powershell.exe"
	})
	return detected
}

// Invocation returns the executable and arguments that run cmdline.
func Invocation(cmdline string) (string, []string) {
	return Detect(), []string{
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		cmdline,
	}
}

// configureProcess starts the command in its own process group.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

// killTree kills the shell and every child it spawned.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// taskkill walks the process tree, which cmd.Process.Kill() does not do:
	// killing only PowerShell would leave the real command running.
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
	if err := kill.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
