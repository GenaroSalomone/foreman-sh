package main

// Train T5, area C (reuse): launch-time discovery of a live executor to re-task, and the route a dispatch takes
// when it finds one. bin/hw `_next_run_dir`, `_run_done_marker`, `_run_env_value`, `_chaining_lease_*`,
// `_check_reusable_executor`, `_reuse_*`, `_lease_seconds`, the reuse block that follows `_dispatch_manifest`, and
// the route forecast (`_route_forecast_path/_write/_drop`; `_route_forecast_check` is the launch's).

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const rtLeaseMaxH = 24 // _LEASE_MAX_H // # MUTATION-ANCHOR: 844-37-M04

// leaseSeconds is `_lease_seconds`: 90m | 2h | 4 -> seconds. Empty -> 4h.
func (d *dsp) leaseSeconds(arg string) string {
	v := arg // # MUTATION-ANCHOR: 844-37-M01
	if v == "" {
		v = "4h" // # MUTATION-ANCHOR: 844-R01
	}
	var n string
	var u int64
	switch {
	case strings.HasSuffix(v, "h"):
		n, u = strings.TrimSuffix(v, "h"), 3600 // # MUTATION-ANCHOR: 844-R02
	case strings.HasSuffix(v, "m"):
		n, u = strings.TrimSuffix(v, "m"), 60 // # MUTATION-ANCHOR: 844-R03
	default:
		n, u = v, 3600
	}
	if n == "" || strings.Trim(n, "0123456789") != "" {
		d.die("--keep-preview takes a duration like 90m, 4h or a bare number of hours (got: " + arg + ")") // # MUTATION-ANCHOR: 844-R05
	}
	nv, err := strconv.ParseInt(n, 10, 64)
	if err != nil || nv <= 0 { // # MUTATION-ANCHOR: 844-R06
		d.die("--keep-preview needs a duration greater than zero") // # MUTATION-ANCHOR: 844-R07
	}
	// $(( n * u )): a leading zero reads as octal, and a digit that is not one ends hw
	if len(n) > 1 && n[0] == '0' { // # MUTATION-ANCHOR: 844-R08
		o, err := strconv.ParseInt(n, 8, 64)
		if err != nil {
			panic(exitNow{1})
		}
		nv = o // # MUTATION-ANCHOR: 844-R09
	}
	secs := nv * u
	if secs > int64(rtLeaseMaxH)*3600 { // # MUTATION-ANCHOR: 844-R10
		d.die("a preview lease may not exceed " + strconv.Itoa(rtLeaseMaxH) + "h (asked for " + arg + "). A preview nobody has looked at in a day is not being kept, it is being forgotten.") // # MUTATION-ANCHOR: 844-R11
	}
	return strconv.FormatInt(secs, 10)
}

// ── the run directory ───────────────────────────────────────────────────────

// nextRunDir is `_next_run_dir`: the newest run directory of a live executor, found from the cwd herdr reports
// for its pane, walking up. "" when there is none.
func (d *dsp) nextRunDir(cwd string) string {
	dir := cwd
	for hops := 0; hops < 12 && dir != "" && dir != "/"; hops++ {
		if isDir(dir + "/.hw") {
			if n := rtNewestRun(dir + "/.hw"); n != "" {
				return n
			}
		}
		dir = rtBashDirname(dir)
	}
	return ""
}

// rtNewestRun is `fd -H -t d -d 1 '^2[0-9]{7}-[0-9]{6}-[0-9]+$' <hw> | sort -r | head -1`, minus a trailing slash.
func rtNewestRun(hw string) string {
	ents, err := os.ReadDir(hw)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() && RunIDPattern.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // # MUTATION-ANCHOR: 844-R13
	return hw + "/" + names[0]
}

