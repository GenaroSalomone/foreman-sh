package main

// reap, without --apply (train T2): the survey of each repo lane's git worktrees, its merged task branches that no
// worktree holds, the loose work dirs beside them, the databases hw recorded whose worktree is gone, the build output
// an idle kept worktree would give back, and the executors that reported BLOCKED and are still waiting.
//
// Port of cmd_reap (lib/hw/reap.sh:261-476) and of what it asks: _wt_disposition, _workdir_disposition,
// _wt_irreplaceable, _wt_branch_merged, _wt_pr_merged, _wt_build_idle, _wt_occupants, _leased_rundir_for and the
// retention gates (bin/hw:8560-9760). `--apply` removes things and stays bash, so does `-h`.
//
// WHAT A DRY RUN WRITES, and what hw-core does with it. The bash dry run is not a pure read: it appends to the
// database ledger a run recorded (`.hw-db-ledger-<lane>`) and caches its survey for `hw status` (`.hw-reap-<lane>`).
// Both are kept here, because status reads the second, and both are skipped under HW_CORE_READONLY (shadow: bash has
// written them already, and Go must not write them twice).
//
// Tools still run where bash runs them, because their answer IS the contract: `fd` (it honours .gitignore: a
// directory's contents under an ignore rule are not there for it), `du` (blocks, not bytes), `lsof`, `psql`, `gh`.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WT_REGENERABLE / WT_ARCHIVABLE (bin/hw:9122-9133).
const wtRegenerable = ".agents .atl .claude .codex .opencode .hw .specify .venv .ruff_cache .pytest_cache .mypy_cache __pycache__ node_modules dist build .next .turbo .DS_Store AGENTS.md PROGRESS.md opencode.json local next-env.d.ts tsconfig.tsbuildinfo tsconfig.build.tsbuildinfo .astro"
const wtArchivable = ".artifacts qa-report test-results playwright-report"

type reapRun struct {
	*app
	occ     occupantsReader
	prsMain string
	prsOK   bool
	prs     string
	// the WT_* and WD_* answers
	verdict, why, evidence string
	archive                []string
	wdVerdict, wdWhy       string
	wdRetAge               string
	retAge, retWin, retKnd string
	buildDirs              []string
	buildKB                int64
	buildAge               int64
	seedNote               string
}

func (a *app) cmdReap() int {
	projFilter := ""
	for _, arg := range a.args {
		switch {
		case arg == "--apply" || arg == "-h" || arg == "--help":
			a.errf("hw-core: reap %s removes things or prints the full usage, which the bash implementation answers\n", arg)
			return 64
		case strings.HasPrefix(arg, "-"):
			a.die("unknown option: " + arg)
		default:
			projFilter = a.c.t.canonProject(arg)
		}
	}
	r := &reapRun{app: a}
	r.occ = occupantsReader{h: herdrClient{getenv: a.getenv}, win: windowsPaths(a.getenv)}
	return r.run(projFilter)
}

// ── small shell-shaped helpers ──────────────────────────────────────────────

// gitRC is `git -C dir <args> 2>/dev/null`: stdout, and the exit status.
func gitRC(dir string, args ...string) (string, int) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	rc := exitStatus(cmd.Run())
	return out.String(), rc
}

// toolOut runs a program as the shell does: stdout, the status, stderr dropped.
func toolOut(name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	rc := exitStatus(cmd.Run())
	return out.String(), rc
}

func retentionDays(def int64, v string) int64 { // _retention_days: a whole number read as base 10, else the default
	if !allDigits(v) {
		return def
	}
	if len(v) > 9 {
		v = v[:9]
	}
	n, _ := pyInt64(v)
	return n
}

func (a *app) laneRepo(lane string) bool { return a.c.checkout(lane) != "" }

// laneBase is _lane_base with reap's `: "${base:=main}"`.
func (a *app) laneBaseOrMain(lane string) string {
	if b := a.c.t.get(lane, "base"); b != "" {
		return b
	}
	return "main"
}

// canonicalRoot is _wt_canonical_root: the directory a lane's worktrees belong in.
func (a *app) canonicalRoot(lane string) (string, bool) {
	d := a.c.lanePath(lane, "worktree", "__probe__")
	if d == "" {
		return "", false
	}
	return filepath.Dir(d), true
}

