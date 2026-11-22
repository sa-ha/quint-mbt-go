// Package itf decodes Informal Trace Format (ITF) traces produced by Quint and
// provides a canonical value model for comparing them.
//
// Values are kept in canonical form: set elements are sorted and deduplicated,
// map entries are sorted by key, and record fields are sorted by name. Two
// values that denote the same Quint value are therefore structurally equal,
// regardless of the element order a Quint backend chose when serializing.
package itf

import (
	"fmt"
	"math/big"
	"sort"
)

// Kind identifies the shape of a Value.
type Kind uint8

// Value kinds. Sum types (variants) have no kind of their own: Quint encodes
// them as records with "tag" and "value" fields, and so does this package.
const (
	KindInvalid Kind = iota
	KindBool
	KindInt
	KindStr
	KindList
	KindTuple
	KindSet
	KindMap
	KindRecord
	KindUnserializable
)

var kindNames = [...]string{"invalid", "bool", "int", "str", "list", "tuple", "set", "map", "record", "unserializable"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", k)
}

// Value is an immutable ITF value. The zero Value is invalid.
type Value struct {
	kind   Kind
	b      bool
	i      *big.Int
	s      string
	elems  []Value // list, tuple, set
	keys   []Value // map keys, parallel to elems for maps
	fields []string
}

// Entry is a key-value pair of a map value.
type Entry struct{ Key, Value Value }

// Bool returns a boolean value.
func Bool(b bool) Value { return Value{kind: KindBool, b: b} }

// Int returns an integer value.
func Int(n int64) Value { return Value{kind: KindInt, i: big.NewInt(n)} }

// BigInt returns an integer value holding a copy of n.
func BigInt(n *big.Int) Value { return Value{kind: KindInt, i: new(big.Int).Set(n)} }

// Str returns a string value.
func Str(s string) Value { return Value{kind: KindStr, s: s} }

// List returns a list value. Order is significant.
func List(elems ...Value) Value { return Value{kind: KindList, elems: clone(elems)} }

// Tuple returns a tuple value. Tuple() is Quint's unit value.
func Tuple(elems ...Value) Value { return Value{kind: KindTuple, elems: clone(elems)} }

// Set returns a set value. Duplicate elements are merged.
func Set(elems ...Value) Value {
	es := clone(elems)
	sort.Slice(es, func(a, b int) bool { return Compare(es[a], es[b]) < 0 })
	out := es[:0]
	for _, e := range es {
		if len(out) == 0 || !Equal(out[len(out)-1], e) {
			out = append(out, e)
		}
	}
	return Value{kind: KindSet, elems: out}
}

// NewMap returns a map value. It fails if two entries have equal keys.
func NewMap(entries ...Entry) (Value, error) {
	es := append([]Entry(nil), entries...)
	sort.Slice(es, func(a, b int) bool { return Compare(es[a].Key, es[b].Key) < 0 })
	v := Value{kind: KindMap, keys: make([]Value, len(es)), elems: make([]Value, len(es))}
	for n, e := range es {
		if n > 0 && Equal(es[n-1].Key, e.Key) {
			return Value{}, fmt.Errorf("itf: duplicate map key %s", e.Key)
		}
		v.keys[n], v.elems[n] = e.Key, e.Value
	}
	return v, nil
}

// Map is like NewMap but panics on duplicate keys.
func Map(entries ...Entry) Value {
	v, err := NewMap(entries...)
	if err != nil {
		panic(err)
	}
	return v
}

// Record returns a record value with the given fields.
func Record(fields map[string]Value) Value {
	v := Value{kind: KindRecord, fields: make([]string, 0, len(fields))}
	for name := range fields {
		v.fields = append(v.fields, name)
	}
	sort.Strings(v.fields)
	for _, name := range v.fields {
		v.elems = append(v.elems, fields[name])
	}
	return v
}

// Variant returns the encoding of a Quint sum-type variant: a record with a
// "tag" and a "value" field. Use Tuple() as the payload of a variant without
// arguments.
func Variant(tag string, payload Value) Value {
	return Record(map[string]Value{"tag": Str(tag), "value": payload})
}

