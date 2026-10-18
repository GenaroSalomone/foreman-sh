package main

// hw train add|status|push (bin/hw cmd_train + bin/hw-train): the merge train for a lane whose repo is brain itself.
// Accepted task branches go into one integration branch `train-<project>`, the full suite runs ONCE on it, and the
// base advances to it without touching other brainers' uncommitted edits.
//
// A merge here never touches a working tree: `git merge-tree --write-tree` computes it, `commit-tree` records it,
// `update-ref` moves the branch with the old value as a guard. The only conflict resolved is setup/test-budgets.json
// (t6_train_budgets.go). The base is advanced, not pushed.
//
// Every message is hw-train's own (`say` on stdout, `refuse` on stderr, status 1), not bin/hw's glyph helpers; only
// cmd_train's own checks (usage, unknown project, "not brain") use `die`.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const trainUsage = "usage: hw train add <project> <task> | status <project> | push <project>"

// trainBudgetsPath is the one file whose conflict the train resolves.
const trainBudgetsPath = "setup/test-budgets.json"

type trainRun struct {
	*app
	co, base, proj string // HW_TRAIN_CHECKOUT, HW_TRAIN_BASE, the canonical project
	taskBranch     string // HW_TRAIN_TASK_BRANCH
	taskWT         string // HW_TRAIN_TASK_WT
	t, tref        string // train-<project>, refs/heads/train-<project>
	state          string // <git common dir>/hw-train
	lockDir, pushL string

	mu                 sync.Mutex
	haveLock, havePush bool
}

func (a *app) cmdTrain() int {
	args := a.args
	verb := ""
	if len(args) > 0 {
		verb = args[0]
	}
	switch verb {
	case "add", "status", "push":
	default:
		a.die(trainUsage)
	}
	if len(args) < 2 {
		suffix := ""
		if verb == "add" {
			suffix = " <task>"
		}
		a.die("usage: hw train " + verb + " <project>" + suffix)
	}
	proj := a.c.t.canonProject(args[1])
	if !a.c.t.known(proj) {
		a.die("unknown project: " + args[1] + " (expected: " + a.laneNamesHint() + ")")
	}
	co := a.c.checkout(proj)
	if co == "" || !isDir(co) || trainPhys(co) != trainPhys(a.c.brain) {
		a.die("hw train: " + proj + "'s checkout is not brain (" + a.c.brain + "), so it has no train — its repo lands through PRs")
	}
	r := &trainRun{app: a, co: co, proj: proj}
	if verb == "add" {
		if len(args) != 3 {
			a.die("usage: hw train add <project> <task>")
		}
		(&dsp{app: a}).checkTask(args[2])
		r.taskBranch = a.c.lanePath(proj, "branch", args[2])
		r.taskWT = a.c.lanePath(proj, "worktree", args[2])
	} else if len(args) != 2 {
		a.die("usage: hw train " + verb + " <project>")
	}
	r.base = a.c.t.get(proj, "base_ref_prefix") + a.c.t.get(proj, "base")
	return r.main(verb, args[2:])
}