// shellGlobMatch is `case $s in $pat)`: * matches anything including a slash, ? one character, [..] a class.
func shellGlobMatch(pat, s string) bool {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; c {
		case '*':
			sb.WriteString("(?s:.*)")
		case '?':
			sb.WriteString("(?s:.)")
		case '[':
			j := i + 1
			if j < len(pat) && (pat[j] == '!' || pat[j] == '^') {
				j++
			}
			if j < len(pat) && pat[j] == ']' {
				j++
			}
			for j < len(pat) && pat[j] != ']' {
				j++
			}
			if j >= len(pat) {
				sb.WriteString(regexp.QuoteMeta("["))
				continue
			}
			class := pat[i+1 : j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + class + "]")
			i = j
		case '\\':
			if i+1 < len(pat) {
				i++
				sb.WriteString(regexp.QuoteMeta(string(pat[i])))
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	return err == nil && re.MatchString(s)
}

// ── _wt_seed_patterns ───────────────────────────────────────────────────────

var reCopyOne = regexp.MustCompile(`_copy_one "\$wt" "\$rel" "\$dry"`)

// seedPatterns is _wt_seed_patterns: the glob wt.sh copies with, read from wt.sh itself; sets seedNote.
func (r *reapRun) seedPatterns(wtSh string) ([]string, bool) {
	r.seedNote = ""
	body, ok := readText(wtSh)
	if !ok {
		r.seedNote = "no " + wtSh + " — reap falls back to the hand-maintained allowlist"
		return nil, false
	}
	lineno, line := 0, ""
	for i, l := range strings.Split(body, "\n") {
		if reCopyOne.MatchString(l) {
			lineno, line = i+1, l
			break
		}
	}
	if lineno == 0 {
		r.seedNote = wtSh + " has no _copy_one case line at the shape reap expects — falling back to the hand-maintained allowlist"
		return nil, false
	}
	pat := strings.TrimLeft(line, " \t\r\n\f\v")
	if i := strings.Index(pat, ")"); i >= 0 {
		pat = pat[:i]
	}
	if pat == "" {
		r.seedNote = "could not parse the case pattern at " + wtSh + ":" + itoa(lineno) + " — falling back to the hand-maintained allowlist"
		return nil, false
	}
	r.seedNote = "seeded from " + wtSh + ":" + itoa(lineno) + " (" + pat + ")"
	return strings.Split(pat, "|"), true
}

// copyOfMain is _wt_copy_of_main: a path the lane copies into every worktree, whose original is still in main.
func (r *reapRun) copyOfMain(wt, rel string) bool {
	main, lane := "", ""
	for _, c := range r.c.t.names {
		if r.c.t.get(c, "reap_copies") == "" {
			continue
		}
		m := r.c.checkout(c)
		if m == "" {
			continue
		}
		if strings.HasPrefix(wt, m+"/") {
			main, lane = m, c
			break
		}
	}
	if lane == "" {
		return false
	}
	matched := false
	for _, c := range strings.Split(r.c.t.get(lane, "reap_copies"), "\n") {
		if c != "" && shellGlobMatch(c, rel) {
			matched = true
			break
		}
	}
	if from := r.c.t.get(lane, "reap_copies_from"); !matched && from != "" {
		pats, _ := r.seedPatterns(main + "/" + from)
		for _, p := range pats {
			if p != "" && shellGlobMatch(p, rel) {
				matched = true
				break
			}
		}
	}
	return matched && isFile(main+"/"+rel)
}

// ── merged? ─────────────────────────────────────────────────────────────────

// prList is _wt_pr_list: the pull requests of main's GitHub origin, asked once per process and repository.
func (r *reapRun) prList(main string) bool {
	if r.prsMain == main {
		return r.prsOK
	}
	r.prsMain, r.prsOK, r.prs = main, false, ""
	gh := r.getenv("HW_GH")
	if gh == "" {
		gh = "gh"
	}
	ghPath, err := exec.LookPath(gh)
	if err != nil {
		return false
	}
	url, rc := gitRC(main, "remote", "get-url", "origin")
	if rc != 0 || !strings.Contains(bashSub(url), "github.com") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ghPath, "pr", "list", "--state", "all", "--limit", "1000",
		"--json", "headRefName,state,headRefOid,baseRefName",
		"--jq", `.[] | [.headRefName, .state, .headRefOid, .baseRefName] | @tsv`)
	cmd.Dir = main
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return false
	}
	r.prs, r.prsOK = bashSub(out.String()), true
	return true
}

