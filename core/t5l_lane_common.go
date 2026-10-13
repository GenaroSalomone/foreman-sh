package main

// Track D2 — what the maintainer's own product lanes share and D1 may not provide
// (lanes/*.sh, bin/hw `_pick_port`, `_port_free`, `_link_or_copy`). The `d2` prefix marks a helper the integrator
// can unify with D1's.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// d2Must is `set -e` on one simple command: a failing status ends the launch with that status.
func (l *lch) d2Must(c cmdSpec) string {
	out, rc := l.sh(c)
	if rc != 0 { // # MUTATION-ANCHOR: 847-D2-01
		panic(exitNow{rc})
	}
	return out
}

// d2PaneRun is `herdr pane run "$pane" "$cmd" >/dev/null` under set -e.
func (l *lch) d2PaneRun(pane, cmd string) {
	l.d2Must(cmdSpec{name: "herdr", args: []string{"pane", "run", pane, cmd}, mut: true})
}

// d2Unset is bash's `unset VAR` for the dispatch's environment: the variable reads empty from here on.
// (d.envOver cannot express an absent variable; a child sees VAR= instead of no VAR. Integrator: a real
// unset in environ() makes this exact.)
func (l *lch) d2Unset(keys ...string) { l.unsetEnv(keys...) }

// d2Readable is `[ -r path ]`.
func d2Readable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func d2IsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// d2Exists is `[ -d p ]`.
func d2Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// d2DBName is `basename "$dir" | tr '\-/' '__'`.
func d2DBName(dir string) string {
	d := strings.TrimRight(dir, "/")
	if d == "" && dir != "" {
		d = "/"
	}
	base := d
	if i := strings.LastIndexByte(d, '/'); i >= 0 && d != "/" {
		base = d[i+1:]
	}
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '/' {
			return '_'
		}
		return r
	}, base)
}

// d2TailLines is `printf '%s\n' "$s" | tail -n`.
func d2TailLines(s string, n int) string {
	lines := strings.Split(s, "\n") // s + "\n" splits into the lines plus a final empty element
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}

// d2GrepMatch is `printf '%s' "$text" | grep -q[i] "$pattern"` for a database name used as a basic regular
// expression: the characters BRE takes literally are escaped for Go's syntax.
func d2GrepMatch(text, pattern string, fold bool) bool {
	var b strings.Builder
	b.WriteString("(?m)")
	if fold {
		b.WriteString("(?i)")
	}
	for _, r := range pattern {
		if strings.ContainsRune(`+?|(){}`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

// ── ports ────────────────────────────────────────────────────────────────

// d2Which finds an executable on the dispatch's PATH, as `command -v`.
func (l *lch) d2Which(name string) bool {
	for _, dir := range strings.Split(l.env("PATH"), ":") {
		if dir == "" {
			dir = "."
		}
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// d2PortFree is `_port_free`.
func (l *lch) d2PortFree(p int) bool {
	ps := strconv.Itoa(p)
	switch {
	case l.d2Which("lsof"):
		_, rc := l.sh(cmdSpec{name: "lsof", args: []string{"-nP", "-iTCP:" + ps, "-sTCP:LISTEN"}, quiet: true})
		return rc != 0
	case l.d2Which("ss"):
		out, rc := l.sh(cmdSpec{name: "ss", args: []string{"-Hltn", "sport = :" + ps}})
		if rc != 0 { // ss failed: no answer, not "busy"
			return true
		}
		return strings.TrimRight(out, "\n") == ""
	}
	return true
}

// d2PickPort is `_pick_port "$1"`: lane D1's pickPort, as a string.
func (l *lch) d2PickPort(first int) string { return strconv.Itoa(l.pickPort(first)) }

// d2Mkdir is `mkdir "$d" 2>/dev/null`: fails when it exists (the claim); read-only it is recorded and succeeds.
func (l *lch) d2Mkdir(d string) bool {
	if l.fs.readonly {
		l.fs.rec(fsOp{Op: "mkdir", Path: d})
		return true
	}
	return os.Mkdir(d, 0o777) == nil
}

// d2FreshWithin is `find "$d" -mmin -N` printing something; ok=false when find itself would fail.
func d2FreshWithin(d string, age time.Duration) (fresh, ok bool) {
	now := time.Now()
	ok = true
	filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			if p == d {
				ok = false
			}
			return nil
		}
		if info, ierr := e.Info(); ierr == nil && now.Sub(info.ModTime()) < age {
			fresh = true
		}
		return nil
	})
	return fresh, ok
}

// d2PortOf is `$(( $(lane_get "$PROJ" port_X) + OFF ))`.
func (l *lch) d2PortOf(field string) int {
	base, _ := strconv.Atoi(strings.TrimSpace(l.c.t.get(l.proj, field)))
	off, _ := strconv.Atoi(strings.TrimSpace(l.off))
	return base + off
}

// ── env files ────────────────────────────────────────────────────────────

// d2LinkOrCopy is `_link_or_copy main wt rel mode` (mode: link | copy).
func (l *lch) d2LinkOrCopy(main, wt, rel, mode string) {
	src, dst := main+"/"+rel, wt+"/"+rel
	if !d2Exists(src) {
		l.warn(rel + " — NOT PRESENT in " + main + "; the dev server may boot against code defaults")
		return
	}
	lst, lerr := os.Lstat(dst)
	isLink := lerr == nil && lst.Mode()&os.ModeSymlink != 0
	if isLink {
		if tgt, err := os.Readlink(dst); err == nil && tgt == src {
			l.info(rel + " — already linked")
			return
		}
	}
	if d2Exists(dst) && !isLink && mode == "copy" { // # MUTATION-ANCHOR: 847-D2-03
		l.info(rel + " — already present, kept")
		return
	}
	if d2Exists(dst) || isLink {
		l.d2Must(cmdSpec{name: "rm", args: []string{"-f", dst}, mut: true})
	}
	switch mode {
	case "link":
		l.d2Must(cmdSpec{name: "ln", args: []string{"-s", src, dst}, mut: true})
		l.ok(rel + " — symlink (shared)")
	case "copy":
		l.d2Must(cmdSpec{name: "cp", args: []string{"-p", src, dst}, mut: true})
		l.ok(rel + " — copy (task-private)")
	}
}

// d2EnvLocalGlob is `"$main"/.env.*.local` filtered by `[ -f ]`, in glob order.
func d2EnvLocalGlob(main string) []string {
	ents, err := os.ReadDir(main)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if len(n) >= len(".env..local") && strings.HasPrefix(n, ".env.") && strings.HasSuffix(n, ".local") {
			if st, err := os.Stat(main + "/" + n); err == nil && st.Mode().IsRegular() {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}