// trainPhys is `cd -P "$p" && pwd`.
func trainPhys(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// ── hw-train's own vocabulary ────────────────────────────────────────────────

func (r *trainRun) say(format string, v ...any) {
	r.printf("hw train: %s\n", fmt.Sprintf(format, v...))
}

func (r *trainRun) refuse(format string, v ...any) {
	r.errf("hw train: %s\n", fmt.Sprintf(format, v...))
	panic(exitNow{1})
}

// g is `git -C "$CO" …`; stderr is kept for the caller to forward or drop.
func (r *trainRun) g(args ...string) extResult {
	return extRun(extCall{Name: "git", Args: append([]string{"-C", r.co}, args...), KeepErr: true})
}

// gv is `$(g …)` under set -e: the output without trailing newlines, or the verb ends with git's status and its
// stderr shown.
func (r *trainRun) gv(args ...string) string {
	res := r.g(args...)
	if res.RC != 0 {
		r.gFail(res)
	}
	return bashSub(res.Out)
}

// gx is a bare `g …` under set -e: git's stderr reaches the terminal, a failure ends the verb with git's status.
func (r *trainRun) gx(args ...string) {
	res := r.g(args...)
	r.writeErr(res)
	if res.RC != 0 {
		panic(exitNow{res.RC})
	}
}

func (r *trainRun) gFail(res extResult) {
	r.writeErr(res)
	panic(exitNow{res.RC})
}

func (r *trainRun) writeErr(res extResult) {
	if res.Err != "" {
		io.WriteString(r.err, res.Err)
	}
}

func (r *trainRun) short(rev string) string { return r.gv("rev-parse", "--short", rev) }

// tip is `git rev-parse --verify --quiet "$1^{commit}" 2>/dev/null`.
func (r *trainRun) tip(ref string) (string, bool) {
	res := r.g("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if res.RC != 0 {
		return "", false
	}
	return bashSub(res.Out), true
}

// has is `git merge-base --is-ancestor $1 $2`.
func (r *trainRun) has(a, b string) bool { return r.g("merge-base", "--is-ancestor", a, b).RC == 0 }

// ── locks ────────────────────────────────────────────────────────────────────

func (r *trainRun) release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.haveLock {
		os.RemoveAll(r.lockDir)
		r.haveLock = false
	}
	if r.havePush {
		os.RemoveAll(r.pushL)
		r.havePush = false
	}
}

// take is `take <dir> <what its holder is doing>`: mkdir is atomic, a lock whose holder is gone is taken over.
func (r *trainRun) take(dir, what string) {
	for {
		err := os.Mkdir(dir, 0o777)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			r.refuse("cannot take the lock %s: %v", dir, err)
		}
		raw, _ := os.ReadFile(dir + "/pid")
		pid := bashSub(string(raw))
		if n, err := strconv.Atoi(pid); err == nil && n > 0 && pidAlive(n) {
			r.refuse("the %s train is %s by pid %s — run again when it finishes", r.t, what, pid)
		}
		os.RemoveAll(dir)
	}
	os.WriteFile(dir+"/pid", []byte(strconv.Itoa(os.Getpid())+"\n"), 0o666)
}

func (r *trainRun) lock() {
	r.take(r.lockDir, "being changed")
	r.mu.Lock()
	r.haveLock = true
	r.mu.Unlock()
}

func (r *trainRun) unlock() {
	r.mu.Lock()
	os.RemoveAll(r.lockDir)
	r.haveLock = false
	r.mu.Unlock()
}

// ── the script ───────────────────────────────────────────────────────────────

// main is hw-train's body: its own argument check, the lanes' refusals, the locks and the three verbs.
func (r *trainRun) main(verb string, rest []string) (rc int) {
	if verb == "" || r.proj == "" || r.co == "" || r.base == "" {
		r.refuse(trainUsage)
	}
	if _, ok := r.tip("refs/heads/" + r.base); !ok {
		r.refuse("the base branch %s does not exist in %s", r.base, r.co)
	}
	r.t = "train-" + r.proj
	r.tref = "refs/heads/" + r.t
	// COMMON="$(cd "$CO" && cd "$(git rev-parse --git-common-dir)" && pwd -P)"
	cd := r.gv("rev-parse", "--git-common-dir")
	if !filepath.IsAbs(cd) {
		cd = filepath.Join(r.co, cd)
	}
	common, err := filepath.EvalSymlinks(cd)
	if err != nil {
		return 1
	}
	r.state = common + "/hw-train"
	if err := os.MkdirAll(r.state, 0o777); err != nil {
		r.errf("mkdir: %s: %v\n", r.state, err)
		return 1
	}
	r.lockDir = r.state + "/" + r.proj + ".lock"
	r.pushL = r.state + "/" + r.proj + ".push.lock"
	defer r.release() // trap release EXIT
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		if s, ok := <-sig; ok {
			r.release()
			n := 128
			if ss, ok := s.(syscall.Signal); ok {
				n += int(ss)
			}
			os.Exit(n)
		}
	}()
	switch verb {
	case "add":
		r.cmdAdd(rest)
	case "status":
		r.cmdStatus()
	case "push":
		r.cmdPush()
	default:
		r.refuse("unknown verb %s — %s", verb, trainUsage)
	}
	return 0
}

