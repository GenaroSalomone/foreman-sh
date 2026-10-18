package main

// hw briefs — the plan (lib/hw/briefs.sh `_briefs_plan`, whose python body is ported here): which of a lane's flat
// briefs are finished, from `hw ledger --json`, the work dirs and git. Nothing is written.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// briefsNow is time.time() of the python plan; tests move it.
var briefsNow = time.Now

// briefsLedger returns the lines of `hw ledger <lane> --json` (the seam tests replace); ok is false when the
// ledger failed (bash: `cmd_ledger "$lane" --json > file 2>/dev/null || die`).
var briefsLedger func(a *app, lane string) (string, bool)

func init() { // assigned here, not at the declaration: runWith reaches cmdBriefs, which reads this (an init cycle)
	briefsLedger = func(a *app, lane string) (string, bool) {
		var out bytes.Buffer
		rc := runWith([]string{"ledger", lane, "--json"}, &out, io.Discard, false, a.getenv, nil)
		return out.String(), rc == 0
	}
}

// briefsPlanRow is one line of the plan: `move <task> <yyyy-mm> <reason>` or `skip <task> <why>`.
type briefsPlanRow struct {
	move             bool
	task, ym, reason string // reason is the skip cause when !move
}

const briefsStaleDays = 21

func briefsISO(ts int64) string { return time.Unix(ts, 0).UTC().Format("2006-01-02T15:04:05Z") }

// briefsGit runs git -C dir args, stdout only, and reports failure (python's check=True).
func briefsGit(dir string, args ...string) (string, bool) {
	r := extRun(extCall{Name: "git", Args: append([]string{"-C", dir}, args...)})
	return r.Out, r.RC == 0
}

// briefsRealpath is `cd <dir> && pwd -P`.
func briefsRealpath(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	r, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	return r, true
}

// briefsHwDone is hw_done(wd): the newest run's reported time if it reported and was not reopened.
func briefsHwDone(wd string) (int64, bool) {
	hd := filepath.Join(wd, ".hw")
	ents, err := os.ReadDir(hd)
	if err != nil {
		return 0, false
	}
	var runs []string
	for _, e := range ents {
		if isDir(filepath.Join(hd, e.Name())) {
			runs = append(runs, e.Name())
		}
	}
	if len(runs) == 0 {
		return 0, false
	}
	sort.Strings(runs)
	run := filepath.Join(hd, runs[len(runs)-1])
	d := run
	best := -1
	subs, _ := os.ReadDir(run)
	reT := regexp.MustCompile(`^t[0-9]+$`)
	for _, e := range subs {
		if reT.MatchString(e.Name()) && isDir(filepath.Join(run, e.Name())) {
			if n, _ := strconv.Atoi(e.Name()[1:]); n > best {
				best, d = n, filepath.Join(run, e.Name())
			}
		}
	}
	if isFile(filepath.Join(d, "reopened")) || isFile(filepath.Join(d, "blocked-waiting")) {
		return 0, false
	}
	st, err := os.Stat(filepath.Join(d, "done"))
	if err != nil || st.IsDir() {
		return 0, false
	}
	return st.ModTime().Unix(), true
}

type briefsMerge struct{ sha, when string }

