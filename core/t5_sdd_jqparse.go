package main

// The text of jq's parse errors (jq 1.7.1, jv_parse.c). parseStream knows whether a document is valid JSON; it was
// not written to say what jq says when it is not, and `_gentle_home_ready` lets jq's own stderr through, so what
// jq says is part of the output. This is a port of the parser's scanner reduced to what the messages depend on:
// the kind of each value, the lengths of the containers and the position (line and column count bytes, and a
// newline is counted before the token it ends is checked).

import (
	"fmt"
	"strings"
)

type rtJqFrame struct {
	kind byte // '[' an array, '{' an object, 's' an object key waiting for its value
	n    int  // elements, or pairs
}

type rtJqScan struct {
	stack     []rtJqFrame
	next      bool // `next` is a valid value
	nextIsStr bool
	tok       []byte
	inStr     bool
	esc       bool
	line, col int
}

// rtJqSyntax is the first parse error of data as jq words it, or "" when the scanner finds none.
func rtJqSyntax(data []byte) string {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	s := &rtJqScan{line: 1}
	for _, ch := range data {
		if msg := s.scan(ch); msg != "" {
			return fmt.Sprintf("%s at line %d, column %d", msg, s.line, s.col) // # MUTATION-ANCHOR: 844-R200
		}
	}
	if s.inStr {
		return fmt.Sprintf("Unfinished string at EOF at line %d, column %d", s.line, s.col)
	}
	if msg := s.checkLiteral(); msg != "" {
		return fmt.Sprintf("%s at EOF at line %d, column %d", msg, s.line, s.col)
	}
	if len(s.stack) != 0 {
		return fmt.Sprintf("Unfinished JSON term at EOF at line %d, column %d", s.line, s.col)
	}
	return ""
}

func (s *rtJqScan) scan(ch byte) string {
	s.col++ // # MUTATION-ANCHOR: 844-R204
	if ch == '\n' {
		s.line++
		s.col = 0
	}
	if s.inStr {
		if ch == '"' && !s.esc {
			msg := s.foundString()
			s.inStr = false
			s.tok = s.tok[:0]
			return msg
		}
		s.tok = append(s.tok, ch)
		s.esc = ch == '\\' && !s.esc
		return ""
	}
	switch ch {
	case ' ', '\t', '\r', '\n':
		return s.checkLiteral()
	case '"':
		if msg := s.checkLiteral(); msg != "" {
			return msg
		}
		s.inStr, s.esc = true, false
		s.tok = s.tok[:0]
		return ""
	case '[', ',', ']', '{', ':', '}':
		if msg := s.checkLiteral(); msg != "" {
			return msg
		}
		return s.token(ch)
	}
	s.tok = append(s.tok, ch)
	return ""
}

// value is `value(p, v)`: a value arrives; two in a row need a separator.
func (s *rtJqScan) value(isStr bool) string {
	if s.next {
		return "Expected separator between values"
	}
	s.next, s.nextIsStr = true, isStr
	if len(s.stack) == 0 { // parse_check_done: a top-level value is handed over at once
		s.next, s.nextIsStr = false, false
	}
	return ""
}

func (s *rtJqScan) top() *rtJqFrame { return &s.stack[len(s.stack)-1] }

func (s *rtJqScan) token(ch byte) string {
	switch ch {
	case '[', '{':
		if len(s.stack) >= 10000 {
			return "Exceeds depth limit for parsing"
		}
		if s.next {
			return "Expected separator between values"
		}
		s.stack = append(s.stack, rtJqFrame{kind: ch})
	case ':':
		if !s.next {
			return "Expected string key before ':'"
		}
		if len(s.stack) == 0 || s.top().kind != '{' {
			return "':' not as part of an object"
		}
		if !s.nextIsStr {
			return "Object keys must be strings"
		}
		s.stack = append(s.stack, rtJqFrame{kind: 's'})
		s.next, s.nextIsStr = false, false
	case ',':
		if !s.next {
			return "Expected value before ','"
		}
		if len(s.stack) == 0 {
			return "',' not as part of an object or array"
		}
		switch s.top().kind {
		case '[':
			s.top().n++
		case 's':
			s.stack = s.stack[:len(s.stack)-1]
			s.top().n++
		default:
			return "Objects must consist of key:value pairs"
		}
		s.next, s.nextIsStr = false, false
	case ']':
		if len(s.stack) == 0 || s.top().kind != '[' {
			return "Unmatched ']'"
		}
		if s.next {
			s.top().n++
		} else if s.top().n != 0 {
			return "Expected another array element"
		}
		s.close()
	case '}':
		if len(s.stack) == 0 {
			return "Unmatched '}'"
		}
		if s.next {
			if s.top().kind != 's' {
				return "Objects must consist of key:value pairs"
			}
			s.stack = s.stack[:len(s.stack)-1]
			s.top().n++
		} else {
			if s.top().kind != '{' {
				return "Unmatched '}'"
			}
			if s.top().n != 0 {
				return "Expected another key-value pair"
			}
		}
		s.close()
	}
	return ""
}

