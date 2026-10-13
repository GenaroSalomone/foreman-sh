package main

// The verification `hw done` runs itself (_verify_at_close and its shortcuts, lib/hw/done.sh), behind the lane's
// suite lock (_suite_lock_take, bin/hw) and one slot of the machine-wide gate (sg_take, bin/suite-gate), and the
// brain-leak scan before anything closes (_brain_leak_at_close, bin/hw). The pinned command runs as bash runs it:
// `(cd <dir> && timeout <secs> bash -c "<cmd>")`, its output hashed into the receipt.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	doneReRed     = regexp.MustCompile(`(?i)^(headline|result|status)=.*(fail|\bred\b)`)
	doneReHex40   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	doneFastGate  = "HW_TEST_GATE=fast bash setup/test-hw"
	doneFullSuite = "bash setup/test-hw"
	doneForPush   = "bash setup/verify-for-push"
)

func doneFileLines(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	s := string(b)
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n"), true
}

// cachedVerdict is _verify_cached_verdict: "<tree> <when>" for a green verdict that names this tree and gate.
func doneCachedVerdict(common, tree, cmd string) (string, bool) {
	dir := common + "/hw-push-verified"
	var cands []string
	switch cmd {
	case doneFastGate:
		cands = []string{dir + "/" + tree + ".fast", dir + "/" + tree}
	case doneFullSuite, doneForPush:
		cands = []string{dir + "/" + tree}
	default:
		return "", false
	}
	for _, f := range cands {
		if !readableFile(f) {
			continue
		}
		lines, _ := doneFileLines(f)
		hasTree, red, fast := false, false, false
		for _, l := range lines {
			if l == "tree="+tree {
				hasTree = true
			}
			if doneReRed.MatchString(l) {
				red = true
			}
			if l == "gate=fast" {
				fast = true
			}
		}
		if !hasTree || red { // # MUTATION-ANCHOR: 861-G22
			continue
		}
		isFast := strings.HasSuffix(f, ".fast")
		if isFast && !fast || !isFast && fast {
			continue
		}
		return tree + " " + doneFirstKey(f, "verified_at"), true
	}
	return "", false
}

// doneDocsCoverScript is the subshell of _verify_docs_cover, run as bash runs it: the lib pre-push sources, asked
// the same question.
const doneDocsCoverScript = `wd="$1" common="$2" tree="$3" lib="$4"
cd "$wd" || exit 1
. "$lib" || exit 1
[ -n "${SUITE_TRIGGER_PATTERN:-}" ] || exit 1
declare -F docs_only_init >/dev/null && declare -F docs_only_diff_covers >/dev/null || exit 1
docs_only_init "$(git rev-parse --show-toplevel)" || exit 1
for f in "$common/hw-push-verified"/*; do
  v="$(basename "$f")"
  case "$v" in *[!0-9a-f]*|'') continue ;; esac
  [ "${#v}" -eq 40 ] || continue
  [ -r "$f" ] || continue
  grep -qxF "tree=$v" "$f" 2>/dev/null || continue
  grep -qxF "gate=fast" "$f" 2>/dev/null && continue
  if grep -qiE '^(headline|result|status)=.*(fail|\bred\b)' "$f" 2>/dev/null; then continue; fi
  git cat-file -e "$v^{tree}" 2>/dev/null || continue
  paths="$(docs_only_diff_covers "$v" "$tree")" || continue
  printf '%s\t%s\t%s' "$v" "$(sed -n 's/^commit=//p' "$f" | head -1)" "$paths"
  exit 0
done
exit 1`

func (d *doneRun) docsCover(wd, common, tree string) (string, bool) {
	brain := d.c.brain
	if brain == "" {
		brain = d.getenv("HW_BRAIN_ROOT")
	}
	lib := brain + "/setup/hooks/suite-trigger-pattern.sh"
	if !readableFile(lib) {
		return "", false
	}
	out, rc := doneTool("", nil, "bash", "-c", doneDocsCoverScript, "hw", wd, common, tree, lib)
	if rc != 0 {
		return "", false
	}
	return bashSub(out), true
}