// briefsMerges is git_merges(): task -> the newest merge on the base whose subject names the lane's task branch.
// Python's `(?<![\w./-])` lookbehind does not exist in RE2, so each start position is tried by hand.
func briefsMerges(brain, gbranch, gbase string) map[string]briefsMerge {
	merges := map[string]briefsMerge{}
	if gbranch == "" || gbase == "" {
		return merges
	}
	if _, ok := briefsGit(brain, "rev-parse", "--verify", "-q", gbase+"^{commit}"); !ok {
		return merges
	}
	pre, post, _ := strings.Cut(gbranch, "{task}")
	rx := regexp.MustCompile(`^` + regexp.QuoteMeta(pre) + `([A-Za-z0-9][A-Za-z0-9._-]*)` + regexp.QuoteMeta(post))
	into := regexp.MustCompile(`^Merge (?:remote-tracking )?branch '([^']+)'`)
	out, _ := briefsGit(brain, "log", gbase, "--merges", "--format=%H%x09%cI%x09%s")
	for _, ln := range strings.Split(out, "\n") {
		sha, rest, _ := strings.Cut(ln, "\t")
		when, subj, _ := strings.Cut(rest, "\t")
		s := subj
		if m := into.FindStringSubmatch(subj); m != nil { // git's own "Merge branch 'a' into b" merges a, not b
			s = m[1]
		}
		for i := 0; i < len(s); {
			if i > 0 {
				pr := []rune(s[:i])
				c := pr[len(pr)-1]
				if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '.' || c == '/' || c == '-' {
					i++
					continue
				}
			}
			m := rx.FindStringSubmatch(s[i:])
			if m == nil {
				i++
				continue
			}
			name := strings.TrimRight(m[1], ".")
			if _, seen := merges[name]; !seen {
				merges[name] = briefsMerge{sha, when}
			}
			i += len(m[0])
		}
	}
	return merges
}

type briefsLedgerRow struct {
	state, evidence, doneAt, ts string
}

func briefsStr(v any) string {
	s, _ := v.(string)
	return s
}

