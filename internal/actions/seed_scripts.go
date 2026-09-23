package actions

import (
	"embed"
	"io/fs"
)

// embedFS is the small subset of fs.FS that seed_if_empty needs. Defining
// our own interface lets tests pass a synthetic FS without touching the
// embed package.
type embedFS interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

// embeddedScripts holds the default example scripts shipped inside the
// binary. The contents match the Python reference's
// data/action_scripts/*.json byte-for-byte (modulo whitespace from JSON
// indentation, which is parser-irrelevant).
//
//go:embed seeds/*.json
var embeddedScripts embed.FS

// Seeds returns the embedded scripts as a filesystem handle. Callers
// pass it to Manager.SeedIfEmpty.
//
// The returned fs is rooted at "seeds", so passing empty string lists
// every file.
func Seeds() embedFS { return embeddedScripts }

// EnsureSeedCoverage is a dev-time helper that asserts every expected
// seed is present. Called from a test; not used at runtime.
var expectedSeeds = []string{
	"click.json",
	"loop.json",
	"timed_example.json",
	"space_interval.json",
	"move_wasd_circle.json",
}