// Some and None encode Quint's Option type.
func Some(v Value) Value { return Variant("Some", v) }

// None returns the None variant of Quint's Option type.
func None() Value { return Variant("None", Tuple()) }

// Unserializable returns an opaque value that Quint could not serialize.
func Unserializable(s string) Value { return Value{kind: KindUnserializable, s: s} }

func clone(vs []Value) []Value { return append([]Value(nil), vs...) }

// Kind returns the kind of v.
func (v Value) Kind() Kind { return v.kind }

func (v Value) want(k Kind) error {
	if v.kind != k {
		return fmt.Errorf("itf: expected %s, got %s %s", k, v.kind, v)
	}
	return nil
}

// AsBool returns the boolean held by v.
func (v Value) AsBool() (bool, error) { return v.b, v.want(KindBool) }

// AsString returns the string held by v.
func (v Value) AsString() (string, error) { return v.s, v.want(KindStr) }

// AsBigInt returns a copy of the integer held by v.
func (v Value) AsBigInt() (*big.Int, error) {
	if err := v.want(KindInt); err != nil {
		return nil, err
	}
	return new(big.Int).Set(v.i), nil
}

// AsInt64 returns the integer held by v, failing if it does not fit in int64.
func (v Value) AsInt64() (int64, error) {
	if err := v.want(KindInt); err != nil {
		return 0, err
	}
	if !v.i.IsInt64() {
		return 0, fmt.Errorf("itf: integer %s overflows int64", v.i)
	}
	return v.i.Int64(), nil
}

// AsInt is AsInt64 converted to int.
func (v Value) AsInt() (int, error) {
	n, err := v.AsInt64()
	if err == nil && int64(int(n)) != n {
		err = fmt.Errorf("itf: integer %d overflows int", n)
	}
	return int(n), err
}

// Elems returns the elements of a list, tuple, or set (sets in canonical order).
func (v Value) Elems() ([]Value, error) {
	switch v.kind {
	case KindList, KindTuple, KindSet:
		return clone(v.elems), nil
	}
	return nil, fmt.Errorf("itf: expected list, tuple, or set, got %s %s", v.kind, v)
}

// Entries returns the entries of a map in canonical key order.
func (v Value) Entries() ([]Entry, error) {
	if err := v.want(KindMap); err != nil {
		return nil, err
	}
	es := make([]Entry, len(v.keys))
	for n := range v.keys {
		es[n] = Entry{v.keys[n], v.elems[n]}
	}
	return es, nil
}

// Fields returns the fields of a record.
func (v Value) Fields() (map[string]Value, error) {
	if err := v.want(KindRecord); err != nil {
		return nil, err
	}
	m := make(map[string]Value, len(v.fields))
	for n, name := range v.fields {
		m[name] = v.elems[n]
	}
	return m, nil
}

// Field returns the named field of a record.
func (v Value) Field(name string) (Value, error) {
	if err := v.want(KindRecord); err != nil {
		return Value{}, err
	}
	n := sort.SearchStrings(v.fields, name)
	if n == len(v.fields) || v.fields[n] != name {
		return Value{}, fmt.Errorf("itf: record has no field %q: %s", name, v)
	}
	return v.elems[n], nil
}

// AsVariant decodes v as a sum-type variant. ok is false when v is not a
// record with exactly a string "tag" field and a "value" field.
func (v Value) AsVariant() (tag string, payload Value, ok bool) {
	if v.kind != KindRecord || len(v.fields) != 2 || v.fields[0] != "tag" || v.fields[1] != "value" ||
		v.elems[0].kind != KindStr {
		return "", Value{}, false
	}
	return v.elems[0].s, v.elems[1], true
}

// Option decodes v as Quint's Option type. ok is false when v is not an
// Option variant.
func (v Value) Option() (payload Value, present, ok bool) {
	tag, p, isVariant := v.AsVariant()
	switch {
	case isVariant && tag == "Some":
		return p, true, true
	case isVariant && tag == "None":
		return Value{}, false, true
	}
	return Value{}, false, false
}
