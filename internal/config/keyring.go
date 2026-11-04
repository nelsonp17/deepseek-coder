package config

import (
	"os"

	"github.com/99designs/keyring"
)

const (
	keyringService = "deepseek-coder"
	// legacyKeyringService is the name the key was stored under before the
	// product name was fixed; kept so the rename does not force a re-login.
	legacyKeyringService = "deekseep-coder"
	keyringItem          = "api-key"
)

// openKeyring opens the OS keyring with the backends available on each
// platform: Windows Credential Manager, macOS Keychain and Linux Secret
// Service. It returns an error when no backend is available (e.g. headless
// Linux without a secret service).
func openKeyring() (keyring.Keyring, error) {
	return openKeyringNamed(keyringService)
}

// openKeyringNamed opens the OS keyring under a specific service name.
func openKeyringNamed(service string) (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: service,
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

// migrateLegacyKey moves a key stored under the old service name to the new
// one, so a rename does not look like a lost login.
func migrateLegacyKey() string {
	ring, err := openKeyringNamed(legacyKeyringService)
	if err != nil {
		return ""
	}
	item, err := ring.Get(keyringItem)
	if err != nil || len(item.Data) == 0 {
		return ""
	}
	if modern, err := openKeyring(); err == nil {
		if modern.Set(keyring.Item{Key: keyringItem, Data: item.Data}) == nil {
			_ = ring.Remove(keyringItem) // moved: do not leave a stale copy
		}
	}
	return string(item.Data)
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
	// A key still stored under the old service name is the keyring too: report
	// it as such instead of claiming there is no key.
	if migrateLegacyKey() != "" {
		return "OS keyring"
	}
	if cfg.APIKey != "" {
		return "config file (plain text)"
	}
	return "not set"
}
