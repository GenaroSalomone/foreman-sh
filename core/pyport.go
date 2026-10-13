package main

// The few Python behaviours `hw status` leans on, written once. The Python in
// lib/hw/status.sh is the spec of that verb, so what it does with a number, a
// path, a missing key or a time is what the Go has to do too; each helper names
// the Python it stands for.

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ── JSON as json.loads gives it ──────────────────────────────────────────────

type jmap = map[string]any

// pyLoads is json.loads: a value, or false where Python raises. Numbers keep
// their text (json.Number) so an int prints as an int.
func pyLoads(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err == nil || err.Error() != "EOF" { // trailing data is an error in json.loads
		return nil, false
	}
	return v, true
}

// pyGet is d.get(k) on a value that may not be a dict.
func pyGet(v any, k string) any {
	if m, ok := v.(jmap); ok {
		return m[k]
	}
	return nil
}

// pyDict is `x or {}` for something that should be a dict.
func pyDict(v any) jmap {
	if m, ok := v.(jmap); ok {
		return m
	}
	return jmap{}
}

// pyList is `x or []`, keeping the dict elements: the code that reads them calls .get on each.
func pyDicts(v any) []jmap {
	l, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []jmap
	for _, e := range l {
		if m, ok := e.(jmap); ok {
			out = append(out, m)
		}
	}
	return out
}

func pyTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		f, err := x.Float64()
		return err != nil || f != 0
	case []any:
		return len(x) > 0
	case jmap:
		return len(x) > 0
	}
	return true
}

// pyStr is str(v) / what an f-string prints for it.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			return s
		}
		if f, err := x.Float64(); err == nil {
			return pyFloatRepr(f)
		}
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// pyFloatRepr is repr(float): the shortest digits, fixed notation for a
// decimal exponent in [-4, 16) and scientific outside it.
func pyFloatRepr(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "nan"
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde±XX
	mant, expS, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expS)
	if exp >= -4 && exp < 16 {
		switch {
		case exp >= 0:
			if len(digits) <= exp+1 {
				return sign + digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
			}
			return sign + digits[:exp+1] + "." + digits[exp+1:]
		default:
			return sign + "0." + strings.Repeat("0", -exp-1) + digits
		}
	}
	m := digits[:1]
	if len(digits) > 1 {
		m += "." + digits[1:]
	}
	es := strconv.Itoa(exp)
	if exp >= 0 {
		es = "+" + es
	}
	if len(strings.TrimLeft(es, "+-")) < 2 {
		es = es[:1] + "0" + es[1:]
	}
	return sign + m + "e" + es
}

// pyJSONDump is json.dump(x) with the defaults: ", " and ": " separators, and
// every non-ASCII character as \uXXXX.
func pyJSONDump(v any) string {
	var sb strings.Builder
	var w func(v any)
	w = func(v any) {
		switch x := v.(type) {
		case nil:
			sb.WriteString("null")
		case bool:
			if x {
				sb.WriteString("true")
			} else {
				sb.WriteString("false")
			}
		case int:
			sb.WriteString(strconv.Itoa(x))
		case float64:
			sb.WriteString(pyFloatRepr(x))
		case string:
			sb.WriteByte('"')
			for _, r := range x {
				switch {
				case r == '"':
					sb.WriteString(`\"`)
				case r == '\\':
					sb.WriteString(`\\`)
				case r == '\n':
					sb.WriteString(`\n`)
				case r == '\r':
					sb.WriteString(`\r`)
				case r == '\t':
					sb.WriteString(`\t`)
				case r == '\b':
					sb.WriteString(`\b`)
				case r == '\f':
					sb.WriteString(`\f`)
				case r < 0x20 || (r >= 0x7f && r < 0x10000): // json.dump escapes everything outside ' '..'~'
					sb.WriteString(`\u` + hex4s(int(r)))
				case r >= 0x10000:
					r -= 0x10000
					sb.WriteString(`\u` + hex4s(0xd800+int(r>>10)) + `\u` + hex4s(0xdc00+int(r&0x3ff)))
				default:
					sb.WriteRune(r)
				}
			}
			sb.WriteByte('"')
		case []jmapOrdered:
			sb.WriteByte('[')
			for i, o := range x {
				if i > 0 {
					sb.WriteString(", ")
				}
				w(o)
			}
			sb.WriteByte(']')
		case jmapOrdered:
			sb.WriteByte('{')
			for i, kv := range x {
				if i > 0 {
					sb.WriteString(", ")
				}
				w(kv.k)
				sb.WriteString(": ")
				w(kv.v)
			}
			sb.WriteByte('}')
		}
	}
	w(v)
	return sb.String()
}

type kvOrd struct {
	k string
	v any
}
type jmapOrdered []kvOrd

func hex4s(n int) string {
	const h = "0123456789abcdef"
	return string([]byte{h[n>>12&15], h[n>>8&15], h[n>>4&15], h[n&15]})
}

// ── numbers ──────────────────────────────────────────────────────────────────

// pyInt is int(s) for a str: blanks around, an optional sign, digits with single
// underscores between them.
func pyInt(s string) (int, bool) {
	s = pyStrip(s)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	s = strings.ReplaceAll(s, "_", "")
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		n = math.MaxInt
	}
	if neg {
		n = -n
	}
	return n, true
}

