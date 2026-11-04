package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// configDir overrides the user config directory (used by tests).
var configDir string

// SetConfigDir overrides the config directory (used by tests).
func SetConfigDir(dir string) { configDir = dir }

// Config holds the user settings.
type Config struct {
	APIKey          string `json:"api_key"`
	Model           string `json:"model"`
	BaseURL         string `json:"base_url"`
	Lang            string `json:"lang"`             // "es" or "en"
	Thinking        string `json:"thinking"`         // "enabled" or "disabled"
	ReasoningEffort string `json:"reasoning_effort"` // "low", "high" or "max"
	Shell           string `json:"shell"`            // "on" or "off"
	ShellTimeout    int    `json:"shell_timeout"`    // seconds, 0 = default
	Context         string `json:"context"`          // "full", "light" or "off"
	Compact         string `json:"compact"`          // "auto" or "off"
	ContextWindow   int    `json:"context_window"`   // tokens, 0 = assume the default
	Sandbox         string `json:"sandbox"`          // "read-only", "workspace-write" or "full"
	Approval        string `json:"approval"`         // "ask" or "auto"
}

// SandboxMode returns how much the tools may touch: "read-only",
// "workspace-write" (default) or "full".
//
// It is one of the two independent knobs of the permission model; the other is
// the approval policy. Keeping them separate means "run without asking" and
// "write outside the project" are different decisions, not one switch.
func (c *Config) SandboxMode() string {
	switch strings.ToLower(strings.TrimSpace(c.Sandbox)) {
	case "read-only", "readonly", "read", "ro":
		return "read-only"
	case "full", "danger-full-access", "unrestricted", "none":
		return "full"
	}
	return "workspace-write"
}

// ApprovalAuto reports whether new sessions start without asking for each tool
// call. Destructive shell commands still ask.
func (c *Config) ApprovalAuto() bool {
	switch strings.ToLower(strings.TrimSpace(c.Approval)) {
	case "auto", "never", "off", "no":
		return true
	}
	return false
}

// CompactEnabled reports whether long conversations are summarised automatically
// to stay inside the model's context window.
func (c *Config) CompactEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(c.Compact)) {
	case "off", "false", "no", "0", "disabled":
		return false
	}
	return true
}

// ContextMode returns how much of the project is sent to the model on every
// request: "full" (default), "light" (no README, no git diff) or "off".
//
// The project context is re-sent on every turn, so it is the biggest lever on
// token spend: in a repo with a long README it can be several thousand tokens
// before the user has typed anything.
func (c *Config) ContextMode() string {
	switch strings.ToLower(strings.TrimSpace(c.Context)) {
	case "off", "none", "no", "0", "disabled":
		return "off"
	case "light", "lite", "minimal", "reduced":
		return "light"
	}
	return "full"
}

// ShellEnabled reports whether the model may use the run_shell tool. Every
// call still requires the user's permission; this only controls whether the
// tool is offered at all.
func (c *Config) ShellEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(c.Shell)) {
	case "off", "false", "no", "0", "disabled":
		return false
	}
	return true
}

// Load reads the config file, returning defaults if it does not exist yet.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config file.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Path returns the config file location.
func Path() (string, error) {
	dir := configDir
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "deekseep-coder", "config.json"), nil
}
