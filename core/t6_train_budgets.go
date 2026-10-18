package main

// The three-way merge of setup/test-budgets.json that bin/hw-train ran as an inline python3 script: per key, a side
// that left a key as the base had it takes the other side's value (a deletion included), and where both changed it
// the train's (ours) wins. Written the way setup/measure-tests.sh writes it: json.dumps(indent=2, sort_keys=True).

import (
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// trainGone is the python script's GONE: the key is absent on that side.
type trainGoneT struct{}

var trainGone any = &trainGoneT{}

func trainIsGone(v any) bool { _, ok := v.(*trainGoneT); return ok }

// trainMergeBudgets is resolve_budgets' python: the merged file text, or false where python exits non-zero (a side
// does not parse, or the merge is GONE).
func trainMergeBudgets(base, ours, theirs string) (string, bool) {
	b, ok := pyLoads(base)
	if !ok {
		return "", false
	}
	o, ok := pyLoads(ours)
	if !ok {
		return "", false
	}
	t, ok := pyLoads(theirs)
	if !ok {
		return "", false
	}
	r := trainMerge(b, o, t)
	if trainIsGone(r) {
		return "", false
	}
	var sb strings.Builder
	trainDump(&sb, r, 0)
	sb.WriteByte('\n')
	return sb.String(), true
}

func trainMerge(b, o, t any) any {
	if trainEq(o, t) {
		return o
	}
	if trainEq(b, o) {
		return t
	}
	if trainEq(b, t) {
		return o
	}
	bm, bok := b.(jmap)
	om, ook := o.(jmap)
	tm, tok := t.(jmap)
	if bok && ook && tok {
		out := jmap{}
		get := func(m jmap, k string) any {
			if v, ok := m[k]; ok {
				return v
			}
			return trainGone
		}
		keys := make([]string, 0, len(om)+len(tm))
		for k := range om {
			keys = append(keys, k)
		}
		for k := range tm {
			if _, in := om[k]; !in {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			if r := trainMerge(get(bm, k), get(om, k), get(tm, k)); !trainIsGone(r) {
				out[k] = r
			}
		}
		return out
	}
	return o
}

// trainEq is python's ==: dicts by content, lists in order, numbers by value (1 == 1.0 == True).
func trainEq(a, b any) bool {
	if trainIsGone(a) || trainIsGone(b) {
		return trainIsGone(a) && trainIsGone(b)
	}
	if ra, ok := trainRat(a); ok {
		rb, ok := trainRat(b)
		return ok && ra.Cmp(rb) == 0
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	case jmap:
		y, ok := b.(jmap)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, in := y[k]
			if !in || !trainEq(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !trainEq(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// trainRat reads a number or a bool as the exact value python compares.
func trainRat(v any) (*big.Rat, bool) {
	switch x := v.(type) {
	case bool:
		if x {
			return big.NewRat(1, 1), true
		}
		return big.NewRat(0, 1), true
	case json.Number:
		if r, ok := new(big.Rat).SetString(string(x)); ok {
			return r, true
		}
		if f, err := strconv.ParseFloat(string(x), 64); err == nil && !math.IsInf(f, 0) {
			return new(big.Rat).SetFloat64(f), true
		}
		return big.NewRat(0, 1), true
	}
	return nil, false
}

// trainDump is json.dumps(x, indent=2, sort_keys=True), ensure_ascii on.
func trainDump(sb *strings.Builder, v any, depth int) {
	pad := strings.Repeat("  ", depth+1)
	end := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		if x {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case string:
		sb.WriteString(pyJSONDump(x))
	case json.Number:
		sb.WriteString(trainNumber(string(x)))
	case jmap:
		if len(x) == 0 {
			sb.WriteString("{}")
			return
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString("{\n")
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(",\n")
			}
			sb.WriteString(pad + pyJSONDump(k) + ": ")
			trainDump(sb, x[k], depth+1)
		}
		sb.WriteString("\n" + end + "}")
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[\n")
		for i, e := range x {
			if i > 0 {
				sb.WriteString(",\n")
			}
			sb.WriteString(pad)
			trainDump(sb, e, depth+1)
		}
		sb.WriteString("\n" + end + "]")
	}
}

// trainNumber is what python prints for a parsed JSON number: an int as its digits, a float as repr.
func trainNumber(lit string) string {
	if !strings.ContainsAny(lit, ".eE") {
		if strings.HasPrefix(lit, "-") && strings.Trim(lit[1:], "0") == "" {
			return "0" // int("-0")
		}
		return lit
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil && !math.IsInf(f, 0) {
		return lit
	}
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	return pyFloatRepr(f)
}
