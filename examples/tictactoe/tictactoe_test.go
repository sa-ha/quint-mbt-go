package tictactoe

import (
	"path/filepath"
	"testing"

	"quintmbt/mbt"
)

const traces = "testdata/traces/*.itf.json"

func TestTraces(t *testing.T) {
	mbt.Run(t, traces, NewAdapter)
}

type skipO struct{ *Adapter }

func (s skipO) Actions() map[string]mbt.Action {
	actions := s.Adapter.Actions()
	actions["MoveO"] = func(mbt.Picks) error { return nil }
	return actions
}

// Every trace contains a MoveO, so an adapter that ignores it must fail them all.
func TestIgnoringMoveOFailsEveryTrace(t *testing.T) {
	paths, _ := filepath.Glob(traces)
	for _, r := range mbt.ReplayFiles(paths, func() mbt.Adapter { return skipO{&Adapter{}} }) {
		if r.Passed() {
			t.Errorf("%s passed", r.Trace)
		}
	}
}
