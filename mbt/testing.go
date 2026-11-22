package mbt

import (
	"path/filepath"
	"testing"
)

// Run replays every trace matching the glob pattern and reports each failing
// trace through t.Error. It fails the test when the pattern matches nothing,
// so a wrong path cannot pass silently.
func Run(t testing.TB, pattern string, newAdapter func() Adapter) []Result {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("mbt: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("mbt: no traces match %s", pattern)
	}
	results := ReplayFiles(paths, newAdapter)
	for _, r := range results {
		if !r.Passed() {
			t.Error(r.String())
		}
	}
	return results
}
