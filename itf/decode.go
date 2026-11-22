package itf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"sort"
)

// Trace holds the states of one ITF file. Only States is meaningful for
// replay; Meta and Vars vary across Quint backends and runs.
type Trace struct {
	// Meta is the raw top-level "#meta" object. Quint stores a generation
	// timestamp here, so it differs between otherwise identical runs.
	Meta json.RawMessage
	// Vars is the top-level "vars" list as written. Quint's Rust backend
	// sorts it and repeats the mbt:: variables, so it must not be trusted.
	Vars []string
	// States holds one record per state, without the per-state "#meta".
	States []Value
	// Loop is the "loop" index for lasso-shaped traces, or -1.
	Loop int
}

// ReadFile is Decode applied to a file; errors are prefixed with the path.
func ReadFile(path string) (*Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// Decode reads exactly one trace and rejects trailing data and empty traces.
// Integers may be {"#bigint": "..."} or plain JSON integers, as allowed by
// https://apalache-mc.org/docs/adr/015adr-trace.html; they are never rounded
// through float64.
func Decode(r io.Reader) (*Trace, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var raw struct {
		Meta   json.RawMessage   `json:"#meta"`
		Vars   []string          `json:"vars"`
		States []json.RawMessage `json:"states"`
		Loop   *json.Number      `json:"loop"`
	}
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("itf: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("itf: unexpected data after trace")
	}
	if len(raw.States) == 0 {
		return nil, errors.New("itf: trace has no states")
	}
	t := &Trace{Meta: raw.Meta, Vars: raw.Vars, Loop: -1}
	if raw.Loop != nil {
		n, err := raw.Loop.Int64()
		if err != nil || n < 0 || n >= int64(len(raw.States)) {
			return nil, fmt.Errorf("itf: invalid loop index %s", raw.Loop)
		}
		t.Loop = int(n)
	}
	for n, rs := range raw.States {
		s, err := decodeState(rs)
		if err != nil {
			return nil, fmt.Errorf("itf: state %d: %w", n, err)
		}
		t.States = append(t.States, s)
	}
	return t, nil
}

func decodeState(data json.RawMessage) (Value, error) {
	var obj map[string]json.RawMessage
	if err := unmarshal(data, &obj); err != nil {
		return Value{}, err
	}
	if obj == nil {
		return Value{}, errors.New("state is not an object")
	}
	delete(obj, "#meta")
	fields := make(map[string]Value, len(obj))
	for _, name := range sortedKeys(obj) {
		var x any
		if err := unmarshal(obj[name], &x); err != nil {
			return Value{}, err
		}
		v, err := decodeValue(x)
		if err != nil {
			return Value{}, fmt.Errorf("variable %q: %w", name, err)
		}
		fields[name] = v
	}
	return Record(fields), nil
}

// sortedKeys makes error messages independent of map iteration order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unmarshal(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(dst)
}

var intPattern = regexp.MustCompile(`^-?[0-9]+$`)

func parseInt(s string) (Value, error) {
	if !intPattern.MatchString(s) {
		return Value{}, fmt.Errorf("invalid integer %q", s)
	}
	n, _ := new(big.Int).SetString(s, 10)
	return Value{kind: KindInt, i: n}, nil
}

func decodeValue(x any) (Value, error) {
	switch x := x.(type) {
	case bool:
		return Bool(x), nil
	case string:
		return Str(x), nil
	case json.Number:
		return parseInt(x.String())
	case []any:
		elems, err := decodeAll(x)
		return Value{kind: KindList, elems: elems}, err
	case map[string]any:
		return decodeObject(x)
	}
	return Value{}, fmt.Errorf("unsupported JSON value %v", x)
}

func decodeAll(xs []any) ([]Value, error) {
	vs := make([]Value, len(xs))
	for n, x := range xs {
		v, err := decodeValue(x)
		if err != nil {
			return nil, err
		}
		vs[n] = v
	}
	return vs, nil
}

func decodeObject(obj map[string]any) (Value, error) {
	keys := sortedKeys(obj)
	for _, key := range keys {
		if key == "" || key[0] != '#' {
			continue
		}
		x := obj[key]
		if len(obj) != 1 {
			return Value{}, fmt.Errorf("object with %q must have no other keys", key)
		}
		switch key {
		case "#bigint":
			s, ok := x.(string)
			if !ok {
				return Value{}, errors.New("#bigint must hold a string")
			}
			return parseInt(s)
		case "#unserializable":
			s, ok := x.(string)
			if !ok {
				return Value{}, errors.New("#unserializable must hold a string")
			}
			return Unserializable(s), nil
		}
		xs, ok := x.([]any)
		if !ok {
			return Value{}, fmt.Errorf("%s must hold an array", key)
		}
		switch key {
		case "#tup":
			elems, err := decodeAll(xs)
			return Value{kind: KindTuple, elems: elems}, err
		case "#set":
			elems, err := decodeAll(xs)
			return Set(elems...), err
		case "#map":
			entries := make([]Entry, len(xs))
			for n, p := range xs {
				pair, ok := p.([]any)
				if !ok || len(pair) != 2 {
					return Value{}, errors.New("#map entries must be [key, value] pairs")
				}
				kv, err := decodeAll(pair)
				if err != nil {
					return Value{}, err
				}
				entries[n] = Entry{kv[0], kv[1]}
			}
			return NewMap(entries...)
		}
		return Value{}, fmt.Errorf("unknown ITF key %q", key)
	}
	fields := make(map[string]Value, len(obj))
	for _, name := range keys {
		v, err := decodeValue(obj[name])
		if err != nil {
			return Value{}, fmt.Errorf("field %q: %w", name, err)
		}
		fields[name] = v
	}
	return Record(fields), nil
}
