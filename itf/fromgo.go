package itf

import (
	"fmt"
	"math/big"
	"reflect"
)

// Valuer lets a Go type choose its own ITF encoding, typically a Variant for
// an enum-like type.
type Valuer interface {
	ITFValue() (Value, error)
}

var (
	valueType  = reflect.TypeFor[Value]()
	bigIntType = reflect.TypeFor[*big.Int]()
	valuerType = reflect.TypeFor[Valuer]()
)

// FromGo converts an implementation-side Go value to a Value so that it can be
// compared with a trace state. The mapping is:
//
//   - Value is returned as is; a Valuer supplies its own encoding
//   - bool, string, every integer kind, and *big.Int map to bool, str, and int
//   - map[K]struct{} maps to a set; any other map maps to a map
//   - slices and arrays map to lists; nil maps and slices are empty
//   - structs map to records; the field name is the `itf` tag, or the Go
//     field name when untagged; `itf:"-"` and unexported fields are skipped
//   - pointers and interfaces are followed; nil is an error
//
// Floats, channels, and functions have no ITF counterpart and are errors.
func FromGo(x any) (Value, error) {
	return fromGo(reflect.ValueOf(x))
}

func fromGo(rv reflect.Value) (Value, error) {
	if !rv.IsValid() {
		return Value{}, fmt.Errorf("itf: cannot convert nil")
	}
	t := rv.Type()
	switch {
	case t == valueType:
		return rv.Interface().(Value), nil
	case t.Implements(valuerType):
		if t.Kind() == reflect.Pointer && rv.IsNil() {
			return Value{}, fmt.Errorf("itf: cannot convert nil %s", t)
		}
		return rv.Interface().(Valuer).ITFValue()
	case t == bigIntType:
		if rv.IsNil() {
			return Value{}, fmt.Errorf("itf: cannot convert nil *big.Int")
		}
		return BigInt(rv.Interface().(*big.Int)), nil
	}
	switch t.Kind() {
	case reflect.Bool:
		return Bool(rv.Bool()), nil
	case reflect.String:
		return Str(rv.String()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Int(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return BigInt(new(big.Int).SetUint64(rv.Uint())), nil
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return Value{}, fmt.Errorf("itf: cannot convert nil %s", t)
		}
		return fromGo(rv.Elem())
	case reflect.Slice, reflect.Array:
		elems := make([]Value, rv.Len())
		for n := range elems {
			v, err := fromGo(rv.Index(n))
			if err != nil {
				return Value{}, err
			}
			elems[n] = v
		}
		return List(elems...), nil
	case reflect.Map:
		isSet := t.Elem().Kind() == reflect.Struct && t.Elem().NumField() == 0
		var keys, vals []Value
		for it := rv.MapRange(); it.Next(); {
			k, err := fromGo(it.Key())
			if err != nil {
				return Value{}, err
			}
			keys = append(keys, k)
			if !isSet {
				v, err := fromGo(it.Value())
				if err != nil {
					return Value{}, err
				}
				vals = append(vals, v)
			}
		}
		if isSet {
			return Set(keys...), nil
		}
		entries := make([]Entry, len(keys))
		for n := range keys {
			entries[n] = Entry{keys[n], vals[n]}
		}
		return NewMap(entries...)
	case reflect.Struct:
		fields := map[string]Value{}
		for n := 0; n < t.NumField(); n++ {
			f := t.Field(n)
			name := f.Tag.Get("itf")
			if !f.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			v, err := fromGo(rv.Field(n))
			if err != nil {
				return Value{}, fmt.Errorf("field %s: %w", f.Name, err)
			}
			fields[name] = v
		}
		return Record(fields), nil
	}
	return Value{}, fmt.Errorf("itf: cannot convert %s", t)
}
