package main

import (
	"fmt"
	"runtime"
)

// version is stamped at build time:
//
//	go build -ldflags "-X main.version=v0.1.0" -o dist/deekseep ./cli
//
// A plain `go build` leaves it as "dev", so a binary always says which one it
// is instead of pretending to be a release.
var version = "dev"

// versionString reports what this binary is, including the toolchain that built
// it: the first question in any bug report is which build the user ran.
func versionString() string {
	return fmt.Sprintf("deekseep %s (%s/%s, %s)", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// wantsVersion reports whether the arguments ask for the version.
//
// Only a leading flag counts, so `deekseep ask "--version"` still asks a
// question instead of printing the version.
func wantsVersion(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--json", "--debug":
			continue // global flags may come first
		case "--version", "-v", "version":
			return true
		default:
			return false
		}
	}
	return false
}