// rtDigits is `tr -dc '0-9'`.
func rtDigits(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// runDoneMarker is `_run_done_marker`: the done marker of a run's CURRENT task.
func (d *dsp) runDoneMarker(rundir string) string {
	seq := ""
	if text, ok := rtReadable(rundir + "/task"); ok {
		seq = rtDigits(text)
	}
	switch seq {
	case "", "0", "1":
		return rundir + "/done"
	}
	return rundir + "/t" + seq + "/done" // # MUTATION-ANCHOR: 844-R15
}

// rtStrerror is Python's e.strerror: the C library's text, capitalised.
func rtStrerror(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var en syscall.Errno
	if errors.As(err, &en) {
		s := en.Error()
		r, n := utf8.DecodeRuneInString(s)
		return string(unicode.ToUpper(r)) + s[n:]
	}
	return err.Error()
}

// rtUTF8Error is the text of Python's UnicodeDecodeError for the first invalid sequence of data.
func rtUTF8Error(data []byte) string {
	for i := 0; i < len(data); {
		b := data[i]
		if b < 0x80 {
			i++
			continue
		}
		need, lo, hi := 0, byte(0x80), byte(0xbf)
		switch {
		case b >= 0xc2 && b <= 0xdf:
			need = 1
		case b >= 0xe0 && b <= 0xef:
			need = 2
			if b == 0xe0 {
				lo = 0xa0
			}
			if b == 0xed {
				hi = 0x9f
			}
		case b >= 0xf0 && b <= 0xf4:
			need = 3
			if b == 0xf0 {
				lo = 0x90
			}
			if b == 0xf4 {
				hi = 0x8f
			}
		default:
			return fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: invalid start byte", b, i)
		}
		j := 1
		for ; j <= need; j++ {
			if i+j >= len(data) {
				return rtUTF8Where(i, j, data[i], "unexpected end of data")
			}
			c := data[i+j]
			l, h := byte(0x80), byte(0xbf)
			if j == 1 {
				l, h = lo, hi
			}
			if c < l || c > h {
				return rtUTF8Where(i, j, data[i], "invalid continuation byte")
			}
		}
		i += need + 1
	}
	return ""
}

func rtUTF8Where(pos, n int, first byte, why string) string {
	if n == 1 {
		return fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: %s", first, pos, why)
	}
	return fmt.Sprintf("'utf-8' codec can't decode bytes in position %d-%d: %s", pos, pos+n-1, why)
}

// rtRunEnv is `bin/runenv --bare get <run-dir|env-file> KEY`: the value, whether it answered (exit 0), and
// what it said on stderr (exit 2).
func rtRunEnv(rundir, key string) (string, bool, string) {
	// `_run_env_value` hands runenv "<rundir>/env"; runenv reads a directory's own `env` file.
	p := rundir + "/env"
	if isDir(p) {
		p += "/env"
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, ""
		}
		return "", false, "runenv: cannot read " + p + ": " + rtStrerror(err) + "\n"
	}
	kvs, perr := ParseEnv(data, true, true)
	if perr != nil {
		return "", false, "runenv: " + p + " is not UTF-8: " + rtUTF8Error(data) + "\n"
	}
	for _, kv := range kvs {
		if kv.Key == key {
			return kv.Value, true, ""
		}
	}
	return "", false, ""
}

// runEnvValue is `_run_env_value`: one value of a run's env file, false when the file or key is absent or the
// file is damaged. runenv's complaint about a damaged file reaches stderr, as in bash.
func (d *dsp) runEnvValue(rundir, key string) (string, bool) {
	v, ok, msg := rtRunEnv(rundir, key)
	if msg != "" {
		fmt.Fprint(d.err, msg)
	}
	return v, ok
}

// runEnvValueQuiet is `_run_env_value … 2>/dev/null`.
func (d *dsp) runEnvValueQuiet(rundir, key string) (string, bool) {
	v, ok, _ := rtRunEnv(rundir, key)
	return v, ok
}

// ── the chaining lease ──────────────────────────────────────────────────────

// chainingLeaseField is `_chaining_lease_field`: sed -n "s/^<key>=//p" chaining-lease | tail -1.
func (d *dsp) chainingLeaseField(rundir, key string) string {
	text, ok := rtReadable(rundir + "/chaining-lease")
	if !ok {
		return ""
	}
	last := ""
	for _, line := range rtRecords(text) {
		if strings.HasPrefix(line, key+"=") {
			last = line[len(key)+1:] // # MUTATION-ANCHOR: 844-R16
		}
	}
	return last
}

// chainingLeaseLive is `_chaining_lease_live`: state=live and until in the future.
func (d *dsp) chainingLeaseLive(rundir string) bool {
	if d.chainingLeaseField(rundir, "state") != "live" { // # MUTATION-ANCHOR: 844-R17
		return false
	}
	until := d.chainingLeaseField(rundir, "until")
	if until == "" || strings.Trim(until, "0123456789") != "" {
		return false
	}
	u, err := strconv.ParseInt(until, 10, 64)
	return err == nil && time.Now().Unix() < u // # MUTATION-ANCHOR: 844-R19
}

// ── discovery ───────────────────────────────────────────────────────────────

// rtCand is one `<pane>\t<task>` row of the candidates a reuse check reads.
type rtCand struct{ pane, task string }

// rtTsvEscape is jq's @tsv for a string.
func rtTsvEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r").Replace(s)
}

// rtReadTab is `IFS=$'\t' read -r pane task`: tab is IFS whitespace, so leading tabs go, runs coalesce, and the
// last variable keeps the rest of the line without its trailing tabs.
func rtReadTab(line string) (string, string) {
	s := strings.TrimLeft(line, "\t")
	i := strings.IndexByte(s, '\t')
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimRight(strings.TrimLeft(s[i:], "\t"), "\t")
}

