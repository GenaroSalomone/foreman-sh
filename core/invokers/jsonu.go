package invokers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// jqString is how `jq -c` writes a string: the short escapes, \u00xx for the
// other control characters and DEL, everything else as it is, and U+FFFD for
// each byte that is not UTF-8.
func jqString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && w == 1 {
			b.WriteString("�")
			i++
			continue
		}
		i += w
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pair is one token: a nil value is JSON null, which deletes the key in herdr.
type pair struct {
	k string
	v *string
}

// objectJSON is `jq -nc '[ ... {(k): v} ] | add // {}'`: insertion order, a
// repeated key replaced where it first stood.
func objectJSON(pairs []pair) string {
	order := []string{}
	last := map[string]*string{}
	for _, p := range pairs {
		if _, seen := last[p.k]; !seen {
			order = append(order, p.k)
		}
		last[p.k] = p.v
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range order {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(jqString(k))
		b.WriteByte(':')
		if v := last[k]; v == nil {
			b.WriteString("null")
		} else {
			b.WriteString(jqString(*v))
		}
	}
	b.WriteByte('}')
	return b.String()
}

// orderedObject parses a JSON object keeping the key order; values are kept as
// raw JSON. An error is what `fromjson` failing is.
func orderedObject(s string) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, fmt.Errorf("not an object")
	}
	var keys []string
	vals := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		k := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, err
		}
		if _, seen := vals[k]; !seen {
			keys = append(keys, k)
		}
		vals[k] = raw
	}
	if _, err := dec.Token(); err != nil {
		return nil, nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, fmt.Errorf("trailing data")
	}
	return keys, vals, nil
}

// compactJSON re-writes valid JSON the way jq -c does for the shapes used here
// (strings re-escaped by jqString, everything else compacted).
func compactJSON(raw json.RawMessage) string {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return string(raw)
	}
	return jqValue(v)
}

func jqValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case string:
		return jqString(t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = jqValue(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]any:
		// only reached for nested objects, where order is not tracked
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sortStrings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = jqString(k) + ":" + jqValue(t[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return "null"
}

// jqRaw is `jq -r <path>` on a decoded document, with `// empty` semantics: ok is
// false for a missing key, null or false.
func jqPath(doc any, path ...string) (any, bool) {
	cur := doc
	for _, k := range path {
		m, isMap := cur.(map[string]any)
		if !isMap {
			return nil, false
		}
		cur, isMap = m[k]
		if !isMap {
			return nil, false
		}
	}
	if cur == nil {
		return nil, false
	}
	if b, isBool := cur.(bool); isBool && !b {
		return nil, false
	}
	return cur, true
}

// jqRawString is what `jq -r` prints for a value: a string as it is, anything
// else as compact JSON.
func jqRawString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return jqValue(v)
}

func decodeDoc(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}