// verifyShortcut is _verify_shortcut: a clean checkout already measured is decided from disk.
func (d *doneRun) verifyShortcut(rundir, wd, pin string) bool {
	if d.getenv("HW_VERIFY_FRESH") != "" {
		return false
	}
	if !doneExists(wd + "/.git") {
		return false
	}
	headOut, rc := doneGit(wd, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	head := bashSub(headOut)
	if rc != 0 || head == "" {
		return false
	}
	dirty, rc := doneGit(wd, "status", "--porcelain", "--untracked-files=normal")
	if rc != 0 || bashSub(dirty) != "" {
		return false
	}
	base, _ := doneReceiptLast(rundir+"/receipt.jsonl", "review_base")
	if !doneReHex40.MatchString(base) {
		base = ""
	}
	if base != "" && head == base { // # MUTATION-ANCHOR: 861-G24
		d.receiptInto(rundir, "verify_run", "skipped — no change since dispatch ("+base+")",
			"HEAD == review_base recorded at dispatch and git status is clean in "+wd+"; the pinned verification was not run")
		d.info("verification skipped — no change since dispatch (" + base + "); nothing here claims more than that")
		return true
	}
	lines, ok := doneFileLines(pin)
	if !ok {
		return false
	}
	var hits []string
	for _, l := range lines {
		if doneReNonSpace.MatchString(l) {
			hits = append(hits, l)
		}
	}
	if len(hits) != 1 {
		return false
	}
	cmd := doneTrimSpace(hits[0])
	treeOut, rc := doneGit(wd, "rev-parse", "--verify", "-q", "HEAD^{tree}")
	if rc != 0 {
		return false
	}
	tree := bashSub(treeOut)
	commonOut, rc := doneGit(wd, "rev-parse", "--git-common-dir")
	if rc != 0 {
		return false
	}
	common := bashSub(commonOut)
	if !strings.HasPrefix(common, "/") {
		common = wd + "/" + common
	}
	hit, ok := doneCachedVerdict(common, tree, cmd)
	if !ok {
		if cmd != doneFastGate && cmd != doneFullSuite && cmd != doneForPush {
			return false
		}
		cover, ok := d.docsCover(wd, common, tree)
		if !ok {
			return false
		}
		parts := strings.SplitN(cover, "\t", 3)
		for len(parts) < 3 {
			parts = append(parts, "")
		}
		vt, vc, vp := parts[0], parts[1], strings.TrimRight(parts[2], "\n")
		d.receiptInto(rundir, "verify_run",
			"cached — green full verdict for tree "+vt+" ("+vc+"); differs only in lane docs: "+vp,
			common+"/hw-push-verified/"+vt+" is a full green verdict; HEAD's tree "+tree+" differs from it only in lane documentation (docs_only_diff_covers), "+wd+" is clean; "+cmd+" was not re-run")
		d.ok("VERIFIED (cached)  green full verdict for tree " + vt + " (" + vc + "); differs only in lane docs: " + vp + "  " + cmd)
		return true
	}
	hTree, hWhen := hit, ""
	if i := strings.IndexByte(hit, ' '); i >= 0 {
		hWhen = hit[i+1:]
	}
	if i := strings.LastIndexByte(hit, ' '); i >= 0 {
		hTree = hit[:i]
	}
	d.receiptInto(rundir, "verify_run",
		"cached — green verdict for tree "+hTree+" from "+hWhen,
		common+"/hw-push-verified/"+hTree+"[.fast] named HEAD's tree, "+wd+" is clean; "+cmd+" was not re-run")
	d.ok("VERIFIED (cached)  green verdict for tree " + hTree + " from " + hWhen + "  " + cmd)
	return true
}

// doneArtifactsDir is _artifacts_dir_for.
func (d *doneRun) artifactsDir(proj, wd string) string {
	if d.c.checkout(proj) == "" {
		return wd
	}
	if d.worktreeIsWorkdir(proj) && !doneExists(wd+"/.git") {
		return wd
	}
	return wd + "/.artifacts"
}

func doneExists(p string) bool { _, err := os.Stat(p); return err == nil }

// worktreeIsWorkdir is lane_worktree_is_workdir.
func (d *doneRun) worktreeIsWorkdir(proj string) bool {
	r := d.c.lanePath(proj, "worktree_root", "")
	return r != "" && r == d.c.work+"/"+proj
}

// doneNamesFullSuite is _verify_names_full_suite.
func doneNamesFullSuite(cmd string) bool {
	return strings.Contains(cmd, "setup/test-hw") && !strings.Contains(cmd, "HW_TEST_GATE=fast")
}

// doneBashArith is `$(( x ))` of a variable's text: a number, or 0 for what bash would read as an unset name.
func doneBashArith(s string) int64 {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return 0
}

// verifyAtClose is _verify_at_close: the brief's pinned verification, run here, its exit and hash receipted.
func (d *doneRun) verifyAtClose(rundir, workdir, vproj, vtask string) {
	a := d.app
	verifydir := workdir
	if d.worktreeIsWorkdir(vproj) && !doneExists(workdir+"/.git") {
		verifydir = d.c.checkout(vproj)
		if verifydir == "" {
			verifydir = d.c.brain
		}
	}
	rundir = strings.TrimSuffix(rundir, "/")
	if rundir == "" || !isDir(rundir) {
		return
	}
	if d.verifySkip != "" { // # MUTATION-ANCHOR: 861-G25
		d.receiptInto(rundir, "verify_run", "skipped ("+d.verifySkip+")", "hw done "+d.verifySkip+": the pinned verification was not run")
		a.info("verification skipped (" + d.verifySkip + ") — nothing here claims this task was verified")
		return
	}
	pin := rundir + "/verify"
	if !isFile(pin) {
		return
	}
	if st, err := os.Stat(pin); err == nil && st.Size() == 0 {
		d.receiptInto(rundir, "verify_run", "none — the brief declared no runnable command", pin+", pinned from the brief at dispatch, is empty")
		a.warn("NOT VERIFIED: this task's brief declared no command hw could run")
		a.warn("nothing here says the work was checked — the report is the only claim there is")
		return
	}
	if !isDir(verifydir) {
		d.receiptInto(rundir, "verify_run", "not run — "+verifydir+" does not exist", "test -d "+verifydir+" at hw done")
		a.warn("NOT VERIFIED: " + verifydir + " is gone, so the pinned verification could not run")
		return
	}
	if verifydir == workdir && d.verifyShortcut(rundir, workdir, pin) { // # MUTATION-ANCHOR: 861-G26
		a.info("measured: hw receipt " + vproj + " " + vtask)
		return
	}
	artifacts := d.artifactsDir(vproj, workdir)
	secs := a.getenv("HW_VERIFY_TIMEOUT_SECONDS")
	if secs == "" {
		secs = "900"
	}
	var to []string
	if _, err := exec.LookPath("timeout"); err == nil {
		to = []string{"timeout", secs}
	} else if _, err := exec.LookPath("gtimeout"); err == nil {
		to = []string{"gtimeout", secs}
	}
	toS := strings.Join(to, " ")
	vwait := doneBashArith(secs) * 1000
	if w := a.getenv("HW_VERIFY_SUITE_WAIT_MS"); w != "" {
		vwait = doneBashArith(w)
	}
	gateWait := a.getenv("HW_SUITE_GATE_WAIT_MS")
	if gateWait == "" {
		gateWait = "7200000"
	}
	vtaskName := vtask
	if vtaskName == "" {
		vtaskName = "<unnamed task>"
	}
	a.info(fmt.Sprintf("hw done is now running this task's verification — the slow step; NOTHING is closed until it ends. Ceiling %ss per command, plus up to %ds first if another run holds the lane's suite lock, plus up to %sms for a machine suite slot (bin/suite-gate; never a failure, it runs without one past that).", secs, vwait/1000, gateWait))
	vlock := ""
	switch {
	case d.x.get(vproj, "suite_lock") == "none": // # MUTATION-ANCHOR: 861-G27
		a.info("the " + vproj + " lane has no suite lock: its suite shares the machine's worker pool with every other suite and runs its timing subjects alone — see _suite_lane_pooled. This verification takes a machine suite slot and may wait for one.")
	case d.c.work == "":
		a.warn("the " + vproj + " suite lock could not be located — WORK is unset, so this verification runs UNSERIALISED")
		a.warn("if somebody else is running that lane's suite right now, these two are on the same tree")
	default:
		vlock = d.c.work + "/.locks/suite-" + vproj
		if os.MkdirAll(doneDirname(vlock), 0o777) != nil {
			a.warn("the " + vproj + " suite lock could not be created under " + d.c.work + " — this verification runs UNSERIALISED")
			a.warn("if somebody else is running that lane's suite right now, these two are on the same tree")
			vlock = ""
		}
	}
	held := false
	extraEnv := map[string]string{"HW_ARTIFACTS": artifacts}
	prevMarker := a.getenv("HW_SUITE_LOCK_HELD")
	switch {
	case vlock == "":
	case doneLockHeldByAncestor(vlock, vproj, prevMarker):
		a.info("the " + vproj + " suite lock is already held further up this process tree; this verification runs inside that hold")
		vlock = ""
	default:
		token, ok := d.suiteLockTake(vlock, vproj, vwait, "hw done verification of "+vtaskName)
		if !ok { // # MUTATION-ANCHOR: 861-G28
			holder := doneHolderLine(vlock)
			d.receiptInto(rundir, "verify_run",
				fmt.Sprintf("not run — the %s suite lock was held by a live runner for the whole %ds budget", vproj, vwait/1000),
				"hw done waited for "+vlock+" and did not get it; "+holder)
			a.warn("NOT VERIFIED: the " + vproj + " suite lock is held by a live runner — " + doneHolderLine(vlock))
			a.warn("hw done will NOT run this task's verification over somebody else's suite. Nothing here says the work was checked.")
			a.warn("when that runner is done: hw suite " + vproj + " -- <the command in " + pin + ">")
			a.info("measured: hw receipt " + vproj + " " + vtask)
			return
		}
		held = true
		lock := vlock
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		stop := make(chan struct{})
		go func() {
			select {
			case s := <-sigs:
				os.RemoveAll(lock)
				if s == syscall.SIGINT {
					os.Exit(130)
				}
				os.Exit(143)
			case <-stop:
			}
		}()
		defer func() {
			if held { // the EXIT trap: a die in the loop below still releases the lock
				os.RemoveAll(lock)
			}
			signal.Stop(sigs)
			close(stop)
		}()
		extraEnv["HW_SUITE_LOCK_HELD"] = vproj + ":" + token
		a.ok("holding the " + vproj + " suite lock for this close's verification (pid " + strconv.Itoa(os.Getpid()) + ")")
	}
	gate := &doneGate{d: d}
	if readableFile(d.binDir + "/suite-gate") { // bin/hw sources it when it is there; absent, nothing gates
		gate.take("hw done verification of "+vtaskName, extraEnv)
	}
	data, _ := os.ReadFile(pin)
	n := 0
	for _, cmd := range doneLinesOf(string(data)) {
		if cmd == "" {
			continue
		}
		n++
		log := rundir + "/verify-" + strconv.Itoa(n) + ".log"
		if doneNamesFullSuite(cmd) {
			a.warn("verify command names the full suite without HW_TEST_GATE=fast — that is ~1000s against a " + secs + "s timeout, and it will read as a failed verification, not a slow one. Full-suite coverage is already mandatory at push (setup/hooks/pre-push); point this brief's verify at the fast gate instead: HW_TEST_GATE=fast bash setup/test-hw")
			a.warn("this was also said at dispatch, where the brief could still be edited for free")
		}
		a.info("verifying: " + cmd)
		out, rc := d.runPinned(verifydir, to, cmd, extraEnv)
		os.WriteFile(log, []byte(out+"\n"), 0o666)
		sum := sha256.Sum256([]byte(out))
		d.receiptInto(rundir, "verify_run",
			"exit "+strconv.Itoa(rc)+"  sha256 "+hex.EncodeToString(sum[:])+"  "+cmd,
			"hw done ran: (cd "+verifydir+" && "+toS+" bash -c '"+cmd+"'); output in "+log)
		if rc == 0 { // # MUTATION-ANCHOR: 861-G29
			a.ok("VERIFIED  exit 0  " + cmd)
		} else {
			a.warn("VERIFICATION FAILED  exit " + strconv.Itoa(rc) + "  " + cmd)
			a.warn("the receipt records the failure; nothing here claims this task was verified")
			a.warn("the output is in " + log)
		}
	}
	gate.release()
	if held {
		os.RemoveAll(vlock)
		held = false
		a.ok("released the " + vproj + " suite lock")
	}
	a.info("measured: hw receipt " + vproj + " " + vtask)
}

// runPinned is `out="$( cd "$verifydir" && $to bash -c "$cmd" 2>&1 )"`, with what hw exported for it.
func (d *doneRun) runPinned(dir string, to []string, cmd string, extra map[string]string) (string, int) {
	argv := append(append([]string{}, to...), "bash", "-c", cmd)
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	c.Env = doneChildEnv(extra)
	c.Stdin = os.Stdin
	var buf strings.Builder
	c.Stdout, c.Stderr = &buf, &buf
	rc := doneExit(c.Run())
	return bashSub(buf.String()), rc
}

// doneChildEnv is the environment bash hands a command: its own, without what only the shim exported for
// hw-core, with the variables hw done set on top.
func doneChildEnv(extra map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if k == "HW_CORE_BIN_DIR" || k == "HW_CORE_ROOT" {
			continue
		}
		if _, over := extra[k]; over {
			continue
		}
		env = append(env, kv)
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+extra[k])
	}
	return env
}