// pyFloat is float(s) for a str.
func pyFloat(s string) (float64, bool) {
	s = pyStrip(s)
	if s == "" || strings.ContainsAny(s, "xXpP") {
		return 0, false
	}
	t := s
	if strings.Contains(t, "_") {
		if strings.HasPrefix(t, "_") || strings.HasSuffix(t, "_") || strings.Contains(t, "__") || strings.Contains(t, "_.") || strings.Contains(t, "._") {
			return 0, false
		}
		t = strings.ReplaceAll(t, "_", "")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f, true
		}
		return 0, false
	}
	return f, true
}

// pyG is "%g" % f: six significant digits, trailing zeros dropped.
func pyG(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	return strconv.FormatFloat(f, 'g', 6, 64)
}

// ── text ─────────────────────────────────────────────────────────────────────

// pySplitlines is str.splitlines().
func pySplitlines(s string) []string {
	var out []string
	start := 0
	i := 0
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\r':
			out = append(out, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		case r == '\n', r == '\v', r == '\f', r == 0x1c, r == 0x1d, r == 0x1e, r == 0x85, r == 0x2028, r == 0x2029:
			out = append(out, s[start:i])
			start = i + n
		}
		i += n
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// pyCasefold is str.casefold() to the extent a path comparison needs it.
func pyCasefold(s string) string {
	return strings.Map(func(r rune) rune { return unicode.ToLower(r) }, strings.ReplaceAll(s, "ß", "ss"))
}

// pyLjust is str.ljust(n): padded to n characters, never cut.
func pyLjust(s string, n int) string {
	if c := utf8.RuneCountInString(s); c < n {
		return s + strings.Repeat(" ", n-c)
	}
	return s
}

var monthAbbr = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// pyStrftimeDHM is time.strftime("%d %b %H:%M", time.localtime(t)).
func pyStrftimeDHM(t float64) string {
	tm := time.Unix(int64(math.Floor(t)), 0).In(time.Local)
	return pad2(tm.Day()) + " " + monthAbbr[tm.Month()-1] + " " + pad2(tm.Hour()) + ":" + pad2(tm.Minute())
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// ── the file system ──────────────────────────────────────────────────────────

// pyGlobStar is sorted(glob.glob(dir + "/*")): what a `*` matches, which is
// every name that does not start with a dot, in code point order.
func pyGlobStar(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out
}

// pyIsDir is os.path.isdir: through symlinks.
func pyIsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func pyIsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// pyMtime is os.path.getmtime.
func pyMtime(p string) (float64, bool) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, false
	}
	return mtimeOf(st), true
}

func mtimeOf(st os.FileInfo) float64 {
	t := st.ModTime()
	return float64(t.Unix()) + float64(t.Nanosecond())/1e9
}

// pyRealpath is os.path.realpath (not strict): symlinks resolved as far as the
// path exists, the rest kept as written.
func pyRealpath(p string) string {
	if !filepath.IsAbs(p) {
		if wd, err := os.Getwd(); err == nil {
			p = wd + "/" + p
		}
	}
	res, _ := joinRealpath("", p, map[string]*string{})
	return filepath.Clean(res)
}

func joinRealpath(path, rest string, seen map[string]*string) (string, bool) {
	if strings.HasPrefix(rest, "/") {
		rest = strings.TrimLeft(rest, "/")
		path = "/"
	}
	for rest != "" {
		var name string
		name, rest, _ = strings.Cut(rest, "/")
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			if path != "" {
				dir, base := splitPath(path)
				if base == ".." {
					path = joinPath(dir, "..", "..")
				} else {
					path = dir
				}
			} else {
				path = ".."
			}
			continue
		}
		newpath := joinPath(path, name)
		st, err := os.Lstat(newpath)
		if err != nil || st.Mode()&os.ModeSymlink == 0 {
			path = newpath
			continue
		}
		if prev, ok := seen[newpath]; ok {
			if prev != nil {
				path = *prev
				continue
			}
			return joinPath(newpath, rest), false
		}
		seen[newpath] = nil
		target, err := os.Readlink(newpath)
		if err != nil {
			return joinPath(newpath, rest), false
		}
		var ok bool
		path, ok = joinRealpath(path, target, seen)
		if !ok {
			return joinPath(path, rest), false
		}
		pc := path
		seen[newpath] = &pc
	}
	return path, true
}

func joinPath(a string, b ...string) string {
	for _, x := range b {
		switch {
		case strings.HasPrefix(x, "/"):
			a = x
		case a == "" || strings.HasSuffix(a, "/"):
			a += x
		default:
			a += "/" + x
		}
	}
	return a
}

func splitPath(p string) (string, string) {
	i := strings.LastIndexByte(p, '/') + 1
	head, tail := p[:i], p[i:]
	if head != "" && head != strings.Repeat("/", len(head)) {
		head = strings.TrimRight(head, "/")
	}
	return head, tail
}

// bytesTrimNL is rstrip("\n") on bytes.
func bytesTrimNL(b []byte) string { return string(bytes.TrimRight(b, "\n")) }

// pyInt64 is pyInt for a value that may not fit an int on a 32-bit system.
func pyInt64(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, err == nil
}
