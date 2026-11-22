package mbt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quintmbt/itf"
)

// The tests model a counter spec with `var n: int` and `var log: str`, whose
// actions are init (n' = 0) and add (n' = n + k for a nondet k).
func state(action string, n int, picks string) string {
	return fmt.Sprintf(`{"mbt::actionTaken": %q, "mbt::nondetPicks": %s, "n": {"#bigint": "%d"}, "log": "x"}`, action, picks, n)
}

func trace(t *testing.T, states ...string) *itf.Trace {
	t.Helper()
	tr, err := itf.Decode(strings.NewReader(`{"states": [` + strings.Join(states, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

const noPicks = `{"k": {"tag": "None", "value": {"#tup": []}}}`

func pick(k int) string {
	return fmt.Sprintf(`{"k": {"tag": "Some", "value": {"#bigint": "%d"}}}`, k)
}

type counter struct {
	n       int
	actions map[string]Action
	state   func() (map[string]any, error)
}

func newCounter() *counter {
	c := &counter{}
	c.actions = map[string]Action{
		"init": func(Picks) error { c.n = 0; return nil },
		"add": func(p Picks) error {
			k, err := p.Int("k")
			c.n += k
			return err
		},
	}
	c.state = func() (map[string]any, error) { return map[string]any{"n": c.n}, nil }
	return c
}

func (c *counter) Actions() map[string]Action     { return c.actions }
func (c *counter) State() (map[string]any, error) { return c.state() }

func adapterOf(c *counter) func() Adapter { return func() Adapter { return c } }

func goodTrace(t *testing.T) *itf.Trace {
	return trace(t, state("init", 0, noPicks), state("add", 2, pick(2)), state("add", 5, pick(3)))
}

func TestReplayPasses(t *testing.T) {
	r := Replay("t", goodTrace(t), func() Adapter { return newCounter() })
	if !r.Passed() || r.Steps != 3 {
		t.Fatalf("%s", r)
	}
	if strings.Join(r.Unchecked, ",") != "log" {
		t.Errorf("unchecked = %v, want [log]", r.Unchecked)
	}
}

func TestReplayFailureKinds(t *testing.T) {
	cases := []struct {
		name   string
		tr     *itf.Trace
		tweak  func(c *counter)
		kind   FailureKind
		step   int
		detail string
	}{
		{"mismatch", goodTrace(t), func(c *counter) {
			c.actions["add"] = func(p Picks) error { c.n++; return nil }
		}, StateMismatch, 1, "n: expected 2, observed 1"},
		{"anonymous", trace(t, state("init", 0, noPicks), state("", 0, noPicks)), nil, Anonymous, 1, "name every branch"},
		{"unknown action", trace(t, state("init", 0, noPicks), state("sub", 0, pick(1))), nil, UnknownAction, 1, `no action "sub"`},
		{"action error", goodTrace(t), func(c *counter) {
			c.actions["add"] = func(Picks) error { return errors.New("rejected") }
		}, ActionError, 1, "rejected"},
		{"missing pick", trace(t, state("init", 0, noPicks), state("add", 0, noPicks)), nil, ActionError, 1, `pick "k" is absent`},
		{"action panic", goodTrace(t), func(c *counter) {
			c.actions["add"] = func(Picks) error { panic("boom") }
		}, Panic, 1, "panic: boom"},
		{"state panic", goodTrace(t), func(c *counter) {
			c.state = func() (map[string]any, error) { panic("bad") }
		}, Panic, 0, "panic: bad"},
		{"state error", goodTrace(t), func(c *counter) {
			c.state = func() (map[string]any, error) { return nil, errors.New("down") }
		}, StateError, 0, "down"},
		{"unknown variable", goodTrace(t), func(c *counter) {
			c.state = func() (map[string]any, error) { return map[string]any{"n": c.n, "typo": 1}, nil }
		}, StateError, 0, `"typo"`},
		{"mbt variable", goodTrace(t), func(c *counter) {
			c.state = func() (map[string]any, error) { return map[string]any{"mbt::actionTaken": "init"}, nil }
		}, StateError, 0, "mbt::actionTaken"},
		{"unconvertible", goodTrace(t), func(c *counter) {
			c.state = func() (map[string]any, error) { return map[string]any{"n": 1.5}, nil }
		}, StateError, 0, "cannot convert"},
		{"no mbt metadata", trace(t, `{"n": {"#bigint": "0"}}`), nil, BadTrace, 0, "--mbt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCounter()
			if tc.tweak != nil {
				tc.tweak(c)
			}
			r := Replay("t", tc.tr, adapterOf(c))
			if r.Passed() {
				t.Fatal("expected a failure")
			}
			f := r.Failure
			if f.Kind != tc.kind || f.Step != tc.step || r.Steps != tc.step {
				t.Errorf("got %s at step %d after %d steps, want %s at %d", f.Kind, f.Step, r.Steps, tc.kind, tc.step)
			}
			if !strings.Contains(r.String(), tc.detail) {
				t.Errorf("report lacks %q:\n%s", tc.detail, r)
			}
		})
	}
}

func TestPicksUnwrapOptionOnly(t *testing.T) {
	picks := `{"a": {"tag": "Some", "value": {"tag": "None", "value": {"#tup": []}}},
		"b": {"tag": "None", "value": {"#tup": []}}, "c": {"#bigint": "7"}}`
	var got Picks
	c := newCounter()
	c.actions["init"] = func(p Picks) error { got = p; return nil }
	Replay("t", trace(t, state("init", 0, picks)), adapterOf(c))
	if strings.Join(got.Names(), ",") != "a,c" {
		t.Fatalf("names = %v", got.Names())
	}
	if a, _ := got.Get("a"); !itf.Equal(a, itf.None()) {
		t.Errorf("a = %s; exactly one Option layer must be removed", a)
	}
	if n, err := got.Int("c"); err != nil || n != 7 {
		t.Errorf("c = %d, %v; non-Option picks pass through", n, err)
	}
	if _, err := got.String("c"); err == nil {
		t.Error("String of an int pick must fail")
	}
}

func writeTrace(t *testing.T, dir, name string, states ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(`{"states": [`+strings.Join(states, ",")+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReplayFilesFreshAdapterAndOrder(t *testing.T) {
	dir := t.TempDir()
	b := writeTrace(t, dir, "b.itf.json", state("init", 0, noPicks), state("add", 4, pick(4)))
	a := writeTrace(t, dir, "a.itf.json", state("init", 0, noPicks), state("add", 1, pick(1)))
	bad := writeTrace(t, dir, "c.itf.json")
	created := 0
	results := ReplayFiles([]string{bad, b, a}, func() Adapter {
		created++
		c := newCounter()
		// init does not reset n, so a reused adapter would carry n into the next trace.
		c.actions["init"] = func(Picks) error { return nil }
		return c
	})
	var got []string
	for _, r := range results {
		got = append(got, filepath.Base(r.Trace)+":"+fmt.Sprint(r.Passed()))
	}
	if strings.Join(got, " ") != "a.itf.json:true b.itf.json:true c.itf.json:false" {
		t.Errorf("results = %v", got)
	}
	if created != 2 {
		t.Errorf("created %d adapters, want one per decodable trace", created)
	}
	if f := results[2].Failure; f.Kind != BadTrace || f.Step != -1 {
		t.Errorf("bad file: %s", results[2])
	}
}

func TestReportIsDeterministic(t *testing.T) {
	run := func() string {
		c := newCounter()
		c.state = func() (map[string]any, error) {
			return map[string]any{"n": map[string]int{"z": 1, "a": 2, "m": 3}}, nil
		}
		return Replay("t", goodTrace(t), adapterOf(c)).String()
	}
	first := run()
	for range 20 {
		if got := run(); got != first {
			t.Fatalf("report changed:\n%s\n---\n%s", first, got)
		}
	}
}