// trainTasks is train_tasks: the train's own merges, newest first, "<sha> <task branch>".
func (r *trainRun) trainTasks() [][2]string {
	res := r.g("log", "--first-parent", "--merges", "--format=%H %s", r.base+".."+r.tref)
	re := regexp.MustCompile(`^([0-9a-f]*) merge\(train\): (.*) into ` + regexp.QuoteMeta(r.t) + `$`)
	var out [][2]string
	if res.RC != 0 {
		return nil
	}
	for _, line := range strings.Split(strings.TrimRight(res.Out, "\n"), "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			out = append(out, [2]string{m[1], m[2]})
		}
	}
	return out
}

// mergeIn records "merge(train): <label> into <T>" on the train, or refuses naming every conflict.
func (r *trainRun) mergeIn(theirs, label string) {
	ours, _ := r.tip(r.tref)
	res := extRun(extCall{Name: "git", Args: []string{"-C", r.co, "merge-tree", "--write-tree", "--name-only", ours, theirs}, Merge: true})
	out := bashSub(res.Out)
	rc := res.RC
	if rc > 1 {
		r.refuse("git merge-tree failed merging %s: %s", label, out)
	}
	lines := strings.Split(out, "\n")
	tree := lines[0]
	// files: sed -n '2,/^$/p' | sed '/^$/d'
	var files []string
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		files = append(files, l)
	}
	// msgs: sed -n '/^$/,$p' | grep '^CONFLICT'
	var msgLines []string
	seenBlank := false
	for _, l := range lines {
		if l == "" {
			seenBlank = true
		}
		if seenBlank && strings.HasPrefix(l, "CONFLICT") {
			msgLines = append(msgLines, l)
		}
	}
	msgs := strings.Join(msgLines, "\n")
	if rc == 1 {
		var blob string
		resolved := false
		if strings.Join(files, "\n") == trainBudgetsPath &&
			msgs == "CONFLICT (content): Merge conflict in "+trainBudgetsPath {
			if b, ok := r.resolveBudgets(r.gvQuiet("merge-base", ours, theirs), ours, theirs); ok {
				h := r.g2(b, "hash-object", "-w", "--stdin")
				if h.RC == 0 {
					blob, resolved = bashSub(h.Out), true
				}
			}
		}
		if resolved {
			tree = r.treeWithBudgets(tree, blob)
			r.say("%s: conflict resolved key by key", trainBudgetsPath)
		} else {
			r.refuseConflicts(label, files, msgLines)
		}
	}
	nw := r.gv("commit-tree", tree, "-p", ours, "-p", theirs, "-m", "merge(train): "+label+" into "+r.t)
	r.gx("update-ref", "-m", "hw train: "+label, r.tref, nw, ours)
	r.say("merged %s into %s (%s)", label, r.t, r.short(nw))
}

// gvQuiet is `$(g …)` inside an argument whose failure the caller does not check: empty on failure.
func (r *trainRun) gvQuiet(args ...string) string {
	res := r.g(args...)
	r.writeErr(res)
	return bashSub(res.Out)
}

// g2 is `… | g <args>`: git with a stdin.
func (r *trainRun) g2(stdin string, args ...string) extResult {
	return extRun(extCall{Name: "git", Args: append([]string{"-C", r.co}, args...), Stdin: stdin, KeepErr: true})
}

