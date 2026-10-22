package main

// hw ruling (bin/hw cmd_ruling) and the ruling queue it writes
// (the bash state witness, "THE RULING QUEUE"): a correction queued into the run
// directory of an executor that is still working, delivered at its next turn
// end. The files are the contract — `pending-ruling.<n>`, `ruling-delivered.<n>`,
// `transcript.log` — and executor-turn-end and done-invoker read them.
//
// NOT PORTED HERE: resuming a task that reported --blocked (`_ruling_resume_blocked`).
// That path sends through channel-send and the invoker library, which stay bash;
// the verb hands it to bash whole (see resumeInBash).

import (
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const t3RulingQueueCap = 5

// ── the facts of one pane: witness_executor_state ────────────────────────────

type execState struct {
	verdict, status, rundir, seq, cwd string
}

// t3WitnessRunDir is witness_run_dir: the run directory at or above cwd, the pane's own run first.
func t3WitnessRunDir(cwd, run string) string {
	d := cwd
	for hops := 0; hops < 12 && d != "" && d != "/"; hops++ {
		hw := d + "/.hw"
		if isDir(hw) {
			if run != "" && isDir(hw+"/"+run) {
				return hw + "/" + run
			}
			if ids := RunIDs(hw); len(ids) > 0 {
				return hw + "/" + ids[0]
			}
		}
		d = t3Dirname(d)
	}
	return ""
}

// t3Dirname is dirname(1).
func t3Dirname(p string) string {
	p = strings.TrimRight(p, "/")
	i := strings.LastIndexByte(p, '/')
	switch {
	case i < 0:
		return "."
	case i == 0:
		return "/"
	}
	return strings.TrimRight(p[:i], "/")
}

// t3ReportingLive is witness_reporting_live: the marker names a process, and it is that exact one.
func t3ReportingLive(marker string) bool {
	b, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	lines := strings.Split(string(b), "\n")
	pidS := lines[0]
	start := ""
	if len(lines) > 1 {
		start = lines[1]
	}
	if pidS == "" || strings.Trim(pidS, "0123456789") != "" {
		return false
	}
	pid, err := strconv.Atoi(pidS)
	if err != nil || !t3PidAlive(pid) {
		return false
	}
	out, _ := exec.Command("ps", "-o", "lstart=", "-p", pidS).Output()
	now := strings.Trim(t3Squeeze(strings.TrimRight(string(out), "\n")), " ")
	return now == start // # MUTATION-ANCHOR: 841-R02
}

// t3Squeeze is `tr -s ' '`.
func t3Squeeze(s string) string {
	var sb strings.Builder
	prev := false
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			if prev {
				continue
			}
			prev = true
		} else {
			prev = false
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func (t *t3) executorState(pane string) execState {
	info, _ := herdrClient{getenv: t.getenv}.stdout("agent", "get", pane)
	status := jqRawOr(info, "", "result", "agent", "agent_status")
	if info == "" || status == "" {
		return execState{verdict: "absent"}
	}
	state := jqRawOr(info, "", "result", "agent", "tokens", "turn_state")
	children := jqRawOr(info, "", "result", "agent", "tokens", "children_running")
	cwd := jqRawOr(info, "", "result", "agent", "cwd")
	run := jqRawOr(info, "", "result", "agent", "tokens", "hw_run")
	rundir, taskdir, seq := "", "", "1"
	if cwd != "" {
		rundir = t3WitnessRunDir(cwd, run)
	}
	if rundir != "" {
		taskdir = mkTaskDir(rundir)
		if strings.HasPrefix(taskdir, rundir+"/t") {
			seq = taskdir[len(rundir)+2:]
		}
	}
	var verdict string
	switch {
	case taskdir != "" && t3ReportingLive(taskdir+"/reporting"):
		verdict = "reporting"
	case taskdir != "" && isFile(taskdir+"/done") && !isFile(taskdir+"/reopened"):
		verdict = "reported"
	case state == "ended_awaiting_child" && children != "":
		verdict = "awaiting_child"
	case status == "idle" || status == "done":
		verdict = "idle"
	default:
		verdict = "working"
	}
	return execState{verdict, status, rundir, seq, cwd}
}

// t3RulingRefusal is hw-actions' ruling_refusal: the die() text for a verdict, "" when it would queue.
func t3RulingRefusal(verdict, status, seq, pane string) string {
	switch verdict {
	case "absent":
		return "herdr knows no pane " + pane + ", so there is nothing to queue a ruling for. Nothing was written."
	case "reported":
		return pane + " has already reported task " + seq + ", so there is no turn end left to deliver into. Nothing was written. A correction for its NEXT task belongs in the brief you hand it with hw next."
	case "reporting":
		return pane + " is reporting task " + seq + " right now — its done-invoker is running and has already checked for rulings. Nothing was written. A correction for its NEXT task belongs in the brief you hand it with hw next."
	case "idle":
		return pane + " is idle (herdr agent_status=" + status + ", no child running): channel-send --ruling delivers to it NOW and its gate will not refuse. Nothing was queued — queueing would wait for a turn end that has not started."
	case "working", "awaiting_child":
		return ""
	}
	return "internal: unknown executor state '" + verdict + "' for " + pane + ". Nothing was written."
}

// t3RulingScopeRefusal is ruling_scope_refusal.
func t3RulingScopeRefusal(pane, cwd, rundir string) string {
	if cwd == "" {
		return "herdr reports no cwd for " + pane + ", so hw cannot find its run directory. Nothing was written."
	}
	if rundir == "" {
		return "no hw run directory at or above the cwd herdr reports for " + pane + " — that pane was not launched by hw, so it has no turn-end hook to deliver a ruling from. Nothing was written."
	}
	return ""
}

// ── the queue ────────────────────────────────────────────────────────────────

// t3QueuePending is ruling_queue_pending: the undelivered rulings, oldest first.
func t3QueuePending(d string) []string {
	var out []string
	if isFile(d + "/pending-ruling") {
		out = append(out, d+"/pending-ruling")
	}
	ents, _ := os.ReadDir(d)
	type nf struct {
		n    int64
		path string
	}
	var num []nf
	for _, e := range ents {
		name := e.Name()
		if !strings.HasPrefix(name, "pending-ruling.") {
			continue
		}
		suffix := name[len("pending-ruling."):]
		if suffix == "" || strings.Trim(suffix, "0123456789") != "" {
			continue
		}
		if !isFile(d + "/" + name) {
			continue
		}
		v, _ := strconv.ParseInt(suffix, 10, 64)
		num = append(num, nf{v, d + "/" + name})
	}
	// `sort -n -k1,1`: by number, a tie by the whole line (n<TAB>path)
	sort.Slice(num, func(i, j int) bool {
		if num[i].n != num[j].n { // # MUTATION-ANCHOR: 841-G12
			return num[i].n < num[j].n
		}
		return num[i].path < num[j].path
	})
	for _, x := range num {
		out = append(out, x.path)
	}
	return out
}

// t3QueueNextSeq is ruling_queue_next_seq: one past the highest number any queue file still names.
func t3QueueNextSeq(d string) int64 {
	ents, _ := os.ReadDir(d)
	var max int64
	for _, e := range ents {
		name := e.Name()
		var n string
		switch {
		case strings.HasPrefix(name, "pending-ruling."), strings.HasPrefix(name, "ruling-delivered."):
			n = name[strings.LastIndexByte(name, '.')+1:]
		case strings.HasPrefix(name, ".pending-ruling.claimed."):
			n = name[len(".pending-ruling.claimed."):] // # MUTATION-ANCHOR: 841-G11
			if i := strings.IndexByte(n, '.'); i >= 0 {
				n = n[:i]
			}
		default:
			continue
		}
		if n == "" || strings.Trim(n, "0123456789") != "" {
			continue
		}
		v, err := strconv.ParseInt(n, 10, 64)
		if err == nil && v > max {
			max = v
		}
	}
	return max + 1
}

// transcriptAppend is transcript_append: best effort, always.
func (t *t3) transcriptAppend(d, kind, envelope, text string) {
	if d == "" {
		return
	}
	base := d[strings.LastIndexByte(d, '/')+1:]
	if len(base) > 1 && base[0] == 't' && strings.Trim(base[1:], "0123456789") == "" {
		d = d[:strings.LastIndexByte(d, '/')]
	}
	if !isDir(d) {
		return
	}
	lines := strings.Split(text+"\n", "\n")
	lines = lines[:len(lines)-1]
	for i := range lines {
		lines[i] = "    | " + lines[i]
	}
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if envelope == "" {
		envelope = "-"
	}
	rec := t3IsoNow() + // # MUTATION-ANCHOR: 841-X01
		"  " + kind + // # MUTATION-ANCHOR: 841-X02
		"  envelope=" + envelope + // # MUTATION-ANCHOR: 841-X03
		"\n" + body + "\n" // # MUTATION-ANCHOR: 841-X04
	t.fs.appendTo(d+"/transcript.log", []byte(rec), 0o666)
}

// cockpitKick is `cockpit-state --kick --rundir <d>`, never failing, printing nothing.
func (t *t3) cockpitKick(args ...string) {
	if t.fs.readonly {
		return
	}
	bin := t.binDir() + "/cockpit-state"
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return
	}
	cmd := exec.Command(bin, append([]string{"--kick"}, args...)...)
	cmd.Run()
}

// queueAdd is ruling_queue_add for text: the new ruling's path and place, or a
// refusal (cap) / failure code. Mirrors the bash: the text is written to a name
// beside the queue, then linked to the first free number (`ln` never clobbers), so
// a reader never sees a half-written ruling.
func (t *t3) queueAdd(d, text string) (path string, pos int, code int) {
	t.fs.watch(d)
	n := t3QueueNextSeq(d)
	if t.fs.readonly {
		t.fs.rec(fsOp{Op: "write", Path: d + "/pending-ruling." + strconv.FormatInt(n, 10), Data: t3B64([]byte(text + "\n")), Mode: 0o666})
	} else {
		tmp := d + "/.pending-ruling." + strconv.Itoa(os.Getpid())
		if err := os.WriteFile(tmp, []byte(text+"\n"), 0o666); err != nil { // # MUTATION-ANCHOR: 841-R01
			t.die("could not write the ruling beside " + d)
		}
		tries := 0
		for {
			if err := os.Link(tmp, d+"/pending-ruling."+strconv.FormatInt(n, 10)); err == nil {
				break
			}
			n++
			tries++
			if tries >= 50 {
				os.Remove(tmp)
				return "", 0, 1
			}
		}
		os.Remove(tmp)
	}
	ahead := 0
	for _, f := range t3QueuePending(d) {
		m := f[len(d+"/pending-ruling"):]
		m = strings.TrimPrefix(m, ".")
		if m == "" {
			ahead++
			continue
		}
		if v, err := strconv.ParseInt(m, 10, 64); err == nil && v < n {
			ahead++
		}
	}
	// # MUTATION-ANCHOR: 841-G10
	if ahead >= t3RulingQueueCap && t.queueRetract(d, n) {
		return "", 0, 3
	}
	t.cockpitKick("--rundir", d)
	return d + "/pending-ruling." + strconv.FormatInt(n, 10), ahead + 1, 0
}

func (t *t3) queueRetract(d string, n int64) bool {
	if t.fs.readonly {
		t.fs.rec(fsOp{Op: "remove", Path: d + "/pending-ruling." + strconv.FormatInt(n, 10)})
		return true
	}
	ns := strconv.FormatInt(n, 10)
	tmp := d + "/.pending-ruling.retracted." + ns + "." + strconv.Itoa(os.Getpid())
	if os.Rename(d+"/pending-ruling."+ns, tmp) != nil {
		return false
	}
	os.Remove(tmp)
	return true
}

// queueClaimAll is ruling_queue_claim_all: every pending ruling, oldest first, each claimed with a
// rename; the delivery text, and whether there was one.
func (t *t3) queueClaimAll(d, via string) (string, bool) {
	t.fs.watch(d)
	at := t3IsoNow()
	count := 0
	first, body := "", ""
	for _, f := range t3QueuePending(d) {
		n := f[len(d+"/pending-ruling"):]
		n = strings.TrimPrefix(n, ".")
		if n == "" {
			n = "legacy"
		}
		claim := d + "/.pending-ruling.claimed." + n + "." + strconv.Itoa(os.Getpid())
		if t.fs.rename(f, claim) != nil {
			continue
		}
		text := ""
		if t.fs.readonly {
			text = mkReadSub(f)
		} else {
			text = mkReadSub(claim)
		}
		if text != "" {
			count++
			if n != "legacy" {
				t.fs.write(d+"/ruling-delivered."+n, t3Lines(
					"at="+at,   // # MUTATION-ANCHOR: 841-D01
					"via="+via, // # MUTATION-ANCHOR: 841-D02
				), 0o666)
			}
			t.transcriptAppend(d, "RULING-DELIVERED", "ruling:"+n+" via="+via, text)
			if count == 1 {
				first = text
			}
			body += "── ruling " + strconv.Itoa(count) + " ──\n" + text + "\n\n"
		}
		t.fs.remove(claim) // # MUTATION-ANCHOR: 841-G08
	}
	if count == 0 {
		return "", false
	}
	t.cockpitKick("--rundir", d)
	t.fs.write(d+"/ruling-delivered", t3Lines(
		"at="+at,                     // # MUTATION-ANCHOR: 841-D03
		"via="+via,                   // # MUTATION-ANCHOR: 841-D04
		"count="+strconv.Itoa(count), // # MUTATION-ANCHOR: 841-D05
	), 0o666)
	if count == 1 {
		return first, true
	}
	return strconv.Itoa(count) + " RULINGS, in the order your brainer sent them. Where a later one disagrees with an earlier one, the later one stands.\n\n" + strings.TrimSuffix(body, "\n\n"), true
}

// ── the verb ─────────────────────────────────────────────────────────────────

const t3RulingUsage = "usage: hw ruling <executor-pane> \"<the correction>\" [--dry-run] | hw ruling <executor-pane> - [--dry-run] (a lone '-' reads the correction from stdin — the way for text with backticks, $, quotes or newlines that argv would mangle; MEASURED 2026-09-11: a ruling with backticked identifiers reached its receiver with every one of them missing, because the SENDER's own shell expanded them before hw ever ran). Queues a correction, delivered at its next turn end in the order sent (up to 5 undelivered per executor). For a receiver that is already idle or done, use channel-send --ruling, which delivers now."

func (t *t3) ruling() int {
	args := t.args
	pane := ""
	if len(args) > 0 {
		pane, args = args[0], args[1:]
	}
	if pane == "" {
		t.die(t3RulingUsage)
	}
	text, dry, stdinDash, sawWord := "", false, false, false
loop:
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--dry-run":
			dry = true
			args = args[1:]
		case a == "--":
			args = args[1:]
			if len(args) > 0 {
				sawWord = true
				if text != "" {
					text += " " + strings.Join(args, " ")
				} else {
					text = strings.Join(args, " ")
				}
			}
			break loop
		case a == "-":
			stdinDash = true
			args = args[1:]
		case strings.HasPrefix(a, "-"):
			t.die("unknown option: " + a)
		default:
			sawWord = true
			if text != "" {
				text += " " + a
			} else {
				text = a
			}
			args = args[1:]
		}
	}
	if stdinDash {
		if sawWord {
			t.die("the ruling was given on argv AND as '-' for stdin — pass it ONE way, not both")
		}
		if f, ok := t.stdin.(*os.File); ok && isTerminal(f) {
			t.die("'-' reads the ruling from stdin, but stdin is a terminal — pipe or redirect the text in")
		}
		data, err := io.ReadAll(t.stdin)
		if err != nil {
			t.die("could not read the ruling from stdin")
		}
		text = bashSub(string(data))
		if text == "" {
			t.die("stdin for '-' produced no bytes — an empty ruling delivered would be worse than this refusal")
		}
	}
	if text == "" {
		t.die("the ruling is empty — there is nothing to deliver")
	}

	st := t.executorState(pane)
	if st.verdict == "absent" {
		t.die(t3RulingRefusal("absent", st.status, st.seq, pane))
	}
	if why := t3RulingScopeRefusal(pane, st.cwd, st.rundir); why != "" {
		t.die(why)
	}
	if st.verdict == "reported" && isFile(mkTaskDir(st.rundir)+"/blocked-waiting") {
		return t.resumeInBash(pane, text, dry)
	}
	if why := t3RulingRefusal(st.verdict, st.status, st.seq, pane); why != "" {
		t.die(why)
	}
	rundir := st.rundir
	if dry {
		waiting := len(t3QueuePending(rundir))
		t.printf("  would queue a ruling for %s (%s, task %s) in %s, behind %d undelivered (cap %d)\n  delivered at its next turn end, or by its done-invoker if it reports first; nothing is queued by this dry run\n",
			pane, st.verdict, st.seq, rundir, waiting, t3RulingQueueCap)
		return 0
	}
	entry, pos, code := t.queueAdd(rundir, text)
	switch code {
	case 0:
	case 3:
		t.die(pane + " already has " + itoa(t3RulingQueueCap) + " undelivered rulings queued in " + rundir + " (pending-ruling.*), the cap. Nothing was written. An executor that has not reached a turn end for that many corrections is stuck or deep in one long turn: read what is queued, and if the brief itself has changed, it belongs in a new brief, not in a sixth correction.")
	default:
		t.die("could not queue the ruling in " + rundir + ". Nothing was written.")
	}
	// WRITE, THEN LOOK (cmd_ruling): done-invoker writes `reporting` and then claims rulings; this
	// writes the ruling and then looks for `reporting`. One of the two sees the other.
	taskdir := mkTaskDir(rundir)
	if t3ReportingLive(taskdir+"/reporting") || (isFile(taskdir+"/done") && !isFile(taskdir+"/reopened")) {
		retracted := rundir + "/.pending-ruling.retracted." + strconv.Itoa(os.Getpid())
		if t.fs.rename(entry, retracted) == nil {
			t.fs.remove(retracted)
			t.die(pane + " began reporting task " + st.seq + " while this ruling was being queued, so it was taken back rather than left where nothing reads it. Nothing is queued. A correction for its NEXT task belongs in the brief you hand it with hw next.")
		}
		t.ok("ruling for " + pane + " was claimed at once by its reporting done-invoker, which shows it to the executor and refuses to report until it is addressed.")
		return 0
	}
	num := entry[strings.LastIndexByte(entry, '.')+1:]
	t.transcriptAppend(rundir, "RULING-QUEUED", "ruling:"+num+" task="+st.seq, text) // # MUTATION-ANCHOR: 841-Q01
	t.ok("ruling queued for " + pane + " (" + st.verdict + ", task " + st.seq + "), number " + itoa(pos) + " in its queue — delivered at its next turn end, or by its done-invoker if it reports first; either way before the task can close, and after every ruling queued ahead of it. It is NOT delivered yet; " + rundir + "/ruling-delivered." + num + " records it once it lands.")
	return 0
}

// resumeInBash hands a ruling for a task that reported --blocked to the bash
// implementation: it resumes the task through channel-send and the invoker
// library, which are not ported. The text travels on argv after `--`, so nothing
// the sender's shell could have mangled is mangled here.
func (t *t3) resumeInBash(pane, text string, dry bool) int {
	if t.fs.readonly { // shadow: bash is about to do exactly this, for real
		t.fs.rec(fsOp{Op: "delegated", Path: "ruling-resume"})
		return 0
	}
	args := []string{"ruling", pane}
	if dry {
		args = append(args, "--dry-run")
	}
	args = append(args, "--", text)
	cmd := exec.Command(t.binDir()+"/hw", args...)
	cmd.Env = append(os.Environ(), "HW_IMPL_RULING=bash")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return exitStatus(err)
	}
	return 0
}