// rtCandidates is the jq program of `_check_reusable_executor` (and of the `dependencies` line) over
// `herdr agent list`, then the `read` loop: the agents whose hw_project token is proj. jqOK is false when jq
// would have exited non-zero (a syntax error, or a value it cannot index/iterate/@tsv).
func rtCandidates(agents, proj string) (cands []rtCand, jqOK bool) {
	vals, perr := parseStream([]byte(agents))
	jqOK = perr == nil
	var lines []string
	for _, doc := range vals {
		rows, ok := rtCandRows(doc, proj)
		lines = append(lines, rows...)
		if !ok {
			jqOK = false
		}
	}
	text := bashSub(strings.Join(lines, "\n"))
	if text == "" {
		return nil, jqOK
	}
	for _, line := range strings.Split(text, "\n") {
		pane, task := rtReadTab(line)
		cands = append(cands, rtCand{pane, task})
	}
	return cands, jqOK
}

// rtCandRows is one input of that program: its @tsv lines, up to the first error.
func rtCandRows(doc *jval, proj string) ([]string, bool) {
	res, ok := doc.get("result")
	if !ok {
		return nil, false
	}
	ag, ok := res.get("agents")
	if !ok {
		return nil, false
	}
	if !ag.truthy() {
		return nil, true
	}
	if ag.kind != vArr && ag.kind != vObj {
		return nil, false // Cannot iterate over a scalar
	}
	var rows []string
	for _, a := range rtIter(ag) {
		tok, ok := a.get("tokens")
		if !ok {
			return rows, false
		}
		hp, ok := tok.get("hw_project")
		if !ok {
			return rows, false
		}
		if !hp.truthy() {
			hp = jstr("")
		}
		if hp.kind != vStr || hp.s != proj { // # MUTATION-ANCHOR: 844-R20
			continue
		}
		var fields []string
		for _, k := range [][2]string{{"pane_id", ""}, {"hw_task", "tokens"}} { // # MUTATION-ANCHOR: 844-R21
			src := a
			if k[1] != "" {
				src = tok
			}
			v, ok := src.get(k[0])
			if !ok {
				return rows, false
			}
			if !v.truthy() {
				continue
			}
			switch v.kind {
			case vStr:
				fields = append(fields, rtTsvEscape(v.s))
			case vNum:
				fields = append(fields, canonNumber(v.s))
			case vBool:
				fields = append(fields, "true")
			default:
				return rows, false // @tsv of an array or object
			}
		}
		rows = append(rows, strings.Join(fields, "\t"))
	}
	return rows, true
}

const rtDiscardTail = "): no live chaining lease, so nobody declared it re-taskable and it may have a human in it. --keep-pane <duration> --why \"<why you are coming back>\" at ITS launch is what makes a pane reusable."

// checkReusableExecutor is `_check_reusable_executor`: establishes the reuse candidate, if any, and names every
// candidate it looked at and refused.
func (d *dsp) checkReusableExecutor(proj string) {
	d.reuseDiscarded = ""
	if !d.checkReuse { // # MUTATION-ANCHOR: 844-R24
		d.reuseLine = "fresh session chosen by " + orDefaultStr(d.freshSource, "--fresh") + "; reuse deliberately skipped" // # MUTATION-ANCHOR: 844-R25
		return
	}
	agents, ok := herdrOut(0, "agent", "list")
	if !ok {
		return
	}
	agents = bashSub(agents)
	if agents == "" {
		return
	}
	cands, _ := rtCandidates(agents, proj)
	for _, c := range cands {
		pane, candidateTask := c.pane, c.task
		if pane == "" {
			continue
		}
		info, _ := herdrOut(0, "agent", "get", pane)
		info = bashSub(info)
		if info == "" {
			continue
		}
		cwd := jqRawOr(info, "", "result", "agent", "cwd")
		if cwd == "" {
			continue
		}
		rundir := d.nextRunDir(cwd)
		if rundir == "" {
			continue
		}
		marker := d.runDoneMarker(rundir)
		if !isFile(marker) { // # MUTATION-ANCHOR: 844-R26
			continue
		}
		mode, _ := d.runEnvValue(rundir, "HW_LAUNCH_MODE") // # MUTATION-ANCHOR: 844-R27
		why, leaseReason := "", ""
		if d.chainingLeaseLive(rundir) {
			why = "reported, chaining lease live"
			leaseReason = d.chainingLeaseField(rundir, "reason") // # MUTATION-ANCHOR: 844-R28
		} else {
			if d.reuseDiscarded != "" {
				d.reuseDiscarded += "\n"
			}
			d.reuseDiscarded += "discarded pane " + pane + " (prior task " + orDefaultStr(candidateTask, "unknown") + ", reported, launch mode " + orDefaultStr(mode, "unrecorded") + rtDiscardTail // # MUTATION-ANCHOR: 844-R29
			continue                                                                                                                                                                                // # MUTATION-ANCHOR: 844-71-M01
		}
		d.reusePane = pane
		d.reuseRundir = rundir
		d.reuseWhy = why                                                                                                    // # MUTATION-ANCHOR: 844-R30
		d.reuseLine = "pane " + pane + " selected (prior task " + orDefaultStr(candidateTask, "unknown") + "; " + why + ")" // # MUTATION-ANCHOR: 844-123-M02
		prior := candidateTask
		if prior == "" {
			prior = jqRawOr(info, "", "result", "agent", "tokens", "hw_task")
		}
		if prior == "" {
			prior, _ = d.runEnvValue(rundir, "HW_TASK") // # MUTATION-ANCHOR: 844-R32
		}
		d.reuseTask = prior
		d.reuseLeaseReason = leaseReason // # MUTATION-ANCHOR: 844-R33
		return
	}
}

