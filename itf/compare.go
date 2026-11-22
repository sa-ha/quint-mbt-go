package itf

import (
	"fmt"
	"strconv"
	"strings"
)

// Compare defines a total order on values: first by kind, then by content.
// It returns -1, 0, or +1. Values that compare equal denote the same Quint value.
func Compare(a, b Value) int {
	if a.kind != b.kind {
		return cmpInt(int(a.kind), int(b.kind))
	}
	switch a.kind {
	case KindBool:
		return cmpInt(btoi(a.b), btoi(b.b))
	case KindInt:
		return a.i.Cmp(b.i)
	case KindStr, KindUnserializable:
		return strings.Compare(a.s, b.s)
	case KindMap:
		if c := compareSeq(a.keys, b.keys); c != 0 {
			return c
		}
	case KindRecord:
		if c := compareSeq(strs(a.fields), strs(b.fields)); c != 0 {
			return c
		}
	}
	return compareSeq(a.elems, b.elems)
}

// Equal reports whether a and b denote the same Quint value.
func Equal(a, b Value) bool { return Compare(a, b) == 0 }

func compareSeq(a, b []Value) int {
	for n := 0; n < len(a) && n < len(b); n++ {
		if c := Compare(a[n], b[n]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

func strs(ss []string) []Value {
	vs := make([]Value, len(ss))
	for n, s := range ss {
		vs[n] = Str(s)
	}
	return vs
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// String renders v in Quint-like syntax. Variants are shown as Tag(value).
func (v Value) String() string {
	var sb strings.Builder
	v.write(&sb)
	return sb.String()
}

func (v Value) write(sb *strings.Builder) {
	if tag, p, ok := v.AsVariant(); ok {
		sb.WriteString(tag)
		if p.kind != KindTuple || len(p.elems) != 0 {
			sb.WriteByte('(')
			p.write(sb)
			sb.WriteByte(')')
		}
		return
	}
	switch v.kind {
	case KindBool:
		sb.WriteString(strconv.FormatBool(v.b))
	case KindInt:
		sb.WriteString(v.i.String())
	case KindStr:
		sb.WriteString(strconv.Quote(v.s))
	case KindUnserializable:
		fmt.Fprintf(sb, "<unserializable %q>", v.s)
	case KindList:
		writeSeq(sb, "[", v.elems, "]")
	case KindTuple:
		writeSeq(sb, "(", v.elems, ")")
	case KindSet:
		writeSeq(sb, "Set(", v.elems, ")")
	case KindMap:
		sb.WriteString("Map(")
		for n := range v.keys {
			if n > 0 {
				sb.WriteString(", ")
			}
			v.keys[n].write(sb)
			sb.WriteString(" -> ")
			v.elems[n].write(sb)
		}
		sb.WriteByte(')')
	case KindRecord:
		sb.WriteString("{ ")
		for n, name := range v.fields {
			if n > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name + ": ")
			v.elems[n].write(sb)
		}
		sb.WriteString(" }")
	default:
		sb.WriteString("<invalid>")
	}
}

func writeSeq(sb *strings.Builder, open string, vs []Value, close string) {
	sb.WriteString(open)
	for n, e := range vs {
		if n > 0 {
			sb.WriteString(", ")
		}
		e.write(sb)
	}
	sb.WriteString(close)
}

// Difference describes one point where an expected and an observed value
// disagree. Missing or Extra is set when one side lacks the element at Path.
type Difference struct {
	Path               string
	Expected, Observed Value
	Missing, Extra     bool
	Hint               string
}

func (d Difference) String() string {
	var s string
	switch {
	case d.Missing:
		s = fmt.Sprintf("%s: missing in observed (expected %s)", d.Path, d.Expected)
	case d.Extra:
		s = fmt.Sprintf("%s: unexpected in observed (%s)", d.Path, d.Observed)
	default:
		s = fmt.Sprintf("%s: expected %s, observed %s", d.Path, d.Expected, d.Observed)
	}
	if d.Hint != "" {
		s += " (hint: " + d.Hint + ")"
	}
	return s
}

// bigNumberHint flags the shape Quint's Rust backend emits for integers of 16
// or more decimal digits (Quint 0.33.1), which decodes as a record {c, e, s}.
const bigNumberHint = "expected value looks like a bignumber.js object; Quint's Rust backend " +
	"serializes integers of 16+ digits this way. Keep such integers out of the model " +
	"or generate traces with --backend=typescript"

// Diff lists the differences between expected and observed, rooted at path.
// It returns nil when the values are equal. Output order is deterministic.
func Diff(path string, expected, observed Value) []Difference {
	if Equal(expected, observed) {
		return nil
	}
	if expected.kind == observed.kind {
		switch expected.kind {
		case KindRecord:
			return diffKeyed(strs(expected.fields), expected.elems, strs(observed.fields), observed.elems,
				func(k Value) string { return path + "." + k.s })
		case KindMap:
			return diffKeyed(expected.keys, expected.elems, observed.keys, observed.elems,
				func(k Value) string { return path + "[" + k.String() + "]" })
		case KindSet:
			return diffKeyed(expected.elems, expected.elems, observed.elems, observed.elems,
				func(k Value) string { return path + "{" + k.String() + "}" })
		case KindList, KindTuple:
			if len(expected.elems) == len(observed.elems) {
				var ds []Difference
				for n := range expected.elems {
					ds = append(ds, Diff(fmt.Sprintf("%s[%d]", path, n), expected.elems[n], observed.elems[n])...)
				}
				return ds
			}
		}
	}
	d := Difference{Path: path, Expected: expected, Observed: observed}
	if expected.kind == KindRecord && observed.kind == KindInt &&
		strings.Join(expected.fields, ",") == "c,e,s" {
		d.Hint = bigNumberHint
	}
	return []Difference{d}
}

// diffKeyed merges two key-sorted sequences.
func diffKeyed(ek, ev, ok, ov []Value, sub func(Value) string) []Difference {
	var ds []Difference
	i, j := 0, 0
	for i < len(ek) || j < len(ok) {
		c := 1
		if i < len(ek) && j < len(ok) {
			c = Compare(ek[i], ok[j])
		} else if i < len(ek) {
			c = -1
		}
		switch {
		case c < 0:
			ds = append(ds, Difference{Path: sub(ek[i]), Expected: ev[i], Missing: true})
			i++
		case c > 0:
			ds = append(ds, Difference{Path: sub(ok[j]), Observed: ov[j], Extra: true})
			j++
		default:
			ds = append(ds, Diff(sub(ek[i]), ev[i], ov[j])...)
			i, j = i+1, j+1
		}
	}
	return ds
}
