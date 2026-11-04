package config

import "testing"

func TestContextMode(t *testing.T) {
	cases := map[string]string{
		"":          "full",
		"full":      "full",
		"light":     "light",
		"LITE":      "light",
		"minimal":   "light",
		"off":       "off",
		"NONE":      "off",
		" disabled": "off",
		"basura":    "full",
	}
	for in, want := range cases {
		c := &Config{Context: in}
		if got := c.ContextMode(); got != want {
			t.Errorf("ContextMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellEnabled(t *testing.T) {
	for _, in := range []string{"", "on", "true", "yes"} {
		if !(&Config{Shell: in}).ShellEnabled() {
			t.Errorf("ShellEnabled(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"off", "OFF", "false", "no", "0", "disabled"} {
		if (&Config{Shell: in}).ShellEnabled() {
			t.Errorf("ShellEnabled(%q) = true, want false", in)
		}
	}
}