// reuseCheckField is `_reuse_check_field`: the verdict, then every named discard under it.
func (d *dsp) reuseCheckField() string {
	s := d.reuseLine
	if d.reuseDiscarded == "" {
		return s
	}
	for _, line := range strings.Split(d.reuseDiscarded, "\n") {
		if line != "" {
			s += "\n              " + line // # MUTATION-ANCHOR: 844-R34
		}
	}
	return s
}

// ── what a reuse dispatch says about the pane it lands on ───────────────────

// reuseFlagDecision is `_reuse_flag_decision`: match, unmeasurable or mismatch.
func (d *dsp) reuseFlagDecision(requested, measured string) {
	if measured == "" {
		d.reuseFlagVerdict = "unmeasurable" // # MUTATION-ANCHOR: 844-136-M07
		return
	}
	if measured == requested { // # MUTATION-ANCHOR: 844-R35
		d.reuseFlagVerdict = "match"
		return
	}
	d.reuseFlagVerdict = "mismatch"
}

// reuseMeasuredDesc is `_reuse_measured_desc`.
func (d *dsp) reuseMeasuredDesc(raw, flag string) string {
	switch raw {
	case "":
		return "could not be measured"
	case "<agent default>": // # MUTATION-ANCHOR: 844-R36
		return "its agent's own default (that launch recorded no explicit " + flag + ")"
	}
	return raw
}

// reuseDispatchAbsenceReason is `_reuse_dispatch_absence_reason`: what is true about the dispatch file.
func (d *dsp) reuseDispatchAbsenceReason(key string) string {
	base := path.Base(d.reuseRundir)
	if _, err := os.Stat(d.reuseRundir + "/dispatch"); err != nil {
		return "no dispatch record exists at " + base + "/dispatch" // # MUTATION-ANCHOR: 844-R37
	}
	if f, err := os.Open(d.reuseRundir + "/dispatch"); err != nil {
		return base + "/dispatch could not be read"
	} else {
		f.Close()
	}
	return base + "/dispatch has no " + key + "= line" // # MUTATION-ANCHOR: 844-R38
}

// reuseModelFieldLine is `_reuse_model_field_line`.
func (d *dsp) reuseModelFieldLine() string {
	if d.reuseModel == "" {
		return "model=n/a  (--model cannot change a live session; the REUSED session's own model could not be measured — " + d.reuseDispatchAbsenceReason("model") + ")" // # MUTATION-ANCHOR: 844-R40
	}
	return "model=n/a  (--model cannot change a live session; the REUSED session's own is " + d.reuseMeasuredDesc(d.reuseModel, "--model") + ", read from " + path.Base(d.reuseRundir) + "/dispatch)" // # MUTATION-ANCHOR: 844-R39
}

// reuseEffortFieldLine is `_reuse_effort_field_line`.
func (d *dsp) reuseEffortFieldLine() string {
	if d.reuseEffort == "" {
		return "effort=n/a  (--effort cannot change a live session; the REUSED session's own effort could not be measured — " + d.reuseDispatchAbsenceReason("effort") + ")" // # MUTATION-ANCHOR: 844-R42
	}
	return "effort=n/a  (--effort cannot change a live session; the REUSED session's own is " + d.reuseMeasuredDesc(d.reuseEffort, "--effort") + ", read from " + path.Base(d.reuseRundir) + "/dispatch)" // # MUTATION-ANCHOR: 844-R41
}