// treeWithBudgets swaps the resolved budgets blob into the merge's tree, through a throwaway index.
func (r *trainRun) treeWithBudgets(tree, blob string) string {
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	f, err := os.CreateTemp(tmp, "hw-train-index.")
	if err != nil {
		r.errf("mktemp: %v\n", err)
		panic(exitNow{1})
	}
	idx := f.Name()
	f.Close()
	defer os.Remove(idx)
	env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
	run := func(args ...string) string {
		res := extRun(extCall{Name: "git", Env: env, Args: append([]string{"-C", r.co}, args...), KeepErr: true})
		if res.RC != 0 {
			r.gFail(res)
		}
		r.writeErr(res)
		return bashSub(res.Out)
	}
	run("read-tree", tree)
	run("update-index", "--cacheinfo", "100644,"+blob+","+trainBudgetsPath)
	return run("write-tree")
}

// resolveBudgets is resolve_budgets: the merged file, or false when a side does not parse.
func (r *trainRun) resolveBudgets(base, ours, theirs string) (string, bool) {
	show := func(rev string) (string, bool) {
		res := r.g("show", rev+":"+trainBudgetsPath)
		if res.RC != 0 {
			return "", false
		}
		return res.Out, true
	}
	b, ok := show(base)
	if !ok {
		b = "{}"
	}
	o, ok := show(ours)
	if !ok {
		return "", false
	}
	t, ok := show(theirs)
	if !ok {
		return "", false
	}
	return trainMergeBudgets(b, o, t)
}

// refuseConflicts is the refusal naming each conflict and who should resolve it, all on stderr.
func (r *trainRun) refuseConflicts(label string, files, msgLines []string) {
	r.errf("hw train: %s does not merge into %s — nothing was recorded.\n", label, r.t)
	if len(msgLines) == 0 {
		r.errf("    \n") // printf '%s\n' "" | sed 's/^/    /'
	}
	for _, l := range msgLines {
		r.errf("    %s\n", l)
	}
	tasks := r.trainTasks()
	for _, f := range files {
		f = strings.Trim(f, " \t") // read -r f
		if f == "" {
			continue
		}
		owners := ""
		for _, t := range tasks {
			if r.g("diff", "--quiet", t[0]+"^1", t[0], "--", f).RC != 0 {
				owners += " " + t[1]
			}
		}
		if owners == "" {
			owners = " " + r.base
		}
		r.errf("  %s — on the train side:%s\n", f, owners)
	}
	if label == r.base {
		r.errf("The train cannot take %s: rebuild it (git branch -D %s) after the conflicting task rebases onto %s.\n", r.base, r.t, r.base)
	} else {
		r.errf("Resolve it in %s: rebase it onto %s in its worktree, then hw train add %s again.\n", label, r.t, r.proj)
	}
	panic(exitNow{1})
}

// syncBase: the train starts at the base, restarts there once it has landed, and takes the base in when it moved.
func (r *trainRun) syncBase() {
	b, _ := r.tip("refs/heads/" + r.base)
	cur, exists := r.tip(r.tref)
	switch {
	case !exists:
		r.gx("update-ref", "-m", "hw train: start", r.tref, b, "")
		r.say("started %s at %s (%s)", r.t, r.base, r.short(b))
	case r.has(r.tref, b):
		if cur != b {
			r.gx("update-ref", "-m", "hw train: landed, restart", r.tref, b)
			r.say("%s had landed in %s; restarted at %s", r.t, r.base, r.short(b))
		}
	case !r.has(b, r.tref):
		r.mergeIn(b, r.base)
	}
}

func (r *trainRun) cmdAdd(rest []string) {
	task := ""
	if len(rest) > 0 {
		task = rest[0]
	}
	br, wt := r.taskBranch, r.taskWT
	if task == "" || br == "" {
		r.refuse("usage: hw train add <project> <task>")
	}
	sha, ok := r.tip("refs/heads/" + br)
	if !ok {
		r.refuse("task %s has no branch %s in %s", task, br, r.co)
	}
	if wt != "" && isDir(wt) {
		st := extRun(extCall{Name: "git", Args: []string{"-C", wt, "status", "--porcelain", "--untracked-files=no"}})
		if st.RC == 0 && bashSub(st.Out) != "" {
			r.say("WARNING: %s has uncommitted tracked changes — they are not on %s and do not travel", wt, br)
		}
	}
	r.lock()
	r.syncBase()
	if r.has(sha, r.tref) {
		r.say("%s (%s) is already in %s — nothing to do", br, r.short(sha), r.t)
	} else {
		r.mergeIn(sha, br)
	}
	r.unlock()
	r.cmdStatus()
}