// briefsPlan is _briefs_plan. A failure dies as the bash does.
func (a *app) briefsPlan(lane string) []briefsPlanRow {
	brain := a.c.brain
	lj, ok := briefsLedger(a, lane)
	if !ok {
		a.die("briefs: hw ledger failed for " + lane + ", so no brief's state is known; nothing is planned")
	}
	bdir := brain + "/" + lane + "/briefs"
	flat, _ := filepath.Glob(bdir + "/*.md")
	sort.Strings(flat)
	wtdirs := map[string]string{}
	for _, f := range flat {
		if isFile(f) {
			t := strings.TrimSuffix(filepath.Base(f), ".md")
			wtdirs[t] = a.c.lanePath(lane, "worktree", t)
		}
	}
	// git evidence exists only where the lane's checkout IS the brain repo
	var gbranch, gbase string
	gtop, _ := briefsGit(brain, "rev-parse", "--show-toplevel")
	gtop = strings.TrimRight(gtop, "\n")
	gco := a.c.checkout(lane)
	if gtop != "" && gco != "" && isDir(gco) {
		r1, ok1 := briefsRealpath(gco)
		r2, ok2 := briefsRealpath(gtop)
		if ok1 && ok2 && r1 == r2 {
			if a.c.t.known(lane) {
				gbranch = a.c.lanePath(lane, "branch", "{task}")
			} else {
				gbranch = "task/{task}"
			}
			gbase = a.c.t.get(lane, "base")
		}
	}
	fail := func() { a.die("briefs: the plan could not be computed") }
	prefix, ok1 := briefsGit(brain, "rev-parse", "--show-prefix")
	if !ok1 {
		fail()
	}
	prefix = strings.TrimSpace(prefix)
	tracked := map[string]bool{}
	lsf, ok2 := briefsGit(brain, "ls-files", "-z", "--", lane+"/briefs")
	if !ok2 {
		fail()
	}
	for _, p := range strings.Split(lsf, "\x00") {
		if p != "" {
			tracked[p] = true
		}
	}
	dirty := map[string]bool{}
	st, ok3 := briefsGit(brain, "status", "--porcelain", "-z", "--untracked-files=no", "--no-renames", "--", lane+"/briefs")
	if !ok3 {
		fail()
	}
	for _, ent := range strings.Split(st, "\x00") {
		if len(ent) > 3 {
			p := ent[3:]
			if prefix != "" && strings.HasPrefix(p, prefix) {
				p = p[len(prefix):]
			}
			dirty[p] = true
		}
	}
	rows := map[string]briefsLedgerRow{}
	for _, ln := range strings.Split(lj, "\n") {
		var r map[string]any
		if json.Unmarshal([]byte(ln), &r) != nil || r == nil {
			continue
		}
		if briefsTruthy(r["archived"]) {
			continue
		}
		rows[briefsStr(r["task"])] = briefsLedgerRow{briefsStr(r["state"]), briefsStr(r["evidence"]), briefsStr(r["done_at"]), briefsStr(r["ts"])}
	}
	gmerges := briefsMerges(brain, gbranch, gbase)

	lastc := map[string]int64{} // the newest commit touching each brief, one pass
	lg, ok4 := briefsGit(brain, "log", "--no-renames", "--relative", "--name-only", "--format=%x01%ct", "--", lane+"/briefs")
	if !ok4 {
		fail()
	}
	var cur int64
	have := false
	for _, ln := range strings.Split(lg, "\n") {
		if strings.HasPrefix(ln, "\x01") {
			n, err := strconv.ParseInt(ln[1:], 10, 64)
			if err != nil {
				fail()
			}
			cur, have = n, true
		} else if ln != "" && have {
			if _, seen := lastc[ln]; !seen {
				lastc[ln] = cur
			}
		}
	}
	now := briefsNow().Unix()

	var plan []briefsPlanRow
	skip := func(name, why string) { plan = append(plan, briefsPlanRow{task: name, reason: why}) }
	for _, f := range flat {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		if strings.HasPrefix(name, "_") {
			continue
		}
		rel := lane + "/briefs/" + name + ".md"
		row, hasRow := rows[name]
		state := "never-dispatched"
		if hasRow {
			state = row.state
		}
		when, reason := "", ""
		if state == "in-progress" { // bash anchor 812-M02 (registered on the bash source)
			skip(name, "in-progress")
			continue
		}
		if state == "done" {
			ev := row.evidence
			if ev == "" {
				ev = "done"
			}
			reason = "ledger: " + ev
			when = row.doneAt
			if when == "" {
				when = row.ts
			}
		}
		// every place this task's executor may live: the lane's worktree and the plain work dir
		var present []string
		seen := map[string]bool{}
		for _, w := range []string{wtdirs[name], filepath.Join(a.c.work, lane, name)} {
			if w != "" && !seen[w] {
				seen[w] = true
				if isDir(w) {
					present = append(present, w)
				}
			}
		}
		var held int64
		heldOK := false
		for _, w := range present {
			h, ok := briefsHwDone(w)
			if !ok {
				heldOK = false
				break
			}
			heldOK = true
			if h > held {
				held = h
			}
		}
		if state == "never-dispatched" {
			// no ledger line: the history decides, and the reason names the evidence
			var gev *briefsMerge
			if !heldOK {
				if m, ok := gmerges[name]; ok { // bash anchor 813-M01 (registered on the bash source)
					gev = &m
				}
			}
			if heldOK {
				reason, when = "backfill: .hw done marker", briefsISO(held)
			} else if gev != nil {
				reason, when = "backfill: git merge "+briefsShort(gev.sha)+" of "+strings.Replace(gbranch, "{task}", name, 1), gev.when
			} else if lc, ok := lastc[rel]; ok && now-lc > briefsStaleDays*86400 { // bash anchor 814-M01 (registered on the bash source)
				reason, when = "stale: no ledger entry or evidence; last commit "+time.Unix(lc, 0).UTC().Format("2006-01-02"), briefsISO(lc)
			} else { // bash anchor 813-M02 (registered on the bash source)
				skip(name, "never-dispatched")
				continue
			}
		}
		if len(present) > 0 && !heldOK { // bash anchor 812-M04 (registered on the bash source)
			skip(name, "live-worktree")
			continue
		}
		if when == "" {
			mt := now
			if fi, err := os.Stat(f); err == nil {
				mt = fi.ModTime().Unix()
			}
			when = briefsISO(mt)
			reason += " (month from the brief's mtime)"
		}
		switch {
		case !tracked[rel]:
			skip(name, "untracked")
		case dirty[rel]:
			skip(name, "modified")
		default:
			if m, _ := filepath.Glob(bdir + "/archive/*/" + name + ".md"); len(m) > 0 {
				skip(name, "name-taken")
				continue
			}
			ym := when
			if len(ym) > 7 {
				ym = ym[:7]
			}
			plan = append(plan, briefsPlanRow{move: true, task: name, ym: ym, reason: reason})
		}
	}
	return plan
}

// briefsTruthy is python's truthiness of a decoded JSON value.
func briefsTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func briefsShort(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