// reuseAgentFieldLine is `_reuse_agent_field_line`.
func (d *dsp) reuseAgentFieldLine() string {
	if d.reuseVendor == "" {
		return "the REUSED session's own vendor could not be measured — herdr reports no vendor for " + d.reusePane + " — --agent cannot change a live session" // # MUTATION-ANCHOR: 844-R43
	}
	return "the REUSED session's own: " + d.reuseVendor + "  (measured live via herdr) — --agent cannot change a live session" // # MUTATION-ANCHOR: 844-R44
}

// reuseMeasureAccount is `_reuse_measure_account`: the account the live session runs under, from its own env.
func (d *dsp) reuseMeasureAccount() {
	d.reuseAccountCfg = ""
	d.reuseAccountMeasured = false
	if !rtIsReadable(d.reuseRundir + "/env") { // # MUTATION-ANCHOR: 844-136-M11
		return
	}
	d.reuseAccountMeasured = true
	d.reuseAccountCfg, _ = d.runEnvValue(d.reuseRundir, "CLAUDE_CONFIG_DIR") // # MUTATION-ANCHOR: 844-R45
}

// rtIsReadable is `[ -r path ]`.
func rtIsReadable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// reuseAccountAbsenceReason is `_reuse_account_absence_reason`.
func (d *dsp) reuseAccountAbsenceReason() string {
	base := path.Base(d.reuseRundir)
	if _, err := os.Stat(d.reuseRundir + "/env"); err != nil {
		return "no run environment record exists at " + base + "/env" // # MUTATION-ANCHOR: 844-R46
	}
	return base + "/env could not be read"
}

// reuseAccountDesc is `_reuse_account_desc`.
func (d *dsp) reuseAccountDesc(cfg string) string {
	if cfg == "" {
		return "the default account (its run environment records no CLAUDE_CONFIG_DIR)" // # MUTATION-ANCHOR: 844-R47
	}
	return "CLAUDE_CONFIG_DIR=" + cfg // # MUTATION-ANCHOR: 844-R48
}

// reuseAccountFieldLine is `_reuse_account_field_line`.
func (d *dsp) reuseAccountFieldLine() string {
	base := path.Base(d.reuseRundir)
	switch {
	case !d.reuseAccountMeasured:
		return "the REUSED session's own account could not be measured — " + d.reuseAccountAbsenceReason() + " — --account cannot change a live session" // # MUTATION-ANCHOR: 844-136-M12
	case d.reuseAccountCfg != "": // # MUTATION-ANCHOR: 844-R49
		return "the REUSED session's own: CLAUDE_CONFIG_DIR=" + d.reuseAccountCfg + " (read from " + base + "/env) — --account cannot change a live session"
	}
	return "the REUSED session's own: no CLAUDE_CONFIG_DIR in " + base + "/env, so it runs under the default account — --account cannot change a live session" // # MUTATION-ANCHOR: 844-R50
}

// reuseSubagentModelFieldLine is `_reuse_subagent_model_field_line`: the live session's own subagent model, read from
// its run env, and "could not be measured" when nobody opened that env (never asserted as inherit).
func (d *dsp) reuseSubagentModelFieldLine() string {
	if !d.reuseAccountMeasured { // # MUTATION-ANCHOR: 844-232-M11
		return "the REUSED session's own subagent model could not be measured — " + d.reuseAccountAbsenceReason() + " — --subagent-model cannot change a live session"
	}
	base := path.Base(d.reuseRundir)
	if v, _ := d.runEnvValue(d.reuseRundir, "CLAUDE_CODE_SUBAGENT_MODEL"); v != "" { // # MUTATION-ANCHOR: 844-232-M12
		return "the REUSED session's own: CLAUDE_CODE_SUBAGENT_MODEL=" + v + " (read from " + base + "/env) — --subagent-model cannot change a live session"
	}
	return "the REUSED session's own: no CLAUDE_CODE_SUBAGENT_MODEL in " + base + "/env, so its subagents inherit the executor's model — --subagent-model cannot change a live session"
}

// ── the reuse route, after the manifest ─────────────────────────────────────