// prMerged is _wt_pr_merged: the tip is in the head of a PR merged into the lane's base.
func (r *reapRun) prMerged(main, branch, base string) bool {
	if !r.prList(main) {
		return false
	}
	tipOut, rc := gitRC(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	tip := bashSub(tipOut)
	if rc != 0 {
		return false
	}
	var rows [][]string
	for _, l := range strings.Split(r.prs, "\n") {
		f := strings.Split(l, "\t")
		if len(f) >= 1 && f[0] == branch {
			for len(f) < 4 {
				f = append(f, "")
			}
			rows = append(rows, f)
		}
	}
	if len(rows) == 0 {
		return false
	}
	for _, f := range rows {
		if f[1] == "OPEN" { // it is in use again
			return false
		}
	}
	for _, f := range rows {
		state, oid, prbase := f[1], f[2], f[3]
		if state != "MERGED" || oid == "" || prbase != base {
			continue
		}
		if _, rc := gitRC(main, "merge-base", "--is-ancestor", tip, oid); oid == tip || rc == 0 {
			r.evidence = "PR-merged: its tip is in the head of a PR merged into " + base
			return true
		}
	}
	return false
}

// branchMerged is _wt_branch_merged: ancestry, then CONTENT (a squash lands the work as one new commit), then the PR.
func (r *reapRun) branchMerged(main, branch, base string) bool {
	if _, rc := gitRC(main, "merge-base", "--is-ancestor", branch, "origin/"+base); rc == 0 {
		return true
	}
	if _, rc := gitRC(main, "merge-base", "--is-ancestor", branch, base); rc == 0 {
		return true
	}
	var ref string
	if _, rc := gitRC(main, "rev-parse", "--verify", "--quiet", "origin/"+base); rc == 0 {
		ref = "origin/" + base
	} else if _, rc := gitRC(main, "rev-parse", "--verify", "--quiet", base); rc == 0 {
		ref = base
	} else {
		return false
	}
	mbOut, _ := gitRC(main, "merge-base", ref, branch)
	mb := bashSub(mbOut)
	if mb == "" {
		return false
	}
	names, _ := gitRC(main, "diff", "-z", "--name-only", mb, branch)
	if strings.ReplaceAll(names, "\x00", "") == "" { // a branch that changed nothing is not evidence of a merge
		return false
	}
	var paths []string
	for _, n := range strings.Split(names, "\x00") {
		if n != "" {
			paths = append(paths, n)
		}
	}
	identical := true
	for i := 0; i < len(paths) && identical; i += 500 {
		end := i + 500
		if end > len(paths) {
			end = len(paths)
		}
		args := append([]string{"diff", "--quiet", ref, branch, "--"}, paths[i:end]...)
		if _, rc := gitRC(main, args...); rc != 0 {
			identical = false
		}
	}
	if identical {
		r.evidence = "squash-merged: every path it contributed is already identical in " + base
		return true
	}
	return r.prMerged(main, branch, base)
}

// ── is it disposable? ───────────────────────────────────────────────────────

// leasedRundirFor is _leased_rundir_for: a run under <wt>/.hw whose preview lease is live.
func leasedRundirFor(wt string) (string, bool) {
	if !isDir(wt + "/.hw") {
		return "", false
	}
	for _, id := range RunIDs(wt + "/.hw") {
		rd := wt + "/.hw/" + id
		body, ok := readText(rd + "/lease")
		if !ok || !readableFile(rd+"/lease") {
			continue
		}
		until := firstKey(body, "until")
		if allDigits(until) {
			if u, ok := pyInt64(until); ok && nowSec() < u {
				return rd, true
			}
		}
	}
	return "", false
}

// firstKey is `sed -n "s/^key=//p" | head -1`.
func firstKey(body, key string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

// lockReason is _wt_lock_reason: whether git has the worktree locked, and the reason it gives.
func lockReason(main, wt string) (string, bool) {
	real := wt
	if p, err := filepath.EvalSymlinks(wt); err == nil && isDir(wt) {
		real = p
	}
	out, _ := gitRC(main, "worktree", "list", "--porcelain")
	here, found, reason := false, false, ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			w := l[len("worktree "):]
			here = w == wt || w == real
		case here && strings.HasPrefix(l, "locked"):
			found = true
			reason = ""
			if len(l) > 7 {
				reason = l[7:]
			}
			if len(reason) >= 2 && reason[0] == '"' && reason[len(reason)-1] == '"' {
				reason = reason[1 : len(reason)-1]
			}
		}
	}
	return reason, found
}

func joinSp(lines []string) string { return strings.Join(lines, " ") }

