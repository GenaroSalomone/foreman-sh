package main

// The functions of bin/brain that answer one question each: whether a conversation exists on disk, what is typed in
// claude's input box, the exit confirmation and the background work behind it, the folder trust, the account and the
// permissions, the engram session. Each one names the bash it ports.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"hwcore/internal/engramhealth"
)

var reUUID = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// sessionTranscript is brain_session_transcript: the transcript found for a session id, "" and false when there is
// none. An id that is not a UUID is not judged here ("not-a-uuid", true).
func (b *brainRun) sessionTranscript(sid string) (string, bool) {
	if !reUUID.MatchString(sid) {
		return "not-a-uuid", true
	}
	dirs := []string{b.getenv("CLAUDE_CONFIG_DIR"), b.home + "/.claude"}
	more, _ := filepath.Glob(brEscapeGlob(b.home) + "/.claude-*")
	sort.Strings(more)
	dirs = append(dirs, more...)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		// the leaf is a literal, so it is a stat (case-insensitive on APFS, as bash's `[ -f ]` is), not a listing
		projs, _ := filepath.Glob(brEscapeGlob(d) + "/projects/*")
		sort.Strings(projs)
		for _, p := range projs {
			if f := p + "/" + sid + ".jsonl"; brFileExists(f) {
				return f, true
			}
		}
	}
	return "", false
}

// laneOfCwd is brain_lane_of_cwd: the canonical lane whose directory under the brain root is cwd's, by what the
// directory is and not by how it is spelt.
func (b *brainRun) laneOfCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	real := pyRealpath(cwd)
	rootSt, err := os.Stat(b.brain)
	if err != nil {
		return ""
	}
	parent, err := os.Stat(path.Dir(real))
	if err != nil || !os.SameFile(parent, rootSt) {
		return ""
	}
	cwdSt, err := os.Stat(real)
	if err != nil {
		return ""
	}
	lanes := strings.Fields(strings.Join(b.tab.names, " "))
	sort.Strings(lanes)
	for _, e := range lanes {
		st, err := os.Stat(b.brain + "/" + e)
		if err == nil && st.IsDir() && os.SameFile(st, cwdSt) {
			return e
		}
	}
	return ""
}

