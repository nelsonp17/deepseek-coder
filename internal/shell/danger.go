package shell

import "strings"

// dangerPatterns are command shapes that destroy work. The check is a heuristic,
// not a sandbox: it exists so that "run without asking" still stops for these.
var dangerPatterns = []string{
	"rm -rf /",
	"rm -rf ~",
	"rm -rf .",
	"rm -fr /",
	":(){:|:&};:",
	"mkfs",
	"format ",
	"format.com",
	"diskpart",
	"shutdown",
	"reboot",
	"del /f",
	"del /s",
	"rd /s",
	"rmdir /s",
	"remove-item -recurse -force c:\\",
	"remove-item -recurse -force /",
	"git push --force",
	"git push -f",
	"of=/dev/",
	"chmod -r 777 /",
	"chmod -r 777 /",
}

// Danger reports why a command looks destructive, if it does.
//
// It is deliberately blunt: a false positive costs one confirmation, a false
// negative costs the user's work.
func Danger(cmdline string) (string, bool) {
	cmd := strings.ToLower(strings.Join(strings.Fields(cmdline), " "))
	if cmd == "" {
		return "", false
	}
	for _, pattern := range dangerPatterns {
		if strings.Contains(cmd, pattern) {
			return pattern, true
		}
	}
	// Downloading a script and piping it into a shell runs code nobody reviewed.
	downloads := strings.Contains(cmd, "curl ") || strings.Contains(cmd, "wget ") || strings.Contains(cmd, "invoke-webrequest")
	if downloads && (strings.Contains(cmd, "| sh") || strings.Contains(cmd, "| bash") || strings.Contains(cmd, "| pwsh") || strings.Contains(cmd, "| powershell") || strings.Contains(cmd, "iex")) {
		return "download | shell", true
	}
	return "", false
}