// irreplaceable is _wt_irreplaceable: the ignored entries git cannot rebuild, one per line.
func (r *reapRun) irreplaceable(wt string) []string {
	ignored, rc := gitRC(wt, "status", "--porcelain", "--ignored")
	if rc != 0 {
		return []string{"<the ignored-file list could not be read — git status --ignored exited " + itoa(rc) + ">"}
	}
	var rows []string
	for _, l := range strings.Split(strings.TrimRight(ignored, "\n"), "\n") {
		if strings.HasPrefix(l, "!! ") {
			rows = append(rows, strings.TrimSuffix(l[3:], "/"))
		}
	}
	rows = sortLocale(rows, true)
	regen := " " + wtRegenerable + " " + r.getenv("HW_WT_REGENERABLE") + " "
	var keep []string
	for _, rel := range rows {
		if rel == "" {
			continue
		}
		isRegen := false
		for _, comp := range bashWords(strings.ReplaceAll(rel, "/", " ")) {
			if strings.Contains(regen, " "+comp+" ") {
				isRegen = true
				break
			}
		}
		if isRegen || r.copyOfMain(wt, rel) {
			continue
		}
		entry := wt + "/" + rel
		if st, err := os.Lstat(entry); err == nil && st.Mode()&os.ModeSymlink != 0 { // removing a link loses nothing
			continue
		}
		if isDir(entry) {
			out, rc := toolOut("fd", "-H", "-t", "f", ".", entry)
			if rc != 0 {
				keep = append(keep, rel+" (could not be read — fd exited "+itoa(rc)+")")
			} else if bashSub(out) != "" {
				keep = append(keep, rel)
			}
		} else if _, err := os.Lstat(entry); err == nil || fileExists(entry) {
			keep = append(keep, rel)
		}
	}
	return keep
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// disposition is _wt_disposition: sets verdict and why.
func (r *reapRun) disposition(main, wt, branch, base string) {
	r.verdict, r.why, r.evidence, r.archive = "", "", "", nil
	occ := r.occ.of(wt)
	if !occ.observed {
		r.verdict = "undetermined"
		r.why = "could not read who is working in it — " + occ.why + ". An unread occupant list is not an empty one, and occupancy is the gate that outranks every other here."
		return
	}
	if occ.list != "" {
		r.verdict = "held"
		r.why = "held by " + strings.ReplaceAll(occ.list, "\n", " ")
		return
	}
	if lr, ok := leasedRundirFor(wt); ok {
		body, _ := readText(lr + "/lease")
		r.verdict = "leased"
		r.why = "preview leased until " + firstKey(body, "until_h") + " — " + firstKey(body, "why")
		return
	}
	if lockr, ok := lockReason(main, wt); ok {
		r.verdict = "locked"
		paren := ""
		if lockr != "" {
			paren = " (" + lockr + ")"
		}
		r.why = "locked" + paren + " — hw locks a worktree while its executor lives, and an idle one is invisible to lsof; `hw done` unlocks its own, `git worktree unlock` any other"
		return
	}
	if branch == "" || branch == "detached" {
		r.verdict = "detached"
		r.why = "no branch — a detached worktree is not something this tool decides about"
		return
	}
	statusOut, errText, rc := gitStatusErr(wt)
	if rc != 0 {
		if _, grc := gitRC(wt, "rev-parse", "--git-dir"); grc != 0 {
			if _, src := gitRC(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); branch != "" && src == 0 {
				r.verdict = "undetermined"
				r.why = wt + " is not readable as a git worktree, but " + branch + " EXISTS in " + main + " — so this is a worktree whose .git was lost, not a directory that never was one. What it holds has not been measured and its branch has not been merged."
				return
			}
			r.verdict = "not-a-worktree"
			r.why = wt + " is not a git worktree at all and no branch " + branch + " exists — it is a plain directory, which is what a --worktree none task and every pre-2026-09-16 setup task leaves behind. There is nothing here for git to dispose of."
			return
		}
		r.verdict = "undetermined"
		r.why = "git status exited " + itoa(rc) + " in " + wt + " — " + trimTR(errText, 200) + ". Empty output from a FAILED status is the same bytes as a clean tree; it is not a clean tree."
		return
	}
	if statusOut != "" {
		r.verdict = "dirty"
		r.why = itoa(strings.Count(statusOut, "\n")+1) + " uncommitted tracked change(s)"
		return
	}
	if !r.branchMerged(main, branch, base) {
		n := "?"
		if out, rc := gitRC(main, "rev-list", "--count", "origin/"+base+".."+branch); rc == 0 {
			n = bashSub(out)
		} else if out, rc := gitRC(main, "rev-list", "--count", base+".."+branch); rc == 0 {
			n = bashSub(out)
		}
		r.verdict = "unmerged"
		r.why = branch + " has " + n + " commit(s) not in " + base
		return
	}
	keep := r.irreplaceable(wt)
	// ARCHIVABLE: every kept entry is a task output under a WT_ARCHIVABLE name; an entry that could not be read is never archivable
	if len(keep) > 0 {
		var arch []string
		other := false
		for _, rel := range keep {
			if rel == "" {
				continue
			}
			first := rel
			if i := strings.Index(rel, "/"); i >= 0 {
				first = rel[:i]
			}
			if strings.Contains(rel, " (") || !strings.Contains(" "+wtArchivable+" ", " "+first+" ") {
				other = true
				break
			}
			arch = append(arch, rel)
		}
		if !other && len(arch) > 0 {
			r.archive = arch
			r.verdict = "archivable"
			ev := r.evidence
			if ev == "" {
				ev = "merged into " + base
			}
			r.why = ev + " and clean; its only ignored content is task output, archived before removal: " + joinSp(arch)
			return
		}
	}
	if len(keep) > 0 {
		r.verdict = "irreplaceable"
		r.why = "git-ignored and unreproducible: " + joinSp(keep)
		return
	}
	r.verdict = "safe"
	if r.evidence != "" {
		r.why = r.evidence + "; clean, nothing ignored that git cannot rebuild"
	} else {
		r.why = "merged into " + base + ", clean, nothing ignored that git cannot rebuild"
	}
}

// gitStatusErr is `git -C wt status --porcelain 2>file`: stdout with its trailing newlines stripped, stderr, status.
func gitStatusErr(wt string) (string, string, int) {
	cmd := exec.Command("git", "-C", wt, "status", "--porcelain")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := exitStatus(cmd.Run())
	return bashSub(out.String()), errb.String(), rc
}

// ── build output of an idle kept worktree ───────────────────────────────────

// cwdsHold is `lsof -a -d cwd -Fn` read with awk: some process has its cwd at real or under it. ok is false when
// lsof could not answer (failed and printed nothing).
func cwdsHold(real string) (held, ok bool) {
	out, rc := toolOut("lsof", "-Fn", "-a", "-d", "cwd")
	if rc != 0 && out == "" {
		return false, false
	}
	d := "n" + real
	for _, l := range strings.Split(out, "\n") {
		if l == d || strings.HasPrefix(l, d+"/") {
			return true, true
		}
	}
	return false, true
}

// anyFileWithin is `find <dir> -type f -mtime -<days> -print -quit`: a regular file changed less than days days ago.
// err is true when find would have failed (a directory it could not read), which the callers treat as a no.
func anyFileWithin(dir string, days int64) (found, failed bool) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			failed = true
			return nil
		}
		if d.Type().IsRegular() {
			if info, e := d.Info(); e == nil && info.ModTime().After(cutoff) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return
}