// ── the lane's suite lock ───────────────────────────────────────────────────

// doneProcStart is `ps -o lstart= -p <pid> | tr -s ' '`.
func doneProcStart(pid string) string {
	out, _ := doneTool("", nil, "ps", "-o", "lstart=", "-p", pid)
	var sb strings.Builder
	prevSpace := false
	for _, c := range out {
		if c == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		sb.WriteRune(c)
	}
	return bashSub(sb.String())
}

// doneHolderAlive is _suite_holder_alive: the recorded holder is alive and is the same process.
func doneHolderAlive(lock string) bool {
	owner, _ := doneReadTrim(lock + "/owner.pid")
	if !doneAllDigits(owner) {
		// an absent owner is a claim in progress until the lock is a minute old:
		// `find "$lock" -maxdepth 0 -mmin +1` — minutes since the change, rounded up, more than one
		st, err := os.Stat(lock)
		if err != nil {
			return true
		}
		secs := time.Since(st.ModTime()).Seconds()
		mins := int64(secs / 60)
		if float64(mins)*60 < secs {
			mins++
		}
		return !(mins > 1)
	}
	pid, _ := strconv.Atoi(owner)
	if p, err := os.FindProcess(pid); err != nil || p.Signal(syscall.Signal(0)) != nil {
		return false
	}
	start, _ := doneReadTrim(lock + "/owner.start")
	if start == "" {
		return true
	}
	return doneProcStart(owner) == start
}

