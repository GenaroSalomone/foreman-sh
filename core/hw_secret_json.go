package main

// The five jq programs of bin/hw-secret, read in Go so hw-core needs no jq. Each one answers what its jq answers on
// what bw prints: a string raw, null and false as nothing (`// empty`), anything else as JSON text; a type error
// (`.name` of a string) ends the program, which the bash then reads as an empty answer.

import (
	"bytes"
	"encoding/json"
	"strings"
)

type hsJSON = json.RawMessage

func hsDecode(s string) (hsJSON, bool) {
	var raw json.RawMessage
	d := json.NewDecoder(strings.NewReader(s))
	if d.Decode(&raw) != nil {
		return nil, false
	}
	if _, err := d.Token(); err == nil || err.Error() != "EOF" {
		return nil, false
	}
	return raw, true
}

func hsKind(v hsJSON) byte {
	t := bytes.TrimSpace(v)
	if len(t) == 0 {
		return 0
	}
	return t[0]
}

// hsGet is `.key`: null of null, the member of an object (null when absent), an error of anything else.
func hsGet(v hsJSON, key string) (hsJSON, bool) {
	switch hsKind(v) {
	case 'n':
		return hsJSON("null"), true
	case '{':
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			return nil, false
		}
		if x, ok := m[key]; ok {
			return x, true
		}
		return hsJSON("null"), true
	}
	return nil, false
}

// hsEach is `.[]?`: the elements of an array, the values of an object, nothing of anything else.
func hsEach(v hsJSON) []hsJSON {
	var r []hsJSON
	switch hsKind(v) {
	case '[':
		var a []json.RawMessage
		if json.Unmarshal(v, &a) == nil {
			r = a
		}
	case '{':
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) == nil {
			for _, x := range m {
				r = append(r, x)
			}
		}
	}
	return r
}

// hsFirst is `.[0]`.
func hsFirst(v hsJSON) (hsJSON, bool) {
	switch hsKind(v) {
	case 'n':
		return hsJSON("null"), true
	case '[':
		var a []json.RawMessage
		if json.Unmarshal(v, &a) != nil {
			return nil, false
		}
		if len(a) == 0 {
			return hsJSON("null"), true
		}
		return a[0], true
	}
	return nil, false
}

func hsFalsy(v hsJSON) bool {
	t := strings.TrimSpace(string(v))
	return t == "null" || t == "false"
}

// hsText is what `jq -r` prints for one value.
func hsText(v hsJSON) string {
	switch hsKind(v) {
	case '"':
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	case '{', '[':
		var b bytes.Buffer
		if json.Indent(&b, v, "", "  ") == nil {
			return b.String()
		}
	}
	return strings.TrimSpace(string(v))
}

func hsIsString(v hsJSON, s string) bool {
	var x string
	return hsKind(v) == '"' && json.Unmarshal(v, &x) == nil && x == s
}

// hsStatus is `jq -r '.status // empty'` of `bw status`.
func hsStatus(out string) string {
	raw, ok := hsDecode(out)
	if !ok {
		return ""
	}
	v, ok := hsGet(raw, "status")
	if !ok || hsFalsy(v) {
		return ""
	}
	return hsText(v)
}

// hsFirstID is `[.[]? | select(.name == $n) | .id][0] // empty` of `bw list folders`.
func hsFirstID(out, name string) string {
	raw, ok := hsDecode(out)
	if !ok {
		return ""
	}
	var first hsJSON
	for _, e := range hsEach(raw) {
		n, ok := hsGet(e, "name")
		if !ok {
			return ""
		}
		if hsIsString(n, name) && first == nil {
			id, ok := hsGet(e, "id")
			if !ok {
				return ""
			}
			first = id
		}
	}
	if first == nil || hsFalsy(first) {
		return ""
	}
	return hsText(first)
}

// hsIDs is `.[]? | select(.name == $n) | .id` of `bw list items`, as the bash holds it in `$( )`: every id on its own
// line (an error ends the stream, what came before stays), the trailing newlines gone.
func hsIDs(out, name string) string {
	raw, ok := hsDecode(out)
	if !ok {
		return ""
	}
	var lines []string
	for _, e := range hsEach(raw) {
		n, ok := hsGet(e, "name")
		if !ok {
			break
		}
		if hsIsString(n, name) {
			id, _ := hsGet(e, "id")
			lines = append(lines, hsText(id))
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// hsField evaluates the jq expression of one field of `bw get item`, then `// empty` (null and false are nothing).
func hsField(out, field string) (hsJSON, bool) {
	raw, ok := hsDecode(out)
	if !ok {
		return nil, false
	}
	var v hsJSON
	switch field {
	case "password", "username", "uri":
		login, ok := hsGet(raw, "login")
		if !ok {
			return nil, false
		}
		switch field {
		case "uri":
			uris, ok := hsGet(login, "uris")
			if !ok {
				return nil, false
			}
			u, ok := hsFirst(uris)
			if !ok {
				return nil, false
			}
			v, ok = hsGet(u, "uri")
			if !ok {
				return nil, false
			}
		default:
			v, ok = hsGet(login, field)
			if !ok {
				return nil, false
			}
		}
	case "notes":
		v, ok = hsGet(raw, "notes")
		if !ok {
			return nil, false
		}
	default:
		fields, ok := hsGet(raw, "fields")
		if !ok {
			return nil, false
		}
		v = hsJSON("null")
		found := false
		for _, e := range hsEach(fields) {
			n, ok := hsGet(e, "name")
			if !ok {
				return nil, false
			}
			if hsIsString(n, field) && !found {
				v, _ = hsGet(e, "value")
				found = true
			}
		}
	}
	if hsFalsy(v) {
		return nil, true
	}
	return v, true
}

// hsFieldRaw is `jq -r "(EXPR) // empty"`: the text of the field, "" when it is null or false; ok false on a jq error.
func hsFieldRaw(out, field string) (string, bool) {
	v, ok := hsField(out, field)
	if !ok || v == nil {
		return "", ok
	}
	return hsText(v), true
}

// hsFieldPresent is `jq -e "((EXPR) // \"\") | (tostring | length) > 0"`.
func hsFieldPresent(out, field string) bool {
	v, ok := hsField(out, field)
	if !ok || v == nil {
		return false
	}
	if hsKind(v) == '"' {
		return hsText(v) != ""
	}
	return true
}