// buildIdle is _wt_build_idle: .next/.turbo of a kept worktree idle past the window. THE BIAS IS KEEP.
func (r *reapRun) buildIdle(proj, wt string) bool {
	r.buildDirs, r.buildKB, r.buildAge = nil, 0, 0
	envDays := r.getenv("HW_BUILD_OUTPUT_DAYS")
	if envDays == "" {
		envDays = r.c.t.retBuild
	}
	win := retentionDays(7, envDays)
	if win == 0 {
		return false
	}
	switch r.verdict { // an ALLOWLIST: a live pane reads held, an unread occupant list undetermined, ...
	case "dirty", "unmerged", "irreplaceable": // # MUTATION-ANCHOR: 871-G30
	default:
		return false
	}
	real, err := filepath.EvalSymlinks(wt)
	if err != nil || !isDir(wt) {
		return false
	}
	rootRaw, ok := r.canonicalRoot(proj)
	if !ok {
		return false
	}
	root, err := filepath.EvalSymlinks(rootRaw)
	if err != nil || !isDir(rootRaw) {
		return false
	}
	if !strings.HasPrefix(real, root+"/") {
		return false
	}
	held, ok := cwdsHold(real) // # MUTATION-ANCHOR: 871-G31
	if !ok || held {
		return false
	}
	out, _ := toolOut("fd", "-H", "-I", "-t", "d", "-d", "4", "-E", "node_modules", `^\.(next|turbo)$`, wt)
	var kb, newest int64
	var dirs []string
	for _, d := range strings.Split(out, "\n") {
		d = strings.TrimSuffix(d, "/")
		if d == "" || !isDir(d) {
			continue
		}
		if st, err := os.Lstat(d); err != nil || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if _, rc := gitRC(wt, "check-ignore", "-q", d); rc != 0 {
			continue
		}
		if found, failed := anyFileWithin(d, win); found || failed { // # MUTATION-ANCHOR: 871-G32
			return false
		}
		dirs = append(dirs, d)
		du, _ := toolOut("du", "-sk", d)
		if f := bashWords(du); len(f) > 0 {
			n, _ := pyInt64(f[0])
			kb += n
		}
		st, err := os.Stat(d)
		if err != nil {
			return false
		}
		if m := st.ModTime().Unix(); m > newest {
			newest = m
		}
	}
	if len(dirs) == 0 {
		return false
	}
	// git's own last touch counts too: a commit or checkout is activity. The reflog, NOT the index.
	if lp, rc := gitRC(wt, "rev-parse", "--git-path", "logs/HEAD"); rc == 0 {
		if f := bashSub(lp); f != "" {
			if !filepath.IsAbs(f) {
				f = wt + "/" + f
			}
			if _, err := os.Lstat(f); err == nil {
				if st, err := os.Stat(f); err == nil && st.ModTime().After(time.Now().Add(-time.Duration(win)*24*time.Hour)) {
					return false
				}
			}
		}
	}
	age := (nowSec() - newest) / 86400
	if age < win {
		age = win
	}
	r.buildDirs, r.buildKB, r.buildAge = dirs, kb, age
	return true
}

// ── the loose work dirs ─────────────────────────────────────────────────────

// artifactsDirFor is _artifacts_dir_for: where a task's artifacts go.
func (r *reapRun) artifactsDirFor(proj, wd string) string {
	if !r.laneRepo(proj) {
		return wd
	}
	if root := r.c.lanePath(proj, "worktree_root", ""); root != "" && root == r.c.work+"/"+proj && !fileExists(wd+"/.git") {
		return wd
	}
	return wd + "/.artifacts"
}

// runReported is _run_reported: the newest run of the dir carries its current task's done marker, was not reopened
// and is not a BLOCKED report still waiting for a ruling.
func runReported(dir string) bool {
	ids := RunIDs(dir + "/.hw")
	if len(ids) == 0 {
		return false
	}
	newest := dir + "/.hw/" + ids[0]
	return isFile(runDoneMarker(newest)) && !isFile(runReopenedMarker(newest)) && !isFile(runBlockedMarker(newest))
}

func artifactIsBackup(p string) bool {
	b := strings.ToLower(filepath.Base(p))
	return strings.HasSuffix(b, ".tgz") || strings.HasSuffix(b, ".tar.gz") || strings.HasSuffix(b, ".tar") || strings.Contains(b, "backup")
}

// retentionExpired is _retention_expired: every artifact is past its own window; sets retAge, retWin, retKnd.
func (r *reapRun) retentionExpired(files []string) bool {
	artEnv, bakEnv := r.getenv("HW_ARTIFACT_RETENTION_DAYS"), r.getenv("HW_BACKUP_RETENTION_DAYS")
	if artEnv == "" {
		artEnv = r.c.t.retArtifact
	}
	if bakEnv == "" {
		bakEnv = r.c.t.retBackup
	}
	art, bak := retentionDays(30, artEnv), retentionDays(60, bakEnv)
	r.retAge, r.retWin, r.retKnd = "", "", ""
	now := nowSec()
	best, have := int64(0), false
	for _, f := range files {
		if f == "" {
			continue
		}
		win, kind := art, "artifact"
		if artifactIsBackup(f) {
			win, kind = bak, "backup"
		}
		if win == 0 {
			return false
		}
		st, err := os.Stat(f)
		if err != nil {
			return false
		}
		age := (now - st.ModTime().Unix()) / 86400
		if age <= win { // # MUTATION-ANCHOR: 871-G33
			return false
		}
		if slack := age - win; !have || slack < best {
			best, have = slack, true
			r.retAge, r.retWin, r.retKnd = strconv.FormatInt(age, 10), strconv.FormatInt(win, 10), kind
		}
	}
	return have
}

