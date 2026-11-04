package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestWantsVersion(t *testing.T) {
	asked := [][]string{
		{"--version"},
		{"-v"},
		{"version"},
		{"--json", "--version"},
		{"--debug", "version"},
	}
	for _, args := range asked {
		if !wantsVersion(args) {
			t.Errorf("wantsVersion(%v) = false, want true", args)
		}
	}

	// A question that happens to look like a flag must not be hijacked.
	notAsked := [][]string{
		nil,
		{},
		{"ask", "--version"},
		{"ask", "que es --version"},
		{"config", "get"},
		{"--pipe"},
	}
	for _, args := range notAsked {
		if wantsVersion(args) {
			t.Errorf("wantsVersion(%v) = true, want false", args)
		}
	}
}

func TestVersionString(t *testing.T) {
	old := version
	version = "v9.9.9"
	defer func() { version = old }()

	got := versionString()
	for _, want := range []string{"deekseep", "v9.9.9", runtime.GOOS, runtime.GOARCH, runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("versionString() = %q, missing %q", got, want)
		}
	}
}

// An unstamped build must say so instead of pretending to be a release.
func TestVersionDefaultsToDev(t *testing.T) {
	if version == "" {
		t.Fatal("version is empty: a build must always identify itself")
	}
}