// reuseRoute is the block of bin/hw that follows `_dispatch_manifest` on the reuse route: the session flags a
// live pane cannot take are refused, then the brief goes to `hw next` (a child process here) and the dry run
// ends with its success line.
func (d *dsp) reuseRoute() {
	refuse := func(flag, value, tail string) {
		d.die(flag + " " + value + " REFUSED on the REUSE route: " + tail)
	}
	codexHandoff := false
	if d.modelSrc == "chosen" || d.agentSrc == "chosen" || d.effort != "" { // # MUTATION-ANCHOR: 844-R51
		var surface string
		if d.c.t.get(d.proj, "product_repo") == "1" {
			surface = d.c.checkout(d.proj) // _surface_dir
		} else {
			surface = path.Dir(path.Dir(d.reuseRundir)) // # MUTATION-ANCHOR: 844-R52
		}
		d.resolveEffectiveSddMode(d.sdd, surface, d.runDispatchFramework(d.reuseRundir)) // # MUTATION-ANCHOR: 844-R53
		// # MUTATION-ANCHOR: 844-136-M04
		if d.agentSrc == "chosen" {
			switch d.agent + ":" + d.sddEffectiveMode {
			case "codex:speckit": // # MUTATION-ANCHOR: 844-136-M03
				codexHandoff = true
			}
		} // # MUTATION-ANCHOR-END: 844-136-M04

	}
	if d.modelSrc == "chosen" && !codexHandoff { // # MUTATION-ANCHOR: 844-136-M01
		d.reuseFlagDecision(d.model, d.reuseModel)
		switch d.reuseFlagVerdict {
		case "unmeasurable":
			refuse("--model", d.model, "pane "+d.reusePane+"'s current model could not be measured ("+d.reuseDispatchAbsenceReason("model")+"), and --model cannot reach a live session regardless. Nothing was sent. Launch a fresh executor with that model instead: add --fresh to this command.") // # MUTATION-ANCHOR: 844-R54
		case "mismatch":
			refuse("--model", d.model, "pane "+d.reusePane+" is already running with "+d.reuseMeasuredDesc(d.reuseModel, "--model")+", fixed at ITS launch — --model cannot reach a live session. Nothing was sent. Launch a fresh executor with that model instead: add --fresh to this command.") // # MUTATION-ANCHOR: 844-R55
		}
	}
	if d.agentSrc == "chosen" && !codexHandoff { // # MUTATION-ANCHOR: 844-136-M05
		d.reuseFlagDecision(d.agent, d.reuseVendor)
		switch d.reuseFlagVerdict {
		case "unmeasurable":
			refuse("--agent", d.agent, "pane "+d.reusePane+"'s current vendor could not be measured (herdr reports no vendor for "+d.reusePane+"), and --agent cannot reach a live session regardless. Nothing was sent. Launch a fresh executor with that vendor instead: add --fresh to this command.") // # MUTATION-ANCHOR: 844-R56
		case "mismatch":
			refuse("--agent", d.agent, "pane "+d.reusePane+" is already running "+d.reuseMeasuredDesc(d.reuseVendor, "--agent")+", fixed at ITS launch — --agent selects which vendor process to start, and there is no live process left to select. Nothing was sent. Launch a fresh executor with that vendor instead: add --fresh to this command.") // # MUTATION-ANCHOR: 844-R57
		}
	}
	if d.effort != "" && !codexHandoff { // # MUTATION-ANCHOR: 844-136-M06
		d.reuseFlagDecision(d.effort, d.reuseEffort)
		switch d.reuseFlagVerdict {
		case "unmeasurable":
			refuse("--effort", d.effort, "pane "+d.reusePane+"'s current effort could not be measured ("+d.reuseDispatchAbsenceReason("effort")+"), and --effort cannot reach a live session regardless. Nothing was sent. Launch a fresh executor with that effort instead: add --fresh to this command.")
		case "mismatch":
			refuse("--effort", d.effort, "pane "+d.reusePane+" is already running with "+d.reuseMeasuredDesc(d.reuseEffort, "--effort")+", fixed at ITS launch — --effort is baked into the vendor process at spawn the same way --model is. Nothing was sent. Launch a fresh executor with that effort instead: add --fresh to this command.") // # MUTATION-ANCHOR: 844-R58
		}
	}
	if d.accountSrc == "chosen" { // # MUTATION-ANCHOR: 844-136-M09 # MUTATION-ANCHOR: 844-136-M10 # MUTATION-ANCHOR: 844-136-M13
		vendorClause := ""
		switch d.reuseVendor {
		case "", "claude": // # MUTATION-ANCHOR: 844-R59
		default:
			vendorClause = " AND SEPARATELY, BY VENDOR: that pane is running " + d.reuseVendor + ", which does not read CLAUDE_CONFIG_DIR at all, so no account would reach it even on a fresh launch under that vendor — a fresh launch under this account needs --agent claude as well." // # MUTATION-ANCHOR: 844-R60
		}
		want, known := d.accountConfigDirOf(d.account)
		if !known { // # MUTATION-ANCHOR: 844-R61
			refuse("--account", d.account, "'"+d.account+"' is not an account hw knows (known: default, personal), so there is nothing to compare against pane "+d.reusePane+". Nothing was sent. Name a known account, or drop --account to hand this task to the pane as it already is.")
		}
		if !d.reuseAccountMeasured { // # MUTATION-ANCHOR: 844-R62
			refuse("--account", d.account, "pane "+d.reusePane+"'s current account could not be measured ("+d.reuseAccountAbsenceReason()+"), and --account cannot reach a live session regardless."+vendorClause+" Nothing was sent. Drop --account to hand this task to the pane as it already is, or launch a fresh executor under that account: add --fresh to this command.")
		} else if want != d.reuseAccountCfg { // # MUTATION-ANCHOR: 844-R63
			refuse("--account", d.account, "pane "+d.reusePane+" is already running under "+d.reuseAccountDesc(d.reuseAccountCfg)+", baked into its environment at ITS launch — Claude Code reads CLAUDE_CONFIG_DIR once at startup, so --account cannot reach a live session."+vendorClause+" Nothing was sent. Drop --account to hand this task to the pane as it already is, or launch a fresh executor under that account: add --fresh to this command.")
		}
	}

	args := []string{d.reusePane}
	if d.brief != "" { // # MUTATION-ANCHOR: 844-R64
		args = append(args, "--brief", d.brief)
	} else {
		args = append(args, d.task)
	}
	if d.sdd != "" { // # MUTATION-ANCHOR: 844-R65
		args = append(args, "--sdd", d.sdd)
	}
	if d.allowStaleHooks {
		args = append(args, "--allow-stale-hooks")
	}
	if d.dispatchDry {
		args = append(args, "--dry-run")
	}
	d.warn("REUSE WARNING: automatic launch-time reuse selected pane " + d.reusePane + "; its prior task is " + orDefaultStr(d.reuseTask, "unknown") + ".") // # MUTATION-ANCHOR: 844-71-M04
	d.warn("That pane's prior context and worktree carry into this task. Use --fresh to launch without them.")
	if d.reuseWhy == "reported, chaining lease live" { // # MUTATION-ANCHOR: 844-R66
		d.warn("Its persisted chaining lease reason: " + orDefaultStr(d.reuseLeaseReason, "<not recorded>"))
	}
	if d.dispatchDry {
		if rf := d.routeForecastWrite(d.proj, d.task, "reuse", d.reuseLine); rf != "" { // # MUTATION-ANCHOR: 844-R68
			d.info("route FROZEN as REUSE of " + d.reusePane + " for the next " + d.routeForecastTTL + " s (" + rf + "). If that lease expires or the pane goes before you launch, the launch REFUSES and names the change instead of quietly building a fresh executor.") // # MUTATION-ANCHOR: 844-R69
		}
	}
	d.rtCmdNext(args)
	seq := rtDigits(d.rtReuseTaskText())
	if seq == "" || seq == "0" { // # MUTATION-ANCHOR: 844-R70
		seq = "1"
	}
	n := rtArith(seq)
	if d.dispatchDry {
		n++ // # MUTATION-ANCHOR: 844-R71
		d.ok("would reuse pane " + d.reusePane + " as task " + strconv.FormatInt(n, 10) + "; no new executor would be built, and this session keeps its prior " + d.proj + " lane context (" + d.reuseWhy + ")")
	} else {
		d.ok("reused pane " + d.reusePane + " as task " + strconv.FormatInt(n, 10) + "; no new executor was built, and this session keeps its prior " + d.proj + " lane context (" + d.reuseWhy + ")")
	}
}