// workdirDisposition is _workdir_disposition: sets wdVerdict, wdWhy, wdRetAge.
func (r *reapRun) workdirDisposition(proj, dir string) {
	r.wdVerdict, r.wdWhy, r.wdRetAge = "", "", ""
	expired := false
	if !isDir(dir) {
		r.wdVerdict, r.wdWhy = "undetermined", dir+" is not a directory"
		return
	}
	// GIT FIRST: anything git can see belongs to the worktree pass and must not be ruled on twice
	phys := dir
	if p, err := filepath.EvalSymlinks(dir); err == nil {
		phys = p
	}
	topOut, rc := gitRC(dir, "rev-parse", "--show-toplevel")
	if rc == 0 {
		top := bashSub(topOut)
		if fileExists(dir+"/.git") || top == dir || top == phys {
			r.wdVerdict = "worktree"
			r.wdWhy = "a git worktree — the worktree pass owns it and this one does not reclassify it"
		} else {
			r.wdVerdict = "inside-worktree"
			r.wdWhy = "not a loose work dir: it sits inside the git worktree " + top + ", and what happens to it is git's question, not this one's"
		}
		return
	}
	if rc != 128 {
		r.wdVerdict = "undetermined"
		r.wdWhy = "git rev-parse --show-toplevel exited " + itoa(rc) + " in " + dir + ", so whether git owns this directory was never established — 128 is the only exit that means 'not a repository'"
		return
	}
	// THE SAME THREE-VALUED OCCUPANCY GATE AS A WORKTREE'S
	occ := r.occ.of(dir)
	if !occ.observed {
		r.wdVerdict, r.wdWhy = "undetermined", "could not read who is working in it — "+occ.why
		return
	}
	if occ.list != "" {
		r.wdVerdict, r.wdWhy = "held", "held by "+strings.ReplaceAll(occ.list, "\n", " ")
		return
	}
	if lr, ok := leasedRundirFor(dir); ok {
		body, _ := readText(lr + "/lease")
		r.wdVerdict, r.wdWhy = "leased", "preview leased until "+firstKey(body, "until_h")+" — "+firstKey(body, "why")
		return
	}
	// WHAT THE TASK PRODUCED, read from where the task was TOLD to put it
	adir := r.artifactsDirFor(proj, dir) // # MUTATION-ANCHOR: 871-G35
	if isDir(adir) {
		out, frc := toolOut("fd", "-H", "-t", "f", ".", adir)
		if frc != 0 && frc != 1 {
			r.wdVerdict = "undetermined"
			r.wdWhy = "could not read the artifact store " + adir + " — fd exited " + itoa(frc) + ". An unreadable directory is not an empty one, and this is the gate that decides whether anything is being destroyed"
			return
		}
		var files []string
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if adir == dir && strings.HasPrefix(l, dir+"/.hw/") {
				continue
			}
			files = append(files, l)
		}
		n := 0
		for _, l := range files {
			if strings.TrimFunc(l, func(c rune) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f' }) != "" {
				n++
			}
		}
		if n != 0 {
			// RETENTION: reported AND every artifact past its window. Anything else is the old verdict.
			if runReported(dir) && r.retentionExpired(files) { // # MUTATION-ANCHOR: 871-G36
				expired = true
			} else {
				r.wdVerdict = "artifacts"
				r.wdWhy = itoa(n) + " file(s) under its artifact store " + adir + " — artifacts are the one thing a work dir exists to hold, and nothing here can rebuild them"
				return
			}
		}
	}
	// STILL MOVING: a dispatch seconds old looks exactly like abandoned garbage from the outside
	now := time.Now()
	ago := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()-24, now.Minute(), now.Second(), 0, time.Local)
	if st, err := os.Stat(dir); err == nil && st.ModTime().Unix() > ago.Unix() {
		r.wdVerdict, r.wdWhy = "recent", "modified within the last 24h — too young to call abandoned"
		return
	}
	// AND THE TREE, not just the root
	recent, rrc := toolOut("fd", "-H", "--changed-within", "24h", ".", dir)
	if rrc != 0 && rrc != 1 {
		r.wdVerdict, r.wdWhy = "undetermined", "could not read the directory's contents to date them — fd exited "+itoa(rrc)
		return
	}
	if strings.TrimSpace(recent) != "" {
		r.wdVerdict, r.wdWhy = "recent", "something under it was written within the last 24h — too young to call abandoned"
		return
	}
	r.wdVerdict = "safe"
	if expired {
		r.wdRetAge = r.retAge
		r.wdWhy = "retention: " + r.retAge + "d > " + r.retWin + "d (" + r.retKnd + " window); its task reported, nobody in it, not a git worktree"
		return
	}
	r.wdWhy = "no artifacts, not a git worktree, nothing under it touched in 24h, nobody in it"
}

// ── the merged task branches no worktree holds ──────────────────────────────

// orphanBranches is _reap_orphan_branches: the lane's own task branches, merged, that no worktree holds.
func (r *reapRun) orphanBranches(proj, main, base string) []string {
	full := r.c.lanePath(proj, "branch", "X")
	if full == "" {
		full = "task/X"
	}
	prefix := strings.TrimSuffix(full, "X")
	if prefix == "" || prefix == full {
		return nil
	}
	wl, rc := gitRC(main, "worktree", "list", "--porcelain")
	if rc != 0 {
		return nil
	}
	held := map[string]bool{}
	for _, l := range strings.Split(wl, "\n") {
		if strings.HasPrefix(l, "branch refs/heads/") {
			held[l[len("branch refs/heads/"):]] = true
		}
	}
	refs, _ := gitRC(main, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+prefix)
	var out []string
	for _, b := range strings.Split(refs, "\n") {
		if b == "" || held[b] {
			continue
		}
		if r.branchMerged(main, b, base) {
			out = append(out, b)
		}
	}
	return out
}

// ── cmd_reap ────────────────────────────────────────────────────────────────