func (r *trainRun) cmdStatus() {
	if _, ok := r.tip(r.tref); !ok {
		r.say("no %s yet — hw train add %s <task> starts it", r.t, r.proj)
		return
	}
	n := r.gv("rev-list", "--count", r.base+".."+r.tref)
	if n == "0" {
		r.say("%s carries nothing over %s", r.t, r.base)
		return
	}
	r.say("%s: %s commit(s) over %s, tip %s", r.t, n, r.base, r.short(r.tref))
	for _, t := range r.trainTasks() {
		r.printf("    %s\n", t[1])
	}
	if !r.has("refs/heads/"+r.base, r.tref) {
		r.say("%s moved since; the next add or push takes it in", r.base)
	}
}

// gitOut is a git run in the checkout whose stdin is a diff and whose stderr is dropped (`2>/dev/null`).
func (r *trainRun) applyDiff(h, c, f string, check bool) bool {
	d := r.g("diff", h, c, "--", f)
	if d.RC != 0 || d.Out == "" {
		return false
	}
	args := []string{"apply"}
	if check {
		args = append(args, "--check")
	}
	return r.g2(d.Out, args...).RC == 0
}

// advance moves the base from H to C in the checkout, keeping every uncommitted edit in it (chain-push.sh). A dirty
// file the advance also changes gets the advance's diff applied ON TOP of the edit; one that does not apply refuses
// before anything moves.
func (r *trainRun) advance(h, c string) {
	head := bashSub(r.g("symbolic-ref", "-q", "HEAD").Out)
	if head != "refs/heads/"+r.base {
		r.gx("update-ref", "-m", "hw train push", "refs/heads/"+r.base, c, h)
		return
	}
	nameZ := func(args ...string) string { // not through bashSub: it drops the NULs
		res := r.g(args...)
		if res.RC != 0 {
			r.gFail(res)
		}
		return res.Out
	}
	changed := nameZ("diff", "--name-only", "-z", h, c)
	dirty := nameZ("diff", "--name-only", "-z")
	dirtySet := map[string]bool{}
	for _, f := range strings.Split(dirty, "\x00") {
		if f != "" {
			dirtySet[f] = true
		}
	}
	var both []string
	for _, f := range strings.Split(changed, "\x00") {
		if f != "" && dirtySet[f] {
			both = append(both, f)
		}
	}
	F := sortLocale(both, false)
	for _, f := range F {
		if !r.applyDiff(h, c, f, true) {
			r.refuse("LOCAL EDIT CONFLICT: %s/%s has an uncommitted edit the advance does not apply on top of — nothing moved", r.co, f)
		}
	}
	bak := r.state + "/advance-bak-" + strconv.Itoa(os.Getpid())
	for _, f := range F {
		os.MkdirAll(filepath.Dir(bak+"/"+f), 0o777)
		trainCopyP(r.co+"/"+f, bak+"/"+f+".orig")
		r.applyDiff(h, c, f, false)
		trainCopyP(r.co+"/"+f, bak+"/"+f+".merged")
		r.gx("checkout", h, "--", f)
	}
	restore := func() {
		for _, f := range F {
			trainCopyP(bak+"/"+f+".orig", r.co+"/"+f)
		}
	}
	if rt := r.g("read-tree", "-m", "-u", h, c); rt.RC != 0 {
		r.writeErr(rt)
		restore()
		r.refuse("ADVANCE FAILED: git read-tree refused (staged changes or untracked files in the way) — %s stays at %s, local edits restored (copies in %s)", r.base, r.short(h), bak)
	}
	if ur := r.g("update-ref", "-m", "hw train push", "refs/heads/"+r.base, c, h); ur.RC != 0 {
		r.writeErr(ur)
		r.g("read-tree", "-m", "-u", c, h)
		restore()
		r.refuse("ADVANCE FAILED: %s moved during the advance — the tree was put back, local edits restored (copies in %s)", r.base, bak)
	}
	for _, f := range F {
		trainCopyP(bak+"/"+f+".merged", r.co+"/"+f)
	}
	os.RemoveAll(bak)
	if len(F) > 0 {
		r.say("uncommitted edits carried across: %s", strings.Join(F, " ")+" ") // printf '%s ' $F
	}
}