// rtBashLine is the line of <HW_CORE_BIN_DIR>/hw that bash names in a "hw: line N:" complaint, found by the text
// that anchors it rather than remembered: a number written here goes stale with the first edit of bin/hw above it
// (the shim already moved one by two), and the goldens normalize it away, so only test 846 would notice. 0 when
// the anchor is not there; 846 fails then.
func (d *dsp) rtBashLine(anchor string, exact bool) int {
	data, err := os.ReadFile(d.env("HW_CORE_BIN_DIR") + "/hw")
	if err != nil {
		return 0
	}
	for i, l := range strings.Split(string(data), "\n") {
		if (exact && l == anchor) || (!exact && strings.Contains(l, anchor)) {
			return i + 1
		}
	}
	return 0
}

// The shell, not tr, opens the reused run's task counter (`tr -dc '0-9' < "$_REUSE_RUNDIR/task" 2>/dev/null`) and
// reports a failure before the 2>/dev/null takes hold: that is the line of the complaint.
const rtReuseSeqAnchor = `tr -dc '0-9' < "$_REUSE_RUNDIR/task"`

// rtReuseTaskText is the text of <rundir>/task for that read; when the shell cannot open it, bash's own
// complaint ("<script>: line N: <file>: <strerror>") reaches stderr and the text is empty.
func (d *dsp) rtReuseTaskText() string {
	p := d.reuseRundir + "/task"
	f, err := os.Open(p)
	if err != nil {
		fmt.Fprintf(d.err, "%s/hw: line %d: %s: %s\n", d.env("HW_CORE_BIN_DIR"), d.rtBashLine(rtReuseSeqAnchor, false), p, rtStrerror(err)) // # MUTATION-ANCHOR: 844-R72
		return ""
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return string(data)
}

// rtArith is `$(( s ))` for a string of digits: a leading zero reads as octal, a digit that is not one ends hw.
func rtArith(s string) int64 {
	base := 10
	if len(s) > 1 && s[0] == '0' {
		base = 8
	}
	n, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		panic(exitNow{1})
	}
	return n
}