func (r *reapRun) run(projFilter string) int {
	a := r.app
	totalSafe, totalKept := 0, 0
	var buildKBTotal int64
	for _, proj := range a.c.t.survey {
		if projFilter != "" && projFilter != proj {
			continue
		}
		if !a.laneRepo(proj) {
			continue // _repo_lanes: a lane with no checkout has no worktrees to survey
		}
		main := a.c.checkout(proj)
		if main == "" || !isDir(main) {
			continue
		}
		base := a.laneBaseOrMain(proj)
		a.printf("\n  %s%s%s  (base %s)\n", a.cB, proj, a.c0, base)
		laneSafe0 := totalSafe
		if from := a.c.t.get(proj, "reap_copies_from"); from != "" {
			r.seedPatterns(main + "/" + from)
			a.info(r.seedNote)
		}
		wl, _ := gitRC(main, "worktree", "list", "--porcelain")
		var wts []string
		for _, l := range strings.Split(wl, "\n") {
			if strings.HasPrefix(l, "worktree ") {
				wts = append(wts, l[len("worktree "):])
			}
		}
		if len(wts) > 0 {
			wts = wts[1:] // the main checkout is git's first entry
		}
		for _, wt := range wts {
			if wt == "" || wt == main {
				continue
			}
			branch := "detached"
			if out, rc := gitRC(wt, "symbolic-ref", "--short", "HEAD"); rc == 0 {
				branch = bashSub(out)
			}
			// a database this worktree's own runs recorded, before anything removes it
			if a.c.t.get(proj, "db_provisioned") != "" {
				r.recordDatabases(proj, wt)
			}
			r.disposition(main, wt, branch, base)
			if r.verdict == "safe" || r.verdict == "archivable" {
				totalSafe++
				a.ok(filepath.Base(wt) + "  " + a.cDim + r.why + a.c0)
				if r.verdict != "archivable" {
					continue
				}
				a.printf("      %sarchive to %s/%s/%s, then remove%s\n", a.cDim, a.archiveRoot(), proj, filepath.Base(wt), a.c0)
				continue
			}
			totalKept++
			a.printf("  %s!%s %-42s %s%s: %s%s\n", a.cWarn, a.c0, filepath.Base(wt), a.cDim, r.verdict, r.why, a.c0)
			if r.buildIdle(proj, wt) {
				bwin := retentionDays(7, orStr(a.getenv("HW_BUILD_OUTPUT_DAYS"), a.c.t.retBuild))
				buildKBTotal += r.buildKB
				var rels []string
				for _, d := range r.buildDirs {
					rels = append(rels, strings.TrimPrefix(d, wt+"/"))
				}
				a.printf("      %sbuild output of %s idle %sd > %sd, would free %d MB: %s%s\n", a.cDim, filepath.Base(wt),
					strconv.FormatInt(r.buildAge, 10), strconv.FormatInt(bwin, 10), r.buildKB/1024, strings.Join(rels, " ")+" ", a.c0)
			}
		}

		// ── the merged task branches no worktree holds ──
		for _, ob := range r.orphanBranches(proj, main, base) {
			totalSafe++
			a.ok(ob + "  " + a.cDim + "branch merged into " + base + ", no worktree holds it" + a.c0)
		}

		// ── second pass: the loose work dirs ──
		wdRoot := a.c.work + "/" + proj
		looseSeen := 0
		if isDir(wdRoot) {
			out, _ := toolOut("fd", "-H", "-t", "d", "-d", "1", ".", wdRoot)
			var dirs []string
			for _, d := range strings.Split(out, "\n") {
				if d = strings.TrimSuffix(d, "/"); d != "" {
					dirs = append(dirs, d)
				}
			}
			for _, wd := range sortLocale(dirs, false) {
				r.workdirDisposition(proj, wd)
				if r.wdVerdict == "worktree" {
					continue
				}
				looseSeen++
				if r.wdVerdict == "safe" {
					totalSafe++
					a.ok(filepath.Base(wd) + "  " + a.cDim + "loose work dir: " + r.wdWhy + a.c0)
				} else {
					totalKept++
					a.printf("  %s!%s %-42s %s%s: %s%s\n", a.cWarn, a.c0, filepath.Base(wd), a.cDim, r.wdVerdict, r.wdWhy, a.c0)
				}
			}
		}
		if looseSeen == 0 {
			a.info("no loose work dirs under " + wdRoot)
		}

		// ── the databases hw recorded whose worktree is gone ──
		if a.c.t.get(proj, "db_provisioned") != "" {
			totalSafe += r.databases(proj)
		}

		// ── build output: the weight `df` actually gets back ──
		bkb := int64(0)
		if broot, ok := a.canonicalRoot(proj); ok && isDir(broot) {
			out, _ := toolOut("fd", "-H", "-I", "-t", "d", "-d", "4", "-E", "node_modules", `^\.(next|turbo)$`, broot)
			for _, d := range strings.Split(out, "\n") {
				if d == "" {
					continue
				}
				du, _ := toolOut("du", "-sk", d)
				if f := bashWords(du); len(f) > 0 {
					n, _ := pyInt64(f[0])
					bkb += n
				}
			}
			a.info("build output (.next/.turbo) under " + broot + ": " + strconv.FormatInt(bkb/1048576, 10) + " GB")
		}
		// what `hw status` reads instead of repeating this survey: <build KB> <epoch> <disposable>
		if a.getenv("HW_CORE_READONLY") == "" {
			line := fmt.Sprintf("%d %d %d\n", bkb, nowSec(), totalSafe-laneSafe0)
			os.WriteFile(a.c.brain+"/.hw-reap-"+proj, []byte(line), 0o666)
		}
	}

	// ── third pass: executors that reported BLOCKED and are still waiting ──
	hours := blockedWaitHoursBash(a.getenv)
	seenB := false
	for _, rd := range a.hwRunDirs(true) {
		mk := runBlockedMarker(rd)
		if !isFile(mk) {
			continue
		}
		bproj, btask, bpane := blockedField(mk, "project"), blockedField(mk, "task"), blockedField(mk, "pane")
		if bproj == "" || btask == "" {
			continue
		}
		if projFilter != "" && projFilter != bproj {
			continue
		}
		if !seenB {
			a.printf("\n  %sblocked, waiting for a ruling%s  (closed after %sh unanswered — HW_BLOCKED_WAIT_HOURS)\n", a.cB, a.c0, hours)
			seenB = true
		}
		left := a.blockedSecondsLeft(mk)
		if left > 0 {
			pane := bpane
			if pane == "" {
				pane = "<pane>"
			}
			a.printf("  %s·%s %-42s %swaiting, %dh%02dm left — hw ruling %s \"<the unblock>\" resumes it%s\n",
				a.cDim, a.c0, bproj+":"+btask, a.cDim, left/3600, left%3600/60, pane, a.c0)
		} else {
			a.ok(bproj + ":" + btask + "  " + a.cDim + "reported BLOCKED, unanswered for over " + hours + "h" + a.c0)
			a.printf("      %shw done %s %s --blocked%s\n", a.cDim, bproj, btask, a.c0)
		}
	}

	a.printf("\n")
	if buildKBTotal != 0 {
		a.info("build output of idle kept worktrees: " + strconv.FormatInt(buildKBTotal/1024, 10) + " MB would be freed by --apply (build_output_days)")
	}
	a.info(itoa(totalSafe) + " safe to remove, " + itoa(totalKept) + " kept. Add --apply to remove the safe ones.")
	if totalKept != 0 {
		a.info("nothing kept is ever removed by --apply, whatever the reason says")
		a.info("an ignored path that IS disposable here: HW_WT_REGENERABLE='<name> <name>' hw reap")
	}
	return 0
}