var rePsLine = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s+(\S.*)$`)
var reCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
var reCSIFull = regexp.MustCompile(`^\x1b\[([0-9;?]*)([A-Za-z])$`)

// inputDraft is brain_input_draft: what is typed (not dim) in claude's input box. "" with ok means none; !ok is a
// screen that could not be read.
func (b *brainRun) inputDraft(pane string) (string, bool) {
	r := b.herdr(brainCmd{}, "agent", "read", pane, "--source", "visible", "--lines", "30", "--format", "ansi")
	if r.RC != 0 {
		return "", false
	}
	return inputDraftOf(brTrimNL(r.Out)), true
}

func inputDraftOf(screen string) string {
	src := strings.Split(screen, "\n")
	plain := make([]string, len(src))
	for i, l := range src {
		plain[i] = pyStrip(reCSI.ReplaceAllString(l, ""))
	}
	var lines []string
	for i := 1; i < len(src); i++ {
		if strings.Contains(plain[i], "❯") && strings.HasPrefix(plain[i-1], "──") {
			lines = append(lines, src[i])
		}
	}
	if len(lines) == 0 {
		return ""
	}
	last := lines[len(lines)-1]
	dim := false
	var out []string
	pos := 0
	for _, loc := range reCSI.FindAllStringIndex(last, -1) {
		if loc[0] > pos && !dim {
			out = append(out, last[pos:loc[0]])
		}
		pos = loc[1]
		m := reCSIFull.FindStringSubmatch(last[loc[0]:loc[1]])
		if m[2] == "m" {
			prms := strings.Split(m[1], ";")
			for k := 0; k < len(prms); {
				prm := prms[k]
				// # MUTATION-ANCHOR: go-894-M02
				if prm == "38" || prm == "48" || prm == "58" {
					if k+1 < len(prms) && prms[k+1] == "5" {
						k += 3
					} else {
						k += 5
					}
					continue
				}
				// # MUTATION-ANCHOR-END: go-894-M02
				if prm == "" || prm == "0" || prm == "22" {
					dim = false
				} else if prm == "2" {
					dim = true
				}
				k++
			}
		}
	}
	if pos < len(last) && !dim {
		out = append(out, last[pos:])
	}
	s := strings.Join(out, "")
	if i := strings.Index(s, "❯"); i >= 0 {
		s = s[i+len("❯"):]
	}
	return pyStrip(strings.ReplaceAll(s, " ", " "))
}

// exitConfirmation is brain_exit_confirmation: the question claude asks on /exit with background work. Positive
// evidence only: herdr says `blocked` AND the screen carries the prompt.
func (b *brainRun) exitConfirmation(pane, reply string) bool {
	r := b.herdr(brainCmd{}, "agent", "read", pane, "--source", "detection", "--lines", "40")
	screen := brTrimNL(r.Out)
	if !bjTestEq(reply, "blocked", "result", "agent", "agent_status") { // # MUTATION-ANCHOR: go-893-M01
		return false
	}
	switch {
	case strings.Contains(screen, "Background work is running"), strings.Contains(screen, "will stop when you exit"):
		return true
	}
	if i := strings.Index(screen, "Enter to confirm"); i >= 0 && strings.Contains(screen[i+len("Enter to confirm"):], "Esc to cancel") {
		return true
	}
	return false
}

// claudeDescendants is brain_claude_descendants: the pids claude holds as background work, one per entry.
func (b *brainRun) claudeDescendants(pane string) []string {
	r := b.herdr(brainCmd{}, "pane", "process-info", "--pane", pane)
	v, ok := bjParse(brTrimNL(r.Out))
	if !ok {
		return nil
	}
	pid := ""
	procs, _ := bjGet(v, "result", "process_info", "foreground_processes").([]any)
	for _, e := range procs {
		p, isMap := e.(jmap)
		if !isMap {
			continue
		}
		var arg0 any
		if argv, isArr := p["argv"].([]any); isArr && len(argv) > 0 {
			arg0 = argv[0]
		}
		name, _ := bjAlt(arg0, p["argv0"], "").(string)
		parts := strings.Split(name, "/")
		if parts[len(parts)-1] == "claude" {
			if p["pid"] != nil {
				pid = bjRaw(p["pid"])
			}
			break
		}
	}
	if pid == "" {
		return nil
	}
	root, err := strconv.Atoi(pid)
	if err != nil {
		return nil
	}
	ps := b.run(brainCmd{Name: "ps", Args: []string{"-axo", "pid=,ppid=,comm="}})
	kids := map[int][]int{}
	comm := map[int]string{}
	for _, l := range strings.Split(ps.Out, "\n") {
		m := rePsLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		c, e1 := strconv.Atoi(m[1])
		pp, e2 := strconv.Atoi(m[2])
		if e1 != nil || e2 != nil {
			continue
		}
		kids[pp] = append(kids[pp], c)
		comm[c] = strings.TrimLeft(path.Base(strings.TrimSpace(m[3])), "-")
	}
	var out []int
	for _, c := range kids[root] {
		switch comm[c] {
		case "sh", "bash", "zsh", "dash":
			out = append(out, c)
		}
	}
	todo := append([]int(nil), out...)
	for len(todo) > 0 {
		n := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		for _, c := range kids[n] {
			out = append(out, c)
			todo = append(todo, c)
		}
	}
	var s []string
	for _, p := range out {
		s = append(s, strconv.Itoa(p))
	}
	return s
}

// moveToBackground is brain_exit_move_to_background: answer the exit dialog with its second option, never blind.
// false = Esc is the caller's next move (the agent is untouched).
func (b *brainRun) moveToBackground(pane string) bool {
	read := func() string {
		return brTrimNL(b.herdr(brainCmd{}, "agent", "read", pane, "--source", "visible", "--lines", "30").Out)
	}
	if !strings.Contains(read(), "2. Move to background and exit") {
		return false
	}
	if !b.herdrOK("pane", "send-keys", pane, "down") { // # MUTATION-ANCHOR: go-895-M01
		return false
	}
	b.sleep(1 * time.Second)
	// # MUTATION-ANCHOR: go-895-M02
	if !strings.Contains(read(), "❯ 2. Move to background and exit") {
		return false
	}
	// # MUTATION-ANCHOR-END: go-895-M02
	return b.herdrOK("pane", "send-keys", pane, "enter")
}

// trustDir is brain_trust_dir: mark dir trusted in the account's .claude.json, never lowering a decision.
func (b *brainRun) trustDir(dir, acctDir string) {
	cfg := claudeAccountJSON(acctDir, b.home)
	if !brFileExists(cfg) {
		return
	}
	switch b.trustEdit(cfg, dir) {
	case 0:
		b.errf("brain: %s marked trusted in %s (Claude Code would otherwise open on its folder-trust question)\n", dir, cfg)
	case 2:
	default:
		b.errf("brain: could not mark %s trusted in %s — the brainer may open on the folder-trust question\n", dir, cfg)
	}
}

// trustEdit is the python3 heredoc of brain_trust_dir: 0 written, 2 already trusted, 1 anything else.
func (b *brainRun) trustEdit(cfg, dir string) int {
	d := pyRealpath(dir)
	raw, err := os.ReadFile(cfg)
	if err != nil || !utf8.Valid(raw) {
		return 1
	}
	root, ok := ceParse(string(raw))
	if !ok || root.kind != ceObj {
		return 1
	}
	projects := root.member("projects")
	if projects == nil {
		projects = &ceNode{kind: ceObj}
		root.set("projects", projects)
	}
	if projects.kind != ceObj {
		return 1
	}
	p := projects.member(d)
	if p == nil {
		p = &ceNode{kind: ceObj}
		projects.set(d, p)
	}
	if p.kind != ceObj {
		return 1
	}
	if t := p.member("hasTrustDialogAccepted"); t != nil && t.kind == ceBool && t.b {
		return 2
	}
	st, err := os.Stat(cfg)
	if err != nil {
		return 1
	}
	bak := cfg + ".bak-brain-" + b.now().Format("20060102-150405")
	if err := os.WriteFile(bak, raw, st.Mode().Perm()); err != nil {
		return 1
	}
	_ = os.Chtimes(bak, st.ModTime(), st.ModTime()) // shutil.copy2 keeps the times
	p.set("hasTrustDialogAccepted", &ceNode{kind: ceBool, b: true})
	var sb strings.Builder
	root.dump(&sb, 0)
	tmp := cfg + ".brain-tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o666); err != nil {
		return 1
	}
	if err := os.Rename(tmp, cfg); err != nil {
		return 1
	}
	return 0
}

// versionAtLeast is _version_at_least: dotted a >= b, three numeric parts.
func versionAtLeast(a, b string) bool { return depsAtLeast(a, b) } // # MUTATION-ANCHOR: go-803-M06

// freePort is _free_port.
func freePort() string {
	ln, err := listenLoopback()
	if err != nil {
		return ""
	}
	return ln
}

// ── the account, the permissions ────────────────────────────────────────────────────────────────────────────────

// accountEnv is brain_account_env: the --env argument (or nothing), the state on stderr; ok=false when it refuses.
func (b *brainRun) accountEnv(proj, kind string) (string, bool) {
	acct := b.tab.get(proj, "account")
	if acct == "" {
		acct = "default"
	}
	dir := ""
	switch acct {
	case "personal":
		dir = b.home + "/.claude-personal"
	}
	if dir == "" {
		return "", true
	}
	if kind != "claude" {
		b.errf("brain: account %s is this lane's default but does NOT apply to a %s brainer — only Claude Code reads CLAUDE_CONFIG_DIR.\n", acct, kind)
		return "", true
	}
	if !b.accountVerify(acct) {
		b.errf("brain: account '%s' is this lane's default and its capability parity could not be established (see above).\n", acct)
		b.errf("brain: refusing to open a brainer under an unverified account.\n")
		return "", false
	}
	b.errf("brain: account %s → CLAUDE_CONFIG_DIR=%s (lane default for %s, parity verified)\n", acct, dir, proj)
	return "CLAUDE_CONFIG_DIR=" + dir, true
}

// accountVerify is account_verify: 0 verified (or nothing to verify), the diagnosis on stderr.
func (b *brainRun) accountVerify(acct string) bool {
	verifier := ""
	switch acct {
	case "default":
	case "personal":
		verifier = "claude-personal"
	default:
		b.errf("unknown account: %s (known: default, personal)\n", acct)
		return false
	}
	if verifier == "" {
		return true
	}
	if b.a.lookPath(verifier) == "" {
		b.errf("account %s is verified by `%s`, which is not on PATH — so capability parity cannot be established and the account is refused.\n", acct, verifier)
		return false
	}
	return b.run(brainCmd{Name: verifier, Args: []string{"--check"}, AllErr: true}).RC == 0 // # MUTATION-ANCHOR: go-122-M01
}

// permissionsResolve is hw_permissions_resolve "": PERMISSIONS and where it came from; false when the winner is
// neither ask nor skip (PERMISSIONS_ERR says so).
func (b *brainRun) permissionsResolve() bool {
	v, src := "", ""
	cfgHome := b.getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		cfgHome = b.home + "/.config"
	}
	f := cfgHome + "/hw/permissions"
	b.permissionsErr = ""
	switch {
	case b.getenv("HW_PERMISSIONS") != "":
		v, src = b.getenv("HW_PERMISSIONS"), "HW_PERMISSIONS"
	case brFileExists(f):
		data, _ := os.ReadFile(f)
		v = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
				return -1
			}
			return r
		}, string(data))
		src = "machine config " + f
	default:
		v, src = "skip", "default (recommended)"
	}
	if v == "ask" || v == "skip" {
		b.permissions, b.permissionsSrc = v, src
		return true
	}
	b.permissions, b.permissionsSrc = "skip", src
	b.permissionsErr = "permissions must be ask or skip (got: '" + v + "', from " + src + ")"
	return false
}

// firstRunPending is claude_first_run_pending: the once-per-account steps still ahead, one word each.
func (b *brainRun) firstRunPending(cfgDir string) []string {
	jsonPath := claudeAccountJSON(cfgDir, b.home)
	settings := b.home + "/.claude/settings.json"
	if cfgDir != "" {
		settings = cfgDir + "/settings.json"
	}
	g, s := vdLoadDict(jsonPath), vdLoadDict(settings)
	var out []string
	if g["hasCompletedOnboarding"] != true {
		out = append(out, "onboarding")
	}
	if g["bypassPermissionsModeAccepted"] != true && s["skipDangerousModePermissionPrompt"] != true {
		out = append(out, "bypass")
	}
	return out
}

// sessionEnvPrep is claude_session_env_prep: a claude launched here is never a child session.
func (b *brainRun) sessionEnvPrep(pane string) {
	if pane == "" {
		return
	}
	r := b.herdr(brainCmd{}, "agent", "get", pane)
	if v, ok := bjParse(brTrimNL(r.Out)); r.RC == 0 && ok && bjGet(v, "result", "agent") != nil {
		return
	}
	prep := "unset CLAUDE_CODE_CHILD_SESSION"                  // # MUTATION-ANCHOR: go-810-M03
	prep += "; export CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1" // # MUTATION-ANCHOR: go-810-M04
	prep += "; export CLAUDE_CODE_BASH_EDIT_DIFF=0"            // # MUTATION-ANCHOR: go-810-M05
	if !b.herdrOK("pane", "run", pane, prep) {
		b.errf("claude: could not clear CLAUDE_CODE_CHILD_SESSION in %s; Claude may report 'Transcript saving is off'.\n", pane)
	}
}

// paneConfigDir is brain_pane_config_dir: the pane shell's own CLAUDE_CONFIG_DIR, asked through a file it writes.
func (b *brainRun) paneConfigDir(pane string) (string, bool) {
	tmp := b.getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	fh, err := os.CreateTemp(tmp, "brain-cfgdir.")
	if err != nil {
		return "", false
	}
	f := fh.Name()
	fh.Close()
	os.Remove(f)
	cmd := "printf '%s' \"${CLAUDE_CONFIG_DIR:-}\" > '" + f + ".tmp' && mv '" + f + ".tmp' '" + f + "'"
	if !b.herdrOK("pane", "run", pane, cmd) {
		return "", false
	}
	for i := 0; i < 10; i++ {
		if brFileExists(f) {
			data, _ := os.ReadFile(f)
			os.Remove(f)
			return string(data), true
		}
		b.sleep(500 * time.Millisecond)
	}
	os.Remove(f + ".tmp")
	return "", false
}

// ── engram ──────────────────────────────────────────────────────────────────────────────────────────────────────

func (b *brainRun) engramURL() string {
	if u := b.getenv("HW_ENGRAM_URL"); u != "" {
		return u
	}
	port := b.getenv("ENGRAM_PORT")
	if port == "" {
		port = "7437"
	}
	return "http://127.0.0.1:" + port
}

func (b *brainRun) engramStoreDir() string {
	d := b.getenv("ENGRAM_DATA_DIR")
	if strings.TrimSpace(d) != "" {
		return d
	}
	return b.home + "/.engram"
}

func (b *brainRun) httpGet(u string, timeout time.Duration) (string, bool) {
	cl := &http.Client{Timeout: timeout}
	resp, err := cl.Get(u)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return buf.String(), resp.StatusCode < 400
}

// engramServeWait is engram_serve_wait: give a serve that is starting time to answer.
func (b *brainRun) engramServeWait(u string) bool {
	if b.a.lookPath("engram") == "" { // # MUTATION-ANCHOR: go-401-M02
		return true
	}
	max := b.getenv("HW_ENGRAM_WAIT")
	if max == "" || strings.Trim(max, "0123456789") != "" {
		max = "6"
	}
	deadline := b.now().Add(time.Duration(atoiDefault(max, 6)) * time.Second)
	for {
		if _, ok := b.httpGet(u+"/health", 1*time.Second); ok {
			return true
		}
		if !b.now().Before(deadline) {
			return false
		}
		b.sleep(500 * time.Millisecond)
	}
}

// engramServeVerdict is engram_serve_verdict: the line it prints and the status (0 this store, 2 foreign, 3 unknown).
func (b *brainRun) engramServeVerdict(u string) (string, int) {
	dir := b.engramStoreDir()
	want := ""
	if data, err := os.ReadFile(dir + "/.instance-id"); err == nil {
		want = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
				return -1
			}
			return r
		}, string(data))
	}
	probe := engramhealth.Get(u+"/health", engramhealth.Budget(b.getenv("HW_ENGRAM_HEALTH_TIMEOUT")))
	if !probe.OK {
		return engramhealth.Refusal(u, probe), 3
	}
	got := bjRawOr(probe.Body, "instance_id")
	if want == "" {
		return "cannot prove the serve at " + u + " is this store: " + dir + "/.instance-id is missing", 3
	}
	if got == "" {
		return "cannot prove the serve at " + u + " is this store: its /health names no instance_id", 3
	}
	if got != want {
		return "FOREIGN engram serve at " + u + ": instance " + got + ", but this store (" + dir + ") is instance " + want + " — a serve started under another HOME or ENGRAM_DATA_DIR holds the port", 2
	}
	return "engram serve at " + u + " is this store (" + dir + ", instance " + got + ")", 0
}

// engramSessionPost is engram_session_post: 0 only when engram echoes the id.
func (b *brainRun) engramSessionPost(id, project, dir, u string) bool {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(struct {
		ID        string `json:"id"`
		Project   string `json:"project"`
		Directory string `json:"directory"`
	}{id, project, dir})
	body := strings.TrimRight(buf.String(), "\n")
	cl := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", u+"/sessions", strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cl.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	if resp.StatusCode >= 400 {
		return false
	}
	return bjTestEq(out.String(), id, "id")
}

// engramRegister is brain_engram_register: the brainer gets an engram session of its own, on a proven serve.
func (b *brainRun) engramRegister() {
	id := "brainer-" + b.engramLabel
	if !isReadable(b.binDir + "/engram-serve.sh") {
		return
	}
	u := b.engramURL()
	b.engramServeWait(u) // # MUTATION-ANCHOR: go-401-M01
	verdict, rc := b.engramServeVerdict(u)
	if rc == 0 && b.engramSessionPost(id, b.engramLabel, b.dir, u) {
		b.engramSession = id
		b.errf("brain: engram session %s registered under %s (renewed at every turn end)\n", id, b.engramLabel)
		return
	}
	if verdict == "" {
		verdict = "POST " + u + "/sessions refused"
	}
	b.errf("brain: engram session NOT registered: %s. A mem_save without session_id may fall into manual-save-%s.\n", verdict, b.engramLabel)
}

// ── the cockpit writer, the outbox ──────────────────────────────────────────────────────────────────────────────

// cockpitWriter is _brain_cockpit_writer: the heartbeat of a claude brainer's cockpit, bound to the pane.
func (b *brainRun) cockpitWriter(kind, pane string) {
	if kind != "claude" || pane == "" || !isExecutable(b.binDir+"/cockpit-state") {
		return
	}
	if b.noCockpit || b.getenv("FOREMAN_COCKPIT") == "0" { // # MUTATION-ANCHOR: go-804-M02
		return
	}
	b.spawn(b.binDir+"/cockpit-state", []string{"cockpit-state", "--loop", "5", "--bind-pane", "--invoker", pane}, b.env)
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

// flushOutbox is _brain_flush_outbox: reports that never reached a brainer are handed over when its pane opens.
func (b *brainRun) flushOutbox(proj, pane string) {
	if pane == "" {
		return
	}
	r := b.run(brainCmd{Name: b.brain + "/bin/hw", Args: []string{"outbox", "flush", proj, "--to", pane}, Merge: true, Extra: []string{"HW_INVOKER_WAIT_MS=30000"}})
	out := brTrimNL(r.Out)
	if r.RC != 0 { // # MUTATION-ANCHOR: go-243-M03
		b.errf("brain: some undelivered reports could not be redelivered to %s — `hw outbox %s` lists them\n%s\n", pane, proj, out)
		return
	}
	if out != "" {
		b.outf("%s\n", out)
	}
}

// laneTableDiag is what bin/brain prints for a lane table that does not load, byte for byte: the lines of lane_config_load —
// jq's own words, which differ from one jq version to the next, a bash crash on an empty file, the error of the first
// document of a file whose second does not parse — and the closing «brain: the lane table did not load». loadTable words
// them its own way (the contract hw-core keeps for `hw` leaves jq's diagnostic out), so here the frozen loader is asked, run
// as bin/brain runs it. Without bash, jq or bin/project-spaces.sh (the bash is deleted in 0.6.0) Go's wording stays, and so
// it does when the loader accepts a table Go refused.
func (b *brainRun) laneTableDiag(file string) (string, bool) {
	prog := `set -euo pipefail; . "$1/project-spaces.sh"; lane_config_load "$2" || { echo "brain: the lane table did not load — fix $2/projects.json (the reason is above)" >&2; exit 1; }`
	r := b.run(brainCmd{Name: "bash", Args: []string{"-c", prog, "brain", b.binDir, b.brain}, Merge: true, Extra: []string{"HW_PROJECTS_JSON=" + file}})
	if r.RC == 0 || r.Out == "" { // # MUTATION-ANCHOR: go-p3-M01
		return "", false
	}
	return r.Out, true
}

// ── version ─────────────────────────────────────────────────────────────────────────────────────────────────────

// cliVersion is cli_version: what install.sh recorded, else the tag HEAD sits on, else the short sha.
func (b *brainRun) cliVersion(root string) string {
	v := ""
	if data, err := os.ReadFile(root + "/.brain-install.json"); err == nil {
		v = jqVersion(data)
	}
	if v == "" || v == "null" {
		r := b.run(brainCmd{Name: "git", Args: []string{"-C", root, "describe", "--tags", "--exact-match"}})
		v = brTrimNL(r.Out)
		if r.RC != 0 {
			r = b.run(brainCmd{Name: "git", Args: []string{"-C", root, "rev-parse", "--short", "HEAD"}})
			v = brTrimNL(r.Out)
			if r.RC != 0 {
				v = ""
			}
		}
	}
	if v == "" {
		return "unknown"
	}
	return v
}

// jqVersion is `jq -r '.version // ((.commit // "") | .[0:7]) // empty' FILE 2>/dev/null || true` inside a `$( )`: what jq
// prints for each document of the file (a document it cannot read ends the stream, one it cannot index prints nothing
// and the next is still read), trailing newlines dropped. A version that is not a string prints as jq prints it:
// an object or an array pretty-printed, a number as its literal.
func jqVersion(data []byte) string {
	docs, _ := parseStream(data)
	var out strings.Builder
	for _, d := range docs {
		if s, ok := jqVersionOf(d); ok {
			out.WriteString(s)
			out.WriteByte('\n')
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// jqVersionOf is the program on one document; ok is false when jq errors on it (it prints nothing).
func jqVersionOf(d *jval) (string, bool) {
	ver, ok := d.get("version")
	if !ok {
		return "", false
	}
	if ver.truthy() { // # MUTATION-ANCHOR: go-p2-M01
		return reviveRaw(ver), true // # MUTATION-ANCHOR: go-p2-M02
	}
	com, _ := d.get("commit") // d is an object or null here: get cannot fail
	if !com.truthy() {
		com = jstr("")
	}
	switch com.kind {
	case vStr:
		r := []rune(com.s)
		if len(r) > 7 { // # MUTATION-ANCHOR: go-p2-M03
			r = r[:7]
		}
		return string(r), true
	case vArr:
		cut := &jval{kind: vArr, arr: com.arr[:min(len(com.arr), 7)]}
		return reviveRaw(cut), true
	}
	return "", false // `.[0:7]` of a number, a boolean or an object is a jq error
}

// cockpitFloor is the claude row of lib/hw/deps.conf (the one place the versions live): the floor, and whether the
// row was found at all. lib/hw/deps.sh is looked for beside bin/, else in HW_LIB_DIR.
func (b *brainRun) cockpitFloor() (string, bool) {
	lib := b.binDir + "/../lib"
	if !isReadable(lib + "/hw/deps.sh") {
		if d := b.getenv("HW_LIB_DIR"); d != "" {
			lib = d
		}
	}
	if !isReadable(lib + "/hw/deps.sh") {
		return "", false
	}
	conf := b.getenv("HW_DEPS_CONF")
	if conf == "" {
		conf = lib + "/hw/deps.conf"
	}
	body, err := os.ReadFile(conf)
	if err != nil {
		return "", false
	}
	text := string(body)
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	} else {
		text = ""
	}
	for _, line := range strings.Split(text, "\n") {
		f := strings.SplitN(line, "|", 7)
		if f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[0] == "claude" {
			if len(f) > 1 {
				return f[1], true
			}
			return "", true
		}
	}
	return "", false
}

var reSemver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+`)

func (b *brainRun) claudeVersion() string {
	r := b.run(brainCmd{Name: "claude", Args: []string{"--version"}})
	first := strings.SplitN(r.Out, "\n", 2)[0]
	return reSemver.FindString(first)
}
