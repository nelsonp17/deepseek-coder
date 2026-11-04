package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The project was renamed from "deekseep" to "deepseek". The data directory
// has to follow, or the user loses the config and every stored session.
func TestMigrateLegacyDataDir(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "deekseep-coder")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "config.json"), []byte(`{"model":"deepseek-flash"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := resolveAppDir(base); got != filepath.Join(base, "deepseek-coder") {
		t.Fatalf("resolveAppDir = %q, want the new name", got)
	}

	moved := filepath.Join(base, "deepseek-coder", "config.json")
	data, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("config was not migrated: %v", err)
	}
	if string(data) != `{"model":"deepseek-flash"}` {
		t.Errorf("migrated config = %s", data)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("the legacy directory is still there after migrating")
	}
}

// If the new directory already exists, the old one must be left untouched: a
// merge could overwrite newer settings with stale ones.
func TestMigrateLegacyDataDirNeverOverwrites(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "deepseek-coder")
	oldDir := filepath.Join(base, "deekseep-coder")
	for dir, content := range map[string]string{
		newDir: `{"model":"new"}`,
		oldDir: `{"model":"old"}`,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if got := resolveAppDir(base); got != newDir {
		t.Fatalf("resolveAppDir = %q, want the existing %q", got, newDir)
	}

	data, err := os.ReadFile(filepath.Join(newDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"model":"new"}` {
		t.Errorf("the existing config was overwritten: %s", data)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Error("the legacy directory was removed even though the new one existed")
	}
}

func TestMigrateLegacyDataDirWithoutAnything(t *testing.T) {
	base := t.TempDir()
	if got := resolveAppDir(base); got != filepath.Join(base, "deepseek-coder") {
		t.Errorf("resolveAppDir = %q, want the new name", got)
	}
	if _, err := os.Stat(filepath.Join(base, "deepseek-coder")); !os.IsNotExist(err) {
		t.Error("a migration with nothing to move created the target directory")
	}
}

func TestPathUsesTheNewDirectoryName(t *testing.T) {
	dir := t.TempDir()
	oldConfigDir := configDir
	configDir = dir
	defer func() { configDir = oldConfigDir }()

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "deepseek-coder", "config.json"); path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}
}