// trainCopyP is `cp -p src dst`: contents, mode and times.
func trainCopyP(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}
	st, err := os.Stat(src)
	if err != nil {
		return
	}
	if os.WriteFile(dst, data, st.Mode().Perm()) != nil {
		return
	}
	os.Chmod(dst, st.Mode().Perm())
	os.Chtimes(dst, time.Now(), st.ModTime())
}

func (r *trainRun) cmdPush() {
	if _, ok := r.tip(r.tref); !ok {
		r.refuse("there is no %s — hw train add %s <task> first", r.t, r.proj)
	}
	r.take(r.pushL, "being pushed")
	r.mu.Lock()
	r.havePush = true
	r.mu.Unlock()
	r.lock()
	r.syncBase()
	r.unlock()
	c, _ := r.tip(r.tref)
	if r.gv("rev-list", "--count", r.base+".."+c) == "0" {
		r.refuse("%s carries nothing over %s — nothing to push", r.t, r.base)
	}
	// The suite runs in the train's own detached worktree under the shared git dir: the verdict cache
	// setup/verify-for-push writes is then the checkout's, and `hw worktrees` lists it as internal, not as a stray.
	w := r.state + "/" + r.proj
	if _, err := os.Stat(w + "/.git"); err == nil {
		res := extRun(extCall{Name: "git", Args: []string{"-C", w, "checkout", "-q", "--detach", "-f", c}, KeepErr: true})
		r.writeErr(res)
		if res.RC != 0 {
			panic(exitNow{res.RC})
		}
	} else {
		r.gx("worktree", "prune")
		r.gx("worktree", "add", "-q", "--detach", w, c)
	}
	if st, err := os.Stat(w + "/setup/verify-for-push"); err != nil || !st.Mode().IsRegular() {
		r.refuse("%s has no setup/verify-for-push, so there is no full suite to run — nothing moved", r.t)
	}
	r.say("full suite on %s at %s in %s", r.t, r.short(c), w)
	if r.runSuite(w) != 0 {
		r.refuse("VERIFY FAILED on %s — %s did not move", r.short(c), r.base)
	}
	h, _ := r.tip("refs/heads/" + r.base)
	if !r.has(h, c) {
		r.refuse("MAIN MOVED: %s is at %s, not in the verified %s — run hw train push %s again", r.base, r.short(h), r.short(c), r.proj)
	}
	r.advance(h, c)
	hooks := r.co + "/setup/install-hooks.sh"
	if st, err := os.Stat(hooks); err == nil && st.Mode().IsRegular() {
		if extRun(extCall{Name: "bash", Args: []string{"setup/install-hooks.sh"}, Dir: r.co}).RC != 0 {
			r.say("WARNING: setup/install-hooks.sh failed after the advance — run it by hand")
		}
	}
	r.say("%s advanced %s -> %s. Not pushed; to publish:", r.base, r.short(h), r.short(c))
	for _, rem := range strings.Fields(r.gv("remote")) {
		r.printf("    git -C %s push %s %s\n", r.co, rem, r.base)
	}
}

// runSuite is `(cd "$W" && bash setup/verify-for-push)`: streamed, the suite takes minutes.
func (r *trainRun) runSuite(w string) int {
	cmd := exec.Command("bash", "setup/verify-for-push")
	cmd.Dir = w
	cmd.Env = append(os.Environ(), "HW_TRAIN_CHECKOUT="+r.co, "HW_TRAIN_BASE="+r.base)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = r.out, r.err, r.stdin
	return exitStatus(cmd.Run())
}
