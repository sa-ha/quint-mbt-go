package mbt

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Run replays every trace matching the glob pattern and reports each failing
// trace through t.Error. It fails the test when the pattern matches nothing,
// so a wrong path cannot pass silently, and logs the model variables that
// passing traces never compared.
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
	passing, partial, unchecked := 0, 0, map[string]bool{}
	for _, r := range results {
		if !r.Passed() {
			t.Error(r.String())
			continue
		}
		passing++
		if len(r.Unchecked) > 0 {
			partial++
		}
		for _, v := range r.Unchecked {
			unchecked[v] = true
		}
	}
	if partial > 0 {
		names := slices.Sorted(maps.Keys(unchecked))
		t.Logf("mbt: %d of %d passing traces left variables unchecked: %s", partial, passing, strings.Join(names, ", "))
	}
	return results
}
