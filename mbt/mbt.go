// Package mbt replays Quint ITF traces against an implementation through an
// Adapter and reports, per trace, the first step where the implementation
// diverges from the model.
//
// Traces must be generated with `quint run --mbt` so that each state carries
// mbt::actionTaken and mbt::nondetPicks. State 0 is replayed as a step whose
// action is the init action, and the observed state is checked after every
// step, including state 0.
package mbt

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/sa-ha/quint-mbt-go/itf"
)

const (
	varAction = "mbt::actionTaken"
	varPicks  = "mbt::nondetPicks"
)

// Action executes one model action on the implementation, reading its
// nondeterministic choices from picks.
type Action func(picks Picks) error

// Adapter connects one implementation instance to the model. Replay creates a
// fresh Adapter for every trace, so no state leaks between traces.
type Adapter interface {
	// Actions maps model action names, as recorded in mbt::actionTaken, to
	// implementation calls. It is called once per trace.
	Actions() map[string]Action
	// State returns the observed implementation state keyed by model
	// variable name. Only the returned variables are compared; values are
	// converted with itf.FromGo.
	State() (map[string]any, error)
}

// Picks holds the nondeterministic choices of one step. Quint records every
// nondet name of the spec in every state; only those the step actually used
// are present here, unwrapped from their Option encoding.
type Picks struct{ m map[string]itf.Value }

// Get returns the pick named name, if the step made it.
func (p Picks) Get(name string) (itf.Value, bool) {
	v, ok := p.m[name]
	return v, ok
}

// Value is Get that fails when the pick is absent.
func (p Picks) Value(name string) (itf.Value, error) {
	if v, ok := p.m[name]; ok {
		return v, nil
	}
	return itf.Value{}, fmt.Errorf("mbt: nondet pick %q is absent", name)
}

// Int returns the pick named name as an int.
func (p Picks) Int(name string) (int, error) {
	v, err := p.Value(name)
	if err != nil {
		return 0, err
	}
	n, err := v.AsInt()
	if err != nil {
		return 0, fmt.Errorf("mbt: nondet pick %q: %w", name, err)
	}
	return n, nil
}

// String returns the pick named name as a string.
func (p Picks) String(name string) (string, error) {
	v, err := p.Value(name)
	if err != nil {
		return "", err
	}
	s, err := v.AsString()
	if err != nil {
		return "", fmt.Errorf("mbt: nondet pick %q: %w", name, err)
	}
	return s, nil
}

