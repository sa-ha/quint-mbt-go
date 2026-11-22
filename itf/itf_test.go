package itf

import (
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const golden = "testdata/golden"

func read(t *testing.T, name string) *Trace {
	t.Helper()
	tr, err := ReadFile(filepath.Join(golden, name))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func field(t *testing.T, state Value, name string) Value {
	t.Helper()
	v, err := state.Field(name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func pow2(n uint) *big.Int { return new(big.Int).Lsh(big.NewInt(1), n) }

func TestGoldenFilesDecode(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join(golden, "*.itf.json"))
	if len(paths) != 15 {
		t.Fatalf("expected 15 golden files, found %d", len(paths))
	}
	for _, p := range paths {
		if _, err := ReadFile(p); err != nil {
			t.Error(err)
		}
	}
}

// Expected values follow from phase0/spec/counter.qnt's init action.
func TestCounterInitState(t *testing.T) {
	for _, name := range []string{"counter_rust_0.itf.json", "counter_ts_0.itf.json"} {
		s := read(t, name).States[0]
		want := map[string]Value{
			"zcount": Int(0),
			"tags":   Set(),
			"ledger": Map(),
			"pair":   Tuple(Int(0), Str("a")),
			"rec":    Record(map[string]Value{"name": Str("x"), "n": Int(0)}),
			"opt":    None(),
			"hist":   List(),
		}
		for v, w := range want {
			if got := field(t, s, v); !Equal(got, w) {
				t.Errorf("%s %s = %s, want %s", name, v, got, w)
			}
		}
	}
	// 2^62 survives the TypeScript backend but not the Rust one (16+ digits).
	if got := field(t, read(t, "counter_ts_0.itf.json").States[0], "big"); !Equal(got, BigInt(pow2(62))) {
		t.Errorf("ts big = %s, want 2^62", got)
	}
	if got := field(t, read(t, "counter_rust_0.itf.json").States[0], "big"); got.Kind() != KindRecord {
		t.Errorf("rust big = %s; the Rust backend's bignumber.js leak is no longer reproduced", got)
	}
}

func TestBigIntBeyondInt64(t *testing.T) {
	s := read(t, "counterbig_ts_0.itf.json").States[0]
	if got := field(t, s, "big"); !Equal(got, BigInt(pow2(70))) {
		t.Errorf("big = %s, want 2^70", got)
	}
	if got := field(t, s, "neg"); !Equal(got, BigInt(new(big.Int).Neg(pow2(70)))) {
		t.Errorf("neg = %s, want -2^70", got)
	}
	if _, err := field(t, s, "big").AsInt64(); err == nil {
		t.Error("AsInt64 of 2^70 must fail")
	}
}

// The init state of phase0/spec/probe.qnt is deterministic, but the two
// backends serialize its sets and maps in different element orders.
func TestProbeInitStateEqualAcrossBackends(t *testing.T) {
	rs, ts := read(t, "probe_rust_0.itf.json").States[0], read(t, "probe_ts_0.itf.json").States[0]
	for _, v := range []string{"n", "intKeyed", "tupKeyed", "setOfTup", "setOfSet", "kind", "last"} {
		if a, b := field(t, rs, v), field(t, ts, v); !Equal(a, b) {
			t.Errorf("%s: rust %s != ts %s", v, a, b)
		}
	}
	want := Set(Tuple(Int(3), Int(1)), Tuple(Int(1), Int(2)), Tuple(Int(2), Int(9)))
	if got := field(t, rs, "setOfTup"); !Equal(got, want) {
		t.Errorf("setOfTup = %s, want %s", got, want)
	}
	wantMap := Map(Entry{Tuple(Int(2), Str("y")), Bool(true)}, Entry{Tuple(Int(1), Str("x")), Bool(false)})
	if got := field(t, ts, "tupKeyed"); !Equal(got, wantMap) {
		t.Errorf("tupKeyed = %s, want %s", got, wantMap)
	}
	if got := field(t, ts, "edge"); !Equal(got, BigInt(new(big.Int).Sub(pow2(53), big.NewInt(1)))) {
		t.Errorf("edge = %s, want 2^53-1", got)
	}
}

// Phase 0 measured the mbt::actionTaken rules; these goldens pin them.
func TestActionTakenRecording(t *testing.T) {
	actions := func(name string) []string {
		var out []string
		for _, s := range read(t, name).States {
			a, err := field(t, s, "mbt::actionTaken").AsString()
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, a)
		}
		return out
	}
	if got := actions("probe3_rust_0.itf.json"); strings.Join(got, ",") != "init,step,step,step" {
		t.Errorf("step without any: %v", got)
	}
	for _, a := range actions("probe_rust_0.itf.json")[1:] {
		if a != "outerBump" && a != "paint" && a != "idle" {
			t.Errorf("nested action recorded as %q", a)
		}
	}
	for _, a := range actions("probe2_rust_0.itf.json")[1:] {
		if a != "left" && a != "right" && a != "idle" {
			t.Errorf("nested any recorded as %q", a)
		}
	}
	anonymous := 0
	for _, a := range actions("probe4_rust_0.itf.json") {
		if a == "" {
			anonymous++
		}
	}
	if anonymous == 0 {
		t.Error("probe4 must contain anonymous steps")
	}
	if _, err := read(t, "probe3_nombt_rust_0.itf.json").States[0].Field("mbt::actionTaken"); err == nil {
		t.Error("traces without --mbt must lack mbt::actionTaken")
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"empty states":      `{"states": []}`,
		"float":             `{"states": [{"x": 1.5}]}`,
		"exponent":          `{"states": [{"x": 1e3}]}`,
		"bigint not string": `{"states": [{"x": {"#bigint": 5}}]}`,
		"bigint plus sign":  `{"states": [{"x": {"#bigint": "+5"}}]}`,
		"bigint hex":        `{"states": [{"x": {"#bigint": "0x10"}}]}`,
		"mixed tag":         `{"states": [{"x": {"#set": [], "y": 1}}]}`,
		"unknown tag":       `{"states": [{"x": {"#foo": []}}]}`,
		"bad map pair":      `{"states": [{"x": {"#map": [[1]]}}]}`,
		"duplicate key":     `{"states": [{"x": {"#map": [[1, 2], [1, 3]]}}]}`,
		"null":              `{"states": [{"x": null}]}`,
		"state not object":  `{"states": [1]}`,
		"trailing data":     `{"states": [{}]} {}`,
		"loop out of range": `{"states": [{}], "loop": 1}`,
	}
	for name, in := range cases {
		if _, err := Decode(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDecodeAcceptsPlainIntegers(t *testing.T) {
	tr, err := Decode(strings.NewReader(`{"states": [{"x": 12345678901234567890123, "y": -4}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := new(big.Int).SetString("12345678901234567890123", 10)
	if got := field(t, tr.States[0], "x"); !Equal(got, BigInt(want)) {
		t.Errorf("x = %s", got)
	}
	if got := field(t, tr.States[0], "y"); !Equal(got, Int(-4)) {
		t.Errorf("y = %s", got)
	}
}

func TestCanonicalForms(t *testing.T) {
	if !Equal(Set(Int(2), Int(1), Int(2)), Set(Int(1), Int(2))) {
		t.Error("sets must ignore order and duplicates")
	}
	if Equal(List(Int(1), Int(2)), List(Int(2), Int(1))) {
		t.Error("lists are ordered")
	}
	if Equal(List(Int(1)), Tuple(Int(1))) || Equal(List(), Set()) {
		t.Error("list, tuple, and set are distinct")
	}
	rec := Record(map[string]Value{"a": Int(1)})
	if Equal(rec, Map(Entry{Str("a"), Int(1)})) {
		t.Error("a record is not a string-keyed map")
	}
	if _, err := NewMap(Entry{Int(1), Int(1)}, Entry{Int(1), Int(2)}); err == nil {
		t.Error("duplicate keys must fail")
	}
}

func TestDiffPaths(t *testing.T) {
	exp := Record(map[string]Value{
		"stock": Map(Entry{Str("apple"), Int(2)}, Entry{Str("pear"), Int(1)}),
		"tags":  Set(Str("a"), Str("b")),
	})
	obs := Record(map[string]Value{
		"stock": Map(Entry{Str("apple"), Int(3)}),
		"tags":  Set(Str("a"), Str("c")),
	})
	var got []string
	for _, d := range Diff("s", exp, obs) {
		got = append(got, d.String())
	}
	want := []string{
		`s.stock["apple"]: expected 2, observed 3`,
		`s.stock["pear"]: missing in observed (expected 1)`,
		`s.tags{"b"}: missing in observed (expected "b")`,
		`s.tags{"c"}: unexpected in observed ("c")`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diff:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDiffHintsAtBigNumberLeak(t *testing.T) {
	leaked := field(t, read(t, "probe2_rust_0.itf.json").States[0], "b")
	ds := Diff("b", leaked, BigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil)))
	if len(ds) != 1 || ds[0].Hint == "" {
		t.Errorf("expected one difference with a hint, got %v", ds)
	}
}

type color int

func (c color) ITFValue() (Value, error) { return Variant([]string{"Red", "Green"}[c], Tuple()), nil }

func TestFromGo(t *testing.T) {
	type res struct {
		SKU    string `itf:"sku"`
		Qty    uint8  `itf:"qty"`
		Color  color  `itf:"color"`
		Note   string `itf:"-"`
		hidden int
	}
	var nilMap map[string]int
	cases := []struct {
		in   any
		want Value
	}{
		{map[string]struct{}{"b": {}, "a": {}}, Set(Str("a"), Str("b"))},
		{map[int]string{2: "x", 1: "y"}, Map(Entry{Int(1), Str("y")}, Entry{Int(2), Str("x")})},
		{nilMap, Map()},
		{[]int(nil), List()},
		{[2]bool{true, false}, List(Bool(true), Bool(false))},
		{res{SKU: "apple", Qty: 2, Color: 1, Note: "n"}, Record(map[string]Value{"sku": Str("apple"), "qty": Int(2), "color": Variant("Green", Tuple())})},
		{Tuple(Int(1), Str("a")), Tuple(Int(1), Str("a"))},
		{uint64(1 << 63), BigInt(pow2(63))},
	}
	for _, c := range cases {
		got, err := FromGo(c.in)
		if err != nil || !Equal(got, c.want) {
			t.Errorf("FromGo(%#v) = %s, %v; want %s", c.in, got, err, c.want)
		}
	}
	for _, bad := range []any{1.5, nil, (*int)(nil), make(chan int)} {
		if _, err := FromGo(bad); err == nil {
			t.Errorf("FromGo(%#v) must fail", bad)
		}
	}
}

func FuzzDecode(f *testing.F) {
	paths, _ := filepath.Glob(filepath.Join(golden, "*.itf.json"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`{"states": [{"x": {"#map": [[{"#set": [1, 1]}, {"#tup": []}]]}}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		tr, err := Decode(strings.NewReader(string(data)))
		if err != nil {
			return
		}
		for _, s := range tr.States {
			_ = s.String()
			if !Equal(s, s) {
				t.Fatalf("state not equal to itself: %s", s)
			}
			if len(Diff("s", s, s)) != 0 {
				t.Fatalf("diff of a state with itself: %s", s)
			}
		}
		if len(tr.States) >= 2 {
			a, b := tr.States[0], tr.States[1]
			if Compare(a, b) != -Compare(b, a) {
				t.Fatalf("Compare is not antisymmetric")
			}
		}
	})
}
