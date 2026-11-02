package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