// close pops the finished container: it is the parser's `next`, and a top-level one is handed over.
func (s *rtJqScan) close() {
	s.stack = s.stack[:len(s.stack)-1]
	s.next, s.nextIsStr = true, false
	if len(s.stack) == 0 {
		s.next = false
	}
}

func (s *rtJqScan) checkLiteral() string {
	if len(s.tok) == 0 || s.inStr {
		return ""
	}
	defer func() { s.tok = s.tok[:0] }()
	tok := string(s.tok)
	pattern := ""
	switch tok[0] {
	case 't':
		pattern = "true"
	case 'f':
		pattern = "false"
	case 'n':
		if len(tok) > 1 && tok[1] == 'u' { // # MUTATION-ANCHOR: 844-R201
			pattern = "null"
		}
	}
	if pattern != "" {
		if tok != pattern {
			return "Invalid literal"
		}
		return s.value(false)
	}
	if !rtDecNumber(tok) { // # MUTATION-ANCHOR: 844-R203
		return "Invalid numeric literal" // # MUTATION-ANCHOR: 844-R202
	}
	return s.value(false)
}

// rtDecNumber is what decNumber's string conversion accepts (jq 1.7.1 reads number literals with it).
func rtDecNumber(t string) bool {
	if t != "" && (t[0] == '+' || t[0] == '-') {
		t = t[1:]
	}
	low := strings.ToLower(t)
	switch low {
	case "inf", "infinity", "nan", "snan":
		return true
	}
	for _, p := range []string{"nan", "snan"} {
		if rest, ok := strings.CutPrefix(low, p); ok && rest != "" && strings.Trim(rest, "0123456789") == "" {
			return true
		}
	}
	i, digits := 0, 0
	for i < len(low) && low[i] >= '0' && low[i] <= '9' {
		i++
		digits++
	}
	if i < len(low) && low[i] == '.' {
		i++
		for i < len(low) && low[i] >= '0' && low[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(low) && low[i] == 'e' {
		i++
		if i < len(low) && (low[i] == '+' || low[i] == '-') {
			i++
		}
		ed := 0
		for i < len(low) && low[i] >= '0' && low[i] <= '9' {
			i++
			ed++
		}
		if ed == 0 {
			return false
		}
	}
	return i == len(low)
}

func rtUnhex4(b []byte) int {
	v := 0
	for _, c := range b[:4] {
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | int(c-'0')
		case c >= 'a' && c <= 'f':
			v = v<<4 | int(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v<<4 | int(c-'A'+10)
		default:
			return -1
		}
	}
	return v
}

// foundString is `found_string`: the escapes of the string just closed, then the string as a value.
func (s *rtJqScan) foundString() string {
	b := s.tok
	for i := 0; i < len(b); {
		c := b[i]
		i++
		if c == '\\' {
			if i >= len(b) {
				return "Expected escape character at end of string"
			}
			c = b[i]
			i++
			switch c {
			case '\\', '"', '/', 'b', 'f', 't', 'n', 'r':
			case 'u':
				if i+4 > len(b) {
					return "Invalid \\uXXXX escape"
				}
				hv := rtUnhex4(b[i:])
				if hv < 0 {
					return "Invalid characters in \\uXXXX escape"
				}
				i += 4
				if hv >= 0xD800 && hv <= 0xDBFF {
					if i+6 > len(b) || b[i] != '\\' || b[i+1] != 'u' {
						return "Invalid \\uXXXX\\uXXXX surrogate pair escape"
					}
					sg := rtUnhex4(b[i+2:])
					if !(sg >= 0xDC00 && sg <= 0xDFFF) {
						return "Invalid \\uXXXX\\uXXXX surrogate pair escape"
					}
					i += 6
				}
			default:
				return "Invalid escape"
			}
		} else if c > 0 && c < 0x1f {
			return "Invalid string: control characters from U+0000 through U+001F must be escaped"
		}
	}
	return s.value(true)
}
