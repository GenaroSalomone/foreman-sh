package main

// A JSON reader and writer that behave like jq 1.7 where hw's own files meet
// jq: an ordered object, a stream of concatenated values, numbers written the
// way jq writes a literal (decNumber's to-scientific-string), and the string
// escapes jq's tojson uses. Nothing here is a general JSON library — it is
// exactly the surface the bash implementation reached through `jq -r`/`jq -c`.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type vkind int

const (
	vNull vkind = iota
	vBool
	vNum
	vStr
	vArr
	vObj
)

type member struct {
	k string
	v *jval
}

type jval struct {
	kind vkind
	b    bool
	s    string // the string, or the number literal as written
	arr  []*jval
	obj  []member
}

var jnull = &jval{kind: vNull}

func jstr(s string) *jval { return &jval{kind: vStr, s: s} }

// get is jq's `.key`: null indexes to null, an object to its member or null,
// and anything else is an error (ok == false).
func (v *jval) get(key string) (*jval, bool) {
	switch v.kind {
	case vNull:
		return jnull, true
	case vObj:
		for _, m := range v.obj {
			if m.k == key {
				return m.v, true
			}
		}
		return jnull, true
	}
	return nil, false
}

// truthy is jq's: only null and false are false.
func (v *jval) truthy() bool {
	return !(v.kind == vNull || (v.kind == vBool && !v.b))
}

// raw is `jq -r` for one output: a string as itself, anything else as tojson.
func (v *jval) raw() string {
	if v.kind == vStr {
		return v.s
	}
	return v.toJSON()
}

// tostring is jq's `tostring` / string interpolation.
func (v *jval) tostring() string { return v.raw() }

func (v *jval) toJSON() string {
	var sb strings.Builder
	v.write(&sb)
	return sb.String()
}

func (v *jval) write(sb *strings.Builder) {
	switch v.kind {
	case vNull:
		sb.WriteString("null")
	case vBool:
		if v.b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case vNum:
		sb.WriteString(canonNumber(v.s))
	case vStr:
		writeJSONString(sb, v.s)
	case vArr:
		sb.WriteByte('[')
		for i, e := range v.arr {
			if i > 0 {
				sb.WriteByte(',')
			}
			e.write(sb)
		}
		sb.WriteByte(']')
	case vObj:
		sb.WriteByte('{')
		for i, m := range v.obj {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeJSONString(sb, m.k)
			sb.WriteByte(':')
			m.v.write(sb)
		}
		sb.WriteByte('}')
	}
}

const hexdigits = "0123456789abcdef"

// writeJSONString escapes the way jq does: the two-character escapes, every
// other control character and DEL as \u00xx, and nothing else (not <, >, &,
// U+2028).
func writeJSONString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			sb.WriteString(`\"`)
		case c == '\\':
			sb.WriteString(`\\`)
		case c == '\n':
			sb.WriteString(`\n`)
		case c == '\r':
			sb.WriteString(`\r`)
		case c == '\t':
			sb.WriteString(`\t`)
		case c == '\b':
			sb.WriteString(`\b`)
		case c == '\f':
			sb.WriteString(`\f`)
		case c < 0x20 || c == 0x7f: // # MUTATION-ANCHOR: 816-G17
			sb.WriteString(`\u00`)
			sb.WriteByte(hexdigits[c>>4])
			sb.WriteByte(hexdigits[c&15])
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('"')
}

// canonNumber writes a number literal as jq 1.7 prints it: decNumber's
// to-scientific-string of the literal, so 1.0 stays 1.0, 1e2 is 1E+2, 0.5E1 is
// 5 and 00.5 is 0.5.
func canonNumber(lit string) string {
	neg := false
	s := lit
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	mant, exp := s, 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		e, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return lit
		}
		exp = e
	}
	intp, frac := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intp, frac = mant[:i], mant[i+1:]
	}
	digits := strings.TrimLeft(intp+frac, "0") // # MUTATION-ANCHOR: 816-G16
	if digits == "" {
		digits = "0"
	}
	exp -= len(frac)
	adj := exp + len(digits) - 1
	var out string
	if exp <= 0 && adj >= -6 {
		if exp == 0 {
			out = digits
		} else {
			n := -exp
			if len(digits) <= n {
				digits = strings.Repeat("0", n-len(digits)+1) + digits
			}
			out = digits[:len(digits)-n] + "." + digits[len(digits)-n:]
		}
	} else {
		out = digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		out += "E"
		if adj >= 0 {
			out += "+"
		}
		out += strconv.Itoa(adj)
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ── the reader ──────────────────────────────────────────────────────────────

type jparser struct {
	d     []byte
	i     int
	depth int
}

var errJSON = errors.New("json")

func (p *jparser) fail(msg string) error {
	line, col := 1, 0
	for _, c := range p.d[:min(p.i, len(p.d))] {
		if c == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return fmt.Errorf("%s at line %d, column %d", msg, line, col)
}

// parseStream reads every JSON value in data, whitespace-separated or not, as
// jq reads its input. On a syntax error it returns the values read before it
// and the error: jq has already printed those by then.
func parseStream(data []byte) ([]*jval, error) {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	p := &jparser{d: data}
	var vals []*jval
	for {
		p.ws()
		if p.i >= len(p.d) {
			return vals, nil
		}
		v, err := p.value()
		if err != nil {
			return vals, err
		}
		vals = append(vals, v)
	}
}

func (p *jparser) ws() {
	for p.i < len(p.d) {
		switch p.d[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

func isDelim(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '[', ']', '{', '}', ',', ':', '"':
		return true
	}
	return false
}

func (p *jparser) value() (*jval, error) {
	if p.i >= len(p.d) {
		return nil, p.fail("Unfinished JSON term at EOF")
	}
	switch c := p.d[p.i]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		s, err := p.str()
		if err != nil {
			return nil, err
		}
		return jstr(s), nil
	default:
		start := p.i
		for p.i < len(p.d) && !isDelim(p.d[p.i]) {
			p.i++
		}
		tok := string(p.d[start:p.i])
		switch tok {
		case "":
			return nil, p.fail("Expected JSON value")
		case "true":
			return &jval{kind: vBool, b: true}, nil
		case "false":
			return &jval{kind: vBool}, nil
		case "null":
			return jnull, nil
		}
		if validNumber(tok) {
			return &jval{kind: vNum, s: tok}, nil
		}
		return nil, p.fail("Invalid literal")
	}
}

func validNumber(t string) bool {
	i := 0
	if i < len(t) && t[i] == '-' {
		i++
	}
	nd := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
		nd++
	}
	if i < len(t) && t[i] == '.' {
		i++
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
			nd++
		}
	}
	if nd == 0 {
		return false
	}
	if i < len(t) && (t[i] == 'e' || t[i] == 'E') {
		i++
		if i < len(t) && (t[i] == '+' || t[i] == '-') {
			i++
		}
		ne := 0
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
			ne++
		}
		if ne == 0 {
			return false
		}
	}
	return i == len(t)
}