// doneHolderLine is _suite_holder_line.
func doneHolderLine(lock string) string {
	owner, _ := doneReadTrim(lock + "/owner.pid")
	at, _ := doneReadTrim(lock + "/owner.at")
	task, _ := doneReadTrim(lock + "/owner.task")
	cmd, _ := doneReadTrim(lock + "/owner.cmd")
	age := ""
	if doneAllDigits(at) {
		age = strconv.FormatInt(doneNow()-doneAtoi(at), 10) + "s"
	}
	if owner == "" {
		owner = "<none recorded>"
	}
	s := "pid " + owner
	if task != "" {
		s += ", task " + task
	}
	if age != "" {
		s += ", holding for " + age
	}
	if cmd != "" {
		s += ", running: " + cmd
	}
	return s
}

// doneLockHeldByAncestor is _suite_lock_held_by_ancestor: the lane, the token on disk and a live holder agree.
func doneLockHeldByAncestor(lock, lane, marker string) bool {
	if marker == "" {
		return false
	}
	ml := marker
	if i := strings.IndexByte(marker, ':'); i >= 0 {
		ml = marker[:i]
	}
	if ml != lane {
		return false
	}
	tok, _ := doneReadTrim(lock + "/owner.token")
	if tok == "" {
		return false
	}
	mt := marker
	if i := strings.IndexByte(marker, ':'); i >= 0 {
		mt = marker[i+1:]
	}
	if tok != mt {
		return false
	}
	return doneHolderAlive(lock)
}