// rtCmdNext is `cmd_next <args>` run as `<HW_CORE_BIN_DIR>/hw next <args>`: output passed through, a non-zero
// status ends this run with it (set -e).
func (d *dsp) rtCmdNext(args []string) {
	if d.readonly && !d.dispatchDry { // shadow's first leg changes nothing: `hw next` re-tasks a live pane, and bash is about to do exactly that
		return
	}
	cmd := exec.Command(d.env("HW_CORE_BIN_DIR")+"/hw", append([]string{"next"}, args...)...)
	cmd.Stdout, cmd.Stderr = d.out, d.err
	cmd.Env = d.rtChildEnv()          // # MUTATION-ANCHOR: 844-R74
	if err := cmd.Run(); err != nil { // # MUTATION-ANCHOR: 844-R73
		panic(exitNow{exitStatus(err)})
	}
}

// ── the route forecast ──────────────────────────────────────────────────────

// routeForecastPath is `_route_forecast_path`: ${TMPDIR:-/tmp}/hw-route-forecast-<project>__<task>, the name
// reduced to [A-Za-z0-9_.-].
func (d *dsp) routeForecastPath(proj, task string) string {
	key := strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			return r
		}
		return '_'
	}, proj+"__"+task)
	return orDefaultStr(d.env("TMPDIR"), "/tmp") + "/hw-route-forecast-" + key // # MUTATION-ANCHOR: 844-R75
}

// The forecast file is opened by the redirection (`{ … } > "$f" 2>/dev/null`) of `_route_forecast_write`; the shell,
// not the group, reports a failure to open it, before the 2>/dev/null takes hold, and bash names the line the
// function starts on.
const rtForecastAnchor = "_route_forecast_write() {  # $1=project $2=task $3=route $4=detail"

// rtForecastRefusal is that complaint: bash's text for why `> "$f"` failed under noclobber (set -C).
func (d *dsp) rtForecastRefusal(f string, err error) string {
	why := rtStrerror(err)
	if errors.Is(err, fs.ErrExist) {
		why = "cannot overwrite existing file"
		if st, serr := os.Lstat(f); serr == nil && st.IsDir() {
			why = "Is a directory" // # MUTATION-ANCHOR: 844-R76
		}
	}
	return fmt.Sprintf("%s/hw: line %d: %s: %s\n", d.env("HW_CORE_BIN_DIR"), d.rtBashLine(rtForecastAnchor, true), f, why)
}

// routeForecastWrite is `_route_forecast_write`: record the route a dry run printed, refusing to follow a
// symlink (the old file is removed and the new one created exclusively, mode 0600). It returns the path, or ""
// after a warning when the write was refused. Under HW_CORE_READONLY it writes nothing and says the same.
func (d *dsp) routeForecastWrite(proj, task, route, detail string) string {
	f := d.routeForecastPath(proj, task)
	refuse := func(err error) string {
		fmt.Fprint(d.err, d.rtForecastRefusal(f, err))
		d.warn("could not record the route forecast at " + f + "; this dry run does NOT bind the next launch") // # MUTATION-ANCHOR: 844-R78
		return ""
	}
	if d.readonly {
		if st, err := os.Stat(path.Dir(f)); err != nil || !st.IsDir() {
			return refuse(&fs.PathError{Op: "open", Path: f, Err: syscall.ENOENT})
		}
		if st, err := os.Lstat(f); err == nil && st.IsDir() {
			return refuse(&fs.PathError{Op: "open", Path: f, Err: syscall.EEXIST})
		}
		return f
	}
	if st, err := os.Lstat(f); err == nil && !st.IsDir() { // # MUTATION-ANCHOR: 844-R79
		os.Remove(f)
	}
	fh, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return refuse(err)
	}
	_, err = fmt.Fprintf(fh, "route=%s\nat=%d\ndetail=%s\n", route, time.Now().Unix(), strings.ReplaceAll(detail, "\n", " "))
	if cerr := fh.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return refuse(err)
	}
	return f
}

// routeForecastDrop is `_route_forecast_drop`: rm -f of the forecast.
func (d *dsp) routeForecastDrop(proj, task string) {
	if d.readonly {
		return
	}
	f := d.routeForecastPath(proj, task)
	if st, err := os.Lstat(f); err == nil && !st.IsDir() { // # MUTATION-ANCHOR: 844-R82
		os.Remove(f)
	}
}
