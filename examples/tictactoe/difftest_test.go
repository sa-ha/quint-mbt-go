package tictactoe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sa-ha/quint-mbt-go/mbt"
)

// verdict is the part of a result the differential test compares with
// quint-connect, which reports only whether and at which step a trace failed.
func verdict(r mbt.Result) string {
	if r.Passed() {
		return "pass"
	}
	return fmt.Sprintf("fail@%d", r.Failure.Step)
}

// With DIFFTEST_OUT set, the verdicts are written there as
// "trace<TAB>verdict<TAB>kind" lines for difftest/compare.py.
func TestDifferentialVerdicts(t *testing.T) {
	paths, _ := filepath.Glob(traces)
	tampered, _ := filepath.Glob("difftest/testdata/tampered/*.itf.json")
	if len(paths) != 100 || len(tampered) != 80 {
		t.Fatalf("expected 100 frozen and 80 tampered traces, found %d and %d", len(paths), len(tampered))
	}
	var lines []string
	for _, r := range mbt.ReplayFiles(append(paths, tampered...), NewAdapter) {
		kind := ""
		if r.Failure != nil {
			kind = string(r.Failure.Kind)
		}
		name := strings.TrimSuffix(filepath.Base(r.Trace), ".itf.json")
		lines = append(lines, name+"\t"+verdict(r)+"\t"+kind)
	}
	if out := os.Getenv("DIFFTEST_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportsAreDeterministic(t *testing.T) {
	paths, _ := filepath.Glob(traces)
	tampered, _ := filepath.Glob("difftest/testdata/tampered/*.itf.json")
	all := append(paths, tampered...)
	text := func() string {
		var sb strings.Builder
		for _, r := range mbt.ReplayFiles(all, NewAdapter) {
			sb.WriteString(r.String() + "\n")
		}
		return sb.String()
	}
	first := text()
	if text() != first {
		t.Fatal("two replays in one process produced different reports")
	}
	if out := os.Getenv("REPORT_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(first), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