// suiteLockTake is _suite_lock_take: the token when this process holds the lock, false when the budget ran out
// behind a live holder. It never dies.
func (d *doneRun) suiteLockTake(lock, lane string, waitMS int64, what string) (string, bool) {
	deadline := doneNow() + waitMS/1000
	announced := false
	pid := strconv.Itoa(os.Getpid())
	for {
		if os.Mkdir(lock, 0o777) == nil {
			os.WriteFile(lock+"/owner.pid", []byte(pid+"\n"), 0o666)
			os.WriteFile(lock+"/owner.start", []byte(doneProcStart(pid)+"\n"), 0o666)
			os.WriteFile(lock+"/owner.at", []byte(strconv.FormatInt(doneNow(), 10)+"\n"), 0o666)
			task := d.getenv("HW_TASK")
			if task == "" {
				task = "<not an hw task>"
			}
			os.WriteFile(lock+"/owner.task", []byte(task+"\n"), 0o666)
			os.WriteFile(lock+"/owner.cmd", []byte(what+"\n"), 0o666)
			token := fmt.Sprintf("%s-%d-%d-%d", pid, doneNow(), rand.Intn(32768), rand.Intn(32768))
			os.WriteFile(lock+"/owner.token", []byte(token), 0o666)
			return token, true
		}
		if !doneHolderAlive(lock) {
			d.warn("reclaiming an ORPHANED " + lane + " suite lock — " + doneHolderLine(lock) + " is not running any more")
			os.RemoveAll(lock)
			continue
		}
		if !announced {
			d.info(fmt.Sprintf("waiting for the %s suite lock — %s. Budget %ds. Nothing is being killed; this waits.", lane, doneHolderLine(lock), waitMS/1000))
			announced = true
		}
		if doneNow() >= deadline {
			return "", false
		}
		time.Sleep(5 * time.Second)
	}
}

