package inventory

import (
	"testing"

	"github.com/sa-ha/quint-mbt-go/mbt"
)

const traces = "testdata/traces/*.itf.json"

func TestTraces(t *testing.T) {
	for _, r := range mbt.Run(t, traces, NewAdapter(nil)) {
		if len(r.Unchecked) > 0 {
			t.Fatalf("%s: unchecked variables %v", r.Trace, r.Unchecked)
		}
	}
}
