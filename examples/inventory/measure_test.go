package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sa-ha/quint-mbt-go/mbt"
)

// reports replays every trace against the faithful service and each mutant and
// returns all results as text, which contains every failure detail.
func reports(t testing.TB) string {
	paths, _ := filepath.Glob(traces)
	var sb strings.Builder
	run := func(name string, newAPI func(map[string]int, int64, Clock) API) {
		for _, r := range mbt.ReplayFiles(paths, NewAdapter(newAPI)) {
			sb.WriteString(name + " " + r.String() + "\n")
		}
	}
	run("faithful", nil)
	for _, m := range mutants {
		run(m.name, m.make)
	}
	return sb.String()
}

// With REPORT_OUT set, the report text is written there so that two separate
// processes can be compared byte for byte.
func TestReportsAreDeterministic(t *testing.T) {
	first := reports(t)
	if second := reports(t); second != first {
		t.Fatal("two replays in one process produced different reports")
	}
	if out := os.Getenv("REPORT_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(first), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkReplay300Traces(b *testing.B) {
	paths, _ := filepath.Glob(traces)
	for b.Loop() {
		for _, r := range mbt.ReplayFiles(paths, NewAdapter(nil)) {
			if !r.Passed() {
				b.Fatal(r)
			}
		}
	}
}