// ── the machine-wide gate (bin/suite-gate) ──────────────────────────────────

type doneGate struct {
	d      *doneRun
	held   bool
	slot   string
	ticket string
	extra  map[string]string
}

func (g *doneGate) ncpu() int {
	n := g.d.getenv("HW_SUITE_NCPU")
	if n == "" {
		out, _ := doneTool("", nil, "sysctl", "-n", "hw.ncpu")
		n = bashSub(out)
	}
	if n == "" {
		out, _ := doneTool("", nil, "getconf", "_NPROCESSORS_ONLN")
		n = bashSub(out)
	}
	if !doneAllDigits(n) || n == "0" {
		return 4
	}
	v, _ := strconv.Atoi(n)
	return v
}

func (g *doneGate) slots() int {
	s := g.d.getenv("HW_SUITE_SLOTS")
	if s != "" && (!doneAllDigits(s) || doneAtoi(s) == 0) {
		fmt.Fprintf(g.d.err, "suite-gate: HW_SUITE_SLOTS must be a positive integer (got: %s) — using the default\n", s)
		s = ""
	}
	if s == "" {
		n := g.ncpu() / 4
		if n < 1 {
			n = 1
		}
		return n
	}
	v, _ := strconv.Atoi(s)
	return v
}

func doneFirstLineOf(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return t4FirstLine(string(b))
}