// Names returns the names of the picks present, sorted.
func (p Picks) Names() []string {
	names := make([]string, 0, len(p.m))
	for n := range p.m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (p Picks) format() string {
	var parts []string
	for _, n := range p.Names() {
		parts = append(parts, n+" = "+p.m[n].String())
	}
	return strings.Join(parts, ", ")
}

// FailureKind classifies why a trace failed.
type FailureKind string

// Failure kinds, in the order they can occur within a step.
const (
	BadTrace      FailureKind = "bad trace"        // undecodable file or missing mbt metadata
	Anonymous     FailureKind = "anonymous action" // mbt::actionTaken is "" (unnamed branch of any)
	UnknownAction FailureKind = "unknown action"   // the adapter maps no function to the action
	ActionError   FailureKind = "action error"     // the action function returned an error
	Panic         FailureKind = "panic"            // the adapter or implementation panicked
	StateError    FailureKind = "state error"      // State failed, returned an unknown variable, or could not be converted
	StateMismatch FailureKind = "state mismatch"   // observed state differs from the trace
)

// Failure describes the first problem found in a trace.
type Failure struct {
	Kind   FailureKind
	Step   int // state index in the trace; -1 for BadTrace before any step
	Action string
	Picks  Picks
	Err    error
	Diffs  []itf.Difference
}

// Result is the verdict for one trace.
type Result struct {
	Trace string
	// Steps is the number of steps that passed.
	Steps int
	// Unchecked lists model variables the adapter never returned.
	Unchecked []string
	Failure   *Failure
}

// Passed reports whether every step of the trace matched.
func (r Result) Passed() bool { return r.Failure == nil }

// String renders the verdict; it is deterministic for a given trace and adapter.
func (r Result) String() string {
	if r.Failure == nil {
		s := fmt.Sprintf("PASS %s (%d steps)", r.Trace, r.Steps)
		if len(r.Unchecked) > 0 {
			s += "; unchecked variables: " + strings.Join(r.Unchecked, ", ")
		}
		return s
	}
	f := r.Failure
	var sb strings.Builder
	fmt.Fprintf(&sb, "FAIL %s: %s at step %d", r.Trace, f.Kind, f.Step)
	if f.Action != "" {
		fmt.Fprintf(&sb, ", action %s", f.Action)
	}
	if picks := f.Picks.format(); picks != "" {
		fmt.Fprintf(&sb, " [%s]", picks)
	}
	if f.Err != nil {
		fmt.Fprintf(&sb, "\n  %v", f.Err)
	}
	for _, d := range f.Diffs {
		fmt.Fprintf(&sb, "\n  %s", d)
	}
	if len(r.Unchecked) > 0 {
		fmt.Fprintf(&sb, "\n  unchecked variables: %s", strings.Join(r.Unchecked, ", "))
	}
	return sb.String()
}

// Replay runs one decoded trace against a fresh adapter from newAdapter.
// name identifies the trace in the result.
func Replay(name string, t *itf.Trace, newAdapter func() Adapter) Result {
	res := Result{Trace: name}
	adapter := newAdapter()
	var actions map[string]Action
	if f := protect(func() error { actions = adapter.Actions(); return nil }); f != nil {
		f.Step = 0
		res.Failure = f
		return res
	}
	seen := map[string]bool{}
	for n, state := range t.States {
		fields, _ := state.Fields()
		if f := replayStep(n, fields, actions, adapter, seen); f != nil {
			res.Failure = f
			break
		}
		res.Steps++
	}
	if len(t.States) > 0 {
		fields, _ := t.States[0].Fields()
		for name := range fields {
			if !seen[name] && name != varAction && name != varPicks {
				res.Unchecked = append(res.Unchecked, name)
			}
		}
		sort.Strings(res.Unchecked)
	}
	return res
}

func replayStep(n int, fields map[string]itf.Value, actions map[string]Action, a Adapter, seen map[string]bool) *Failure {
	fail := func(kind FailureKind, err error) *Failure { return &Failure{Kind: kind, Step: n, Err: err} }
	actionV, ok1 := fields[varAction]
	picksV, ok2 := fields[varPicks]
	if !ok1 || !ok2 {
		return fail(BadTrace, fmt.Errorf("state lacks %s or %s; generate traces with quint run --mbt", varAction, varPicks))
	}
	action, err := actionV.AsString()
	if err != nil {
		return fail(BadTrace, err)
	}
	picks, err := decodePicks(picksV)
	if err != nil {
		return fail(BadTrace, err)
	}
	f := &Failure{Step: n, Action: action, Picks: picks}
	run, ok := actions[action]
	switch {
	case action == "":
		f.Kind, f.Err = Anonymous, fmt.Errorf("name every branch of `any` in the spec")
	case !ok:
		f.Kind, f.Err = UnknownAction, fmt.Errorf("adapter has no action %q", action)
	default:
		if pf := protect(func() error { return run(picks) }); pf != nil {
			f.Kind, f.Err = pf.Kind, pf.Err
			if f.Kind == "" {
				f.Kind = ActionError
			}
		} else if sf := compareState(fields, a, seen); sf != nil {
			f.Kind, f.Err, f.Diffs = sf.Kind, sf.Err, sf.Diffs
		} else {
			return nil
		}
	}
	return f
}

// protect runs fn, turning a panic into a Panic failure and an error into a
// failure with an empty Kind for the caller to classify.
func protect(fn func() error) (f *Failure) {
	defer func() {
		if r := recover(); r != nil {
			f = &Failure{Kind: Panic, Err: fmt.Errorf("panic: %v", r)}
		}
	}()
	if err := fn(); err != nil {
		return &Failure{Err: err}
	}
	return nil
}

func decodePicks(v itf.Value) (Picks, error) {
	fields, err := v.Fields()
	if err != nil {
		return Picks{}, fmt.Errorf("%s: %w", varPicks, err)
	}
	p := Picks{m: map[string]itf.Value{}}
	for name, pick := range fields {
		payload, present, isOption := pick.Option()
		switch {
		case !isOption:
			p.m[name] = pick
		case present:
			p.m[name] = payload
		}
	}
	return p, nil
}

func compareState(fields map[string]itf.Value, a Adapter, seen map[string]bool) *Failure {
	var observed map[string]any
	if pf := protect(func() (err error) { observed, err = a.State(); return err }); pf != nil {
		if pf.Kind == "" {
			pf.Kind = StateError
		}
		return pf
	}
	names := make([]string, 0, len(observed))
	for name := range observed {
		names = append(names, name)
	}
	sort.Strings(names)
	var diffs []itf.Difference
	for _, name := range names {
		expected, ok := fields[name]
		if !ok || name == varAction || name == varPicks {
			return &Failure{Kind: StateError, Err: fmt.Errorf("adapter returned variable %q, which the trace does not have", name)}
		}
		got, err := itf.FromGo(observed[name])
		if err != nil {
			return &Failure{Kind: StateError, Err: fmt.Errorf("variable %q: %w", name, err)}
		}
		seen[name] = true
		diffs = append(diffs, itf.Diff(name, expected, got)...)
	}
	if len(diffs) > 0 {
		return &Failure{Kind: StateMismatch, Diffs: diffs}
	}
	return nil
}

// ReplayFiles replays each file in lexical order of its path.
func ReplayFiles(paths []string, newAdapter func() Adapter) []Result {
	paths = slices.Clone(paths)
	sort.Strings(paths)
	results := make([]Result, 0, len(paths))
	for _, p := range paths {
		t, err := itf.ReadFile(p)
		if err != nil {
			results = append(results, Result{Trace: p, Failure: &Failure{Kind: BadTrace, Step: -1, Err: err}})
			continue
		}
		results = append(results, Replay(p, t, newAdapter))
	}
	return results
}