const maxDepth = 10000

func (p *jparser) array() (*jval, error) {
	p.i++
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return nil, p.fail("Exceeds depth limit for parsing")
	}
	v := &jval{kind: vArr}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == ']' {
		p.i++
		return v, nil
	}
	for {
		p.ws()
		e, err := p.value()
		if err != nil {
			return nil, err
		}
		v.arr = append(v.arr, e)
		p.ws()
		if p.i >= len(p.d) {
			return nil, p.fail("Unfinished JSON term at EOF")
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return v, nil
		default:
			return nil, p.fail("Expected separator between values")
		}
	}
}

func (p *jparser) object() (*jval, error) {
	p.i++
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return nil, p.fail("Exceeds depth limit for parsing")
	}
	v := &jval{kind: vObj}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == '}' {
		p.i++
		return v, nil
	}
	for {
		p.ws()
		if p.i >= len(p.d) {
			return nil, p.fail("Unfinished JSON term at EOF")
		}
		if p.d[p.i] != '"' {
			return nil, p.fail("Object keys must be strings")
		}
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.d) || p.d[p.i] != ':' {
			return nil, p.fail("Objects must consist of key:value pairs")
		}
		p.i++
		p.ws()
		e, err := p.value()
		if err != nil {
			return nil, err
		}
		set := false
		for j := range v.obj {
			if v.obj[j].k == k {
				v.obj[j].v = e
				set = true
				break
			}
		}
		if !set {
			v.obj = append(v.obj, member{k, e})
		}
		p.ws()
		if p.i >= len(p.d) {
			return nil, p.fail("Unfinished JSON term at EOF")
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return v, nil
		default:
			return nil, p.fail("Expected separator between values")
		}
	}
}

func hex4(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range b[:4] {
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

func (p *jparser) str() (string, error) {
	p.i++ // opening quote
	var sb strings.Builder
	for {
		if p.i >= len(p.d) {
			return "", p.fail("Unfinished string at EOF")
		}
		c := p.d[p.i]
		switch {
		case c == '"':
			p.i++
			return sb.String(), nil
		case c < 0x20:
			return "", p.fail("Invalid string: control characters from U+0000 through U+001F must be escaped")
		case c == '\\':
			p.i++
			if p.i >= len(p.d) {
				return "", p.fail("Unfinished string at EOF")
			}
			e := p.d[p.i]
			p.i++
			switch e {
			case '"', '\\', '/':
				sb.WriteByte(e)
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'u':
				r, ok := hex4(p.d[p.i:])
				if !ok {
					return "", p.fail("Invalid escape")
				}
				p.i += 4
				if r >= 0xd800 && r < 0xdc00 {
					if p.i+6 <= len(p.d) && p.d[p.i] == '\\' && p.d[p.i+1] == 'u' {
						lo, ok := hex4(p.d[p.i+2:])
						if ok && lo >= 0xdc00 && lo < 0xe000 {
							p.i += 6
							r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
							sb.WriteRune(r)
							continue
						}
					}
					return "", p.fail("Invalid \\uXXXX\\uXXXX surrogate pair escape")
				}
				if r >= 0xdc00 && r < 0xe000 {
					return "", p.fail("Invalid \\uXXXX\\uXXXX surrogate pair escape")
				}
				sb.WriteRune(r)
			default:
				return "", p.fail("Invalid escape")
			}
		case c < utf8.RuneSelf:
			sb.WriteByte(c)
			p.i++
		default:
			r, n := utf8.DecodeRune(p.d[p.i:])
			if r == utf8.RuneError && n == 1 {
				sb.WriteString("�")
				p.i++
			} else {
				sb.Write(p.d[p.i : p.i+n])
				p.i += n
			}
		}
	}
}