// sgAlive is _sg_alive / _sg_ticket_alive: pid and start time, an unrecorded owner young for a minute.
func doneSgAlive(pid, start, path string) bool {
	if pid == "" || start == "" {
		st, err := os.Stat(path)
		var m int64
		if err == nil {
			m = st.ModTime().Unix()
		}
		return doneNow()-m < 60
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	if p, err := os.FindProcess(n); err != nil || p.Signal(syscall.Signal(0)) != nil {
		return false
	}
	return doneProcStart(pid) == start
}

// take is sg_take: one slot of the machine-wide gate, or none past the ceiling. Never a failure.
func (g *doneGate) take(what string, extra map[string]string) {
	d := g.d
	g.extra = extra
	work := d.c.work
	if work == "" {
		fmt.Fprint(d.err, "suite-gate: no WORK — this run takes no suite slot and is not coordinated with other suites\n")
		return
	}
	dir := work + "/.locks/suite-gate"
	if os.MkdirAll(dir, 0o777) != nil {
		fmt.Fprintf(d.err, "suite-gate: could not create %s — this run takes no suite slot\n", dir)
		return
	}
	if m := d.getenv("HW_SUITE_GATE_HELD"); m != "" {
		slot, tok := m, m
		if i := strings.IndexByte(m, ':'); i >= 0 {
			slot, tok = m[:i], m[i+1:]
		}
		sd := dir + "/" + slot
		if doneFirstLineOf(sd+"/owner.token") == tok && doneSgAlive(doneFirstLineOf(sd+"/owner.pid"), doneFirstLineOf(sd+"/owner.start"), sd) {
			return
		}
	}
	prio := d.getenv("HW_SUITE_PRIORITY")
	if prio == "" {
		prio = "normal"
	}
	pcode := "1"
	if prio == "cut" {
		pcode = "0"
	}
	n := g.slots()
	waitMS := d.getenv("HW_SUITE_GATE_WAIT_MS")
	if waitMS == "" {
		waitMS = "7200000"
	}
	deadline := doneNow() + doneBashArith(waitMS)/1000
	pid := strconv.Itoa(os.Getpid())
	g.ticket = fmt.Sprintf("%s/q.%s.%d.%07d", dir, pcode, doneNow(), os.Getpid())
	start := doneProcStart(pid)
	body := pid + "\n" + start
	if start != "" {
		body += "\n"
	}
	body += d.getenv("HW_TASK") + "\n" + what + "\n"
	if os.WriteFile(g.ticket+".part", []byte(body), 0o666) == nil {
		os.Rename(g.ticket+".part", g.ticket)
	}
	announced := false
	for {
		tickets, _ := os_glob(dir, "q.")
		head := ""
		for _, t := range tickets {
			if strings.HasSuffix(t, ".part") {
				continue
			}
			lines := strings.Split(string(doneMustRead(t)), "\n")
			tp, ts := "", ""
			if len(lines) > 0 {
				tp = lines[0]
			}
			if len(lines) > 1 {
				ts = lines[1]
			}
			if doneSgAlive(tp, ts, t) {
				head = t
				break
			}
			os.Remove(t)
		}
		running, who := 0, ""
		slots, _ := os_glob(dir, "slot.")
		for _, k := range slots {
			if doneSgAlive(doneFirstLineOf(k+"/owner.pid"), doneFirstLineOf(k+"/owner.start"), k) {
				running++
				w := doneSgWho(k)
				if who != "" {
					who += "; "
				}
				who += w
			} else if head == g.ticket {
				fmt.Fprintf(d.err, "suite-gate: reclaiming the slot of a dead holder — %s is not running any more\n", doneSgWho(k))
				os.RemoveAll(k)
			}
		}
		if head == g.ticket && running < n {
			for k := 0; k < n; k++ {
				s := dir + "/slot." + strconv.Itoa(k)
				if os.Mkdir(s, 0o777) == nil {
					g.slot = s
					os.WriteFile(s+"/owner.pid", []byte(pid+"\n"), 0o666)
					st := doneProcStart(pid)
					if st != "" {
						st += "\n"
					}
					os.WriteFile(s+"/owner.start", []byte(st), 0o666)
					os.WriteFile(s+"/owner.task", []byte(d.getenv("HW_TASK")+"\n"), 0o666)
					os.WriteFile(s+"/owner.cmd", []byte(what+"\n"), 0o666)
					os.WriteFile(s+"/owner.at", []byte(strconv.FormatInt(doneNow(), 10)+"\n"), 0o666)
					mine := fmt.Sprintf("%s-%d-%d-%d", pid, doneNow(), rand.Intn(32768), rand.Intn(32768))
					os.WriteFile(s+"/owner.token", []byte(mine), 0o666)
					g.held = true
					os.Remove(g.ticket)
					g.ticket = ""
					extra["HW_SUITE_GATE_HELD"] = "slot." + strconv.Itoa(k) + ":" + mine
					return
				}
			}
		}
		if !announced {
			if running > 0 {
				fmt.Fprintf(d.err, "waiting for a suite slot (%d running: %s)\n", running, who)
			} else {
				fmt.Fprint(d.err, "waiting for a suite slot (0 running: a run ahead of this one in the queue is taking the next slot)\n")
			}
			announced = true
		}
		if doneNow() >= deadline {
			os.Remove(g.ticket)
			g.ticket = ""
			fmt.Fprintf(d.err, "suite-gate: waited past the %ss ceiling (HW_SUITE_GATE_WAIT_MS) — running WITHOUT a slot rather than failing\n", strconv.FormatInt(doneBashArith(waitMS)/1000, 10))
			return
		}
		poll := d.getenv("HW_SUITE_POLL_S")
		if poll == "" {
			poll = "2"
		}
		f, err := strconv.ParseFloat(poll, 64)
		if err != nil {
			f = 2
		}
		time.Sleep(time.Duration(f * float64(time.Second)))
	}
}

// release is sg_release.
func (g *doneGate) release() {
	if g.held && g.slot != "" {
		if doneFirstLineOf(g.slot+"/owner.pid") == strconv.Itoa(os.Getpid()) {
			os.RemoveAll(g.slot)
		}
		g.held, g.slot = false, ""
		delete(g.extra, "HW_SUITE_GATE_HELD")
	}
	if g.ticket != "" {
		os.Remove(g.ticket)
		g.ticket = ""
	}
}

func doneSgWho(k string) string {
	t := doneFirstLineOf(k + "/owner.task")
	p := doneFirstLineOf(k + "/owner.pid")
	if p == "" && !doneExists(k+"/owner.pid") {
		p = "?"
	}
	c := doneFirstLineOf(k + "/owner.cmd")
	if c == "" && !doneExists(k+"/owner.cmd") {
		c = "?"
	}
	s := "pid " + p
	if t != "" {
		s += ", task " + t
	}
	return s + ": " + c
}

func doneMustRead(p string) []byte { b, _ := os.ReadFile(p); return b }

// os_glob is `"$dir"/<prefix>*`, in the order the shell expands it.
func os_glob(dir, prefix string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, dir+"/"+e.Name())
		}
	}
	return sortLocaleDone(out), nil
}

