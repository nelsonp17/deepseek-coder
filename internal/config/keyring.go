package config

import (
	"os"

	"github.com/99designs/keyring"
)

const (
	keyringService = "deekseep-coder"
	keyringItem    = "api-key"
)

// openKeyring opens the OS keyring with the backends available on each
// platform: Windows Credential Manager, macOS Keychain and Linux Secret
// Service. It returns an error when no backend is available (e.g. headless
// Linux without a secret service).
func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: keyringService,
		AllowedBackends: []keyring.BackendType{
			keyring.WinCredBackend,
			keyring.KeychainBackend,
			keyring.SecretServiceBackend,
		},
	})
}

// GetAPIKey resolves the API key with this precedence:
// environment variable > OS keyring > config file (legacy).
func GetAPIKey(cfg *Config) string {
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		return v
	}
	if ring, err := openKeyring(); err == nil {
		if item, err := ring.Get(keyringItem); err == nil && len(item.Data) > 0 {
			return string(item.Data)
		}
	}
	return cfg.APIKey
}

// SetAPIKey stores the key in the OS keyring. If no keyring backend is
// available it falls back to the config file.
func SetAPIKey(cfg *Config, key string) error {
	if ring, err := openKeyring(); err == nil {
		if err := ring.Set(keyring.Item{Key: keyringItem, Data: []byte(key)}); err == nil {
			cfg.APIKey = "" // the key now lives in the keyring
			return cfg.Save()
		}
	}
	cfg.APIKey = key
	return cfg.Save()
}

// DeleteAPIKey removes the key from the keyring and the config file.
func DeleteAPIKey(cfg *Config) error {
	if ring, err := openKeyring(); err == nil {
		_ = ring.Remove(keyringItem)
	}
	cfg.APIKey = ""
	return cfg.Save()
}

// APIKeySource reports where the API key is currently stored.
func APIKeySource(cfg *Config) string {
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		return "environment variable"
	}
	if ring, err := openKeyring(); err == nil {
		if item, err := ring.Get(keyringItem); err == nil && len(item.Data) > 0 {
			return "OS keyring"
		}
	}
	if cfg.APIKey != "" {
		return "config file (plain text)"
	}
	return "not set"
}
