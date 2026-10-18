package main

// Small readers the dispatch, the reuse route and `hw revive` share. They lived beside gentle's home checks until
// gentle-ai left hw; nothing about them was gentle's.

import (
	"os"
	"strings"
	"unicode"
)

// rtBashDirname is dirname(1) for the paths the reuse route walks up.
func rtBashDirname(p string) string {
	if p == "" {
		return "."
	}
	t := strings.TrimRight(p, "/")
	if t == "" {
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	t = strings.TrimRight(t[:i], "/")
	if t == "" {
		return "/"
	}
	return t
}

// rtJqWalk walks .a.b[0].c: a string is a key, an int an index. null indexes to null; a value that cannot be
// indexed that way is an error, and the text is jq's.
func rtJqWalk(v *jval, steps []any) (*jval, string) {
	cur := v
	for _, s := range steps {
		switch x := s.(type) {
		case string:
			n, ok := cur.get(x)
			if !ok {
				return nil, "Cannot index " + kindName(cur) + " with string " + jstr(x).toJSON()
			}
			cur = n
		case int:
			switch cur.kind {
			case vNull:
				cur = jnull
			case vArr:
				if x < len(cur.arr) {
					cur = cur.arr[x]
				} else {
					cur = jnull
				}
			default:
				return nil, "Cannot index " + kindName(cur) + " with number"
			}
		}
	}
	return cur, ""
}

// rtJqSteps is rtJqWalk without jq's error text: whether the walk held.
func rtJqSteps(v *jval, steps []any) (*jval, bool) {
	cur, msg := rtJqWalk(v, steps)
	return cur, msg == ""
}

// reviveFramework is _run_dispatch_framework: the second word of the first `  framework …` line of the dispatch.
func reviveFramework(rundir string) string {
	data, err := os.ReadFile(rundir + "/dispatch")
	if err != nil || !readableFile(rundir+"/dispatch") {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if len(l) > len("  framework") && strings.HasPrefix(l, "  framework") && unicode.IsSpace(rune(l[len("  framework")])) {
			f := strings.FieldsFunc(l, func(r rune) bool { return r == ' ' || r == '\t' })
			if len(f) >= 2 {
				return f[1]
			}
			return ""
		}
	}
	return ""
}