// sortLocaleDone is the shell's glob order: `sort` in the caller's locale.
func sortLocaleDone(items []string) []string {
	if len(items) < 2 {
		return items
	}
	cmd := exec.Command("sort", "-z")
	var in strings.Builder
	for _, s := range items {
		in.WriteString(s)
		in.WriteByte(0)
	}
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.Output()
	if err != nil {
		sort.Strings(items)
		return items
	}
	var res []string
	for _, s := range strings.Split(string(out), "\x00") {
		if s != "" {
			res = append(res, s)
		}
	}
	return res
}

// ── the brain-leak scan ─────────────────────────────────────────────────────

// brainLeakAtClose is _brain_leak_at_close: what a product task can carry out, scanned for a brain path.
func (d *doneRun) brainLeakAtClose(proj, wt, branch string) {
	a := d.app
	if d.x.get(proj, "brain_guard") == "0" {
		return
	}
	if !isDir(wt) {
		return
	}
	if _, rc := doneGit(wt, "rev-parse", "--git-dir"); rc != 0 {
		return
	}
	gd := d.root + "/setup/guards"
	if !(isFile(gd+"/brain-guard/hooks/hooks.json") && isFile(gd+"/brain-guard.opencode.json") && isFile(gd+"/deny-brain-writes.js") && isFile(gd+"/deny_brain_writes.py")) {
		a.warn("brain leak check skipped: no brain guard under " + gd)
		return
	}
	c := exec.Command("python3", gd+"/deny_brain_writes.py", "--scan", wt, branch)
	c.Env = doneChildEnv(map[string]string{"HW_BRAIN_GUARD_ROOT": d.c.brain, "PROJ": proj})
	var buf strings.Builder
	c.Stdout, c.Stderr = &buf, &buf
	rc := doneExit(c.Run())
	out := bashSub(buf.String())
	if rc == 0 {
		return
	}
	if a.getenv("HW_DONE_ALLOW_BRAIN_LEAK") == "1" {
		a.warn("brain leak check: " + out + " — closing anyway (HW_DONE_ALLOW_BRAIN_LEAK=1)")
		return
	}
	if rc == 1 {
		a.die("brain leak: " + wt + " carries a commit no remote has that names brain (" + out + "). Rewrite it, or close with HW_DONE_ALLOW_BRAIN_LEAK=1 hw done " + proj + " …")
	}
	a.die("brain leak check could not finish on " + wt + " (" + out + "); nothing was closed. HW_DONE_ALLOW_BRAIN_LEAK=1 closes anyway")
}