// archiveRoot is _archive_root: ${HW_ARCHIVE_ROOT:-$(dirname "$WORK")/archive}.
func (a *app) archiveRoot() string {
	if d := a.getenv("HW_ARCHIVE_ROOT"); d != "" {
		return d
	}
	return filepath.Dir(a.c.work) + "/archive"
}

// ── the database ledger ─────────────────────────────────────────────────────

// recordDatabases is the loop of cmd_reap that adds the databases a worktree's runs recorded to the ledger.
func (r *reapRun) recordDatabases(proj, wt string) {
	var dbs []string
	for _, rcpt := range pyGlobStarGlob(wt + "/.hw/2*/receipt.jsonl") {
		vals, _ := readStream(rcpt)
		for _, v := range vals {
			if k, ok := v.get("key"); ok && k.kind == vStr && k.s == "database" {
				if val, ok := v.get("value"); ok {
					dbs = append(dbs, val.raw())
				}
			}
		}
	}
	for _, db := range sortLocale(dbs, true) {
		r.ledgerAdd(proj, db, wt)
	}
}

// pyGlobStarGlob is a shell glob of one pattern, in the locale's order.
func pyGlobStarGlob(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	return sortLocale(m, false)
}

// ledgerAdd is _db_ledger_add: one `<db> TAB <wt>` row, once.
func (r *reapRun) ledgerAdd(proj, db, wt string) {
	if proj == "" || db == "" || wt == "" || r.getenv("HW_CORE_READONLY") != "" {
		return
	}
	f := r.c.brain + "/.hw-db-ledger-" + proj
	row := db + "\t" + wt
	if body, ok := readText(f); ok {
		for _, l := range strings.Split(body, "\n") {
			if l == row {
				return
			}
		}
	}
	if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666); err == nil {
		fh.WriteString(row + "\n")
		fh.Close()
	}
}

// databases is the ledger pass of cmd_reap: a database whose worktree is gone, listed as safe to drop. It returns
// how many are.
func (r *reapRun) databases(proj string) int {
	a := r.app
	body, ok := readText(a.c.brain + "/.hw-db-ledger-" + proj)
	if !ok || !readableFile(a.c.brain+"/.hw-db-ledger-"+proj) {
		return 0
	}
	n := 0
	user := whoami()
	for _, l := range strings.Split(body, "\n") {
		ldb, lwt, _ := strings.Cut(l, "\t")
		if ldb == "" || lwt == "" {
			continue
		}
		if isDir(lwt) {
			continue
		}
		out, _ := toolOut("psql", "-U", user, "-d", "postgres", "-Atc", "select 1 from pg_database where datname = '"+ldb+"'")
		if !strings.Contains(out, "1") {
			continue // already gone: the row goes too
		}
		if ldb != strings.NewReplacer("-", "_", "/", "_").Replace(filepath.Base(lwt)) {
			a.warn("kept database " + ldb + " — recorded for " + lwt + ", whose name does not derive it; drop it by hand if it is disposable")
			continue
		}
		n++
		a.ok("database " + ldb + "  " + a.cDim + "its worktree " + filepath.Base(lwt) + " is gone; pg_dump into the archive, then drop" + a.c0)
	}
	return n
}
