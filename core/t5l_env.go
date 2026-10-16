package main

// Area A1 — the run's environment (bin/hw: _hw_env, _export_hw_env, _write_run_env, _trust_dir, _apply_sdd,
// _ensure_artifacts_dir, _artifacts_dir_for, claude_session_env_prep from bin/project-spaces.sh), and the private
// helpers they call that no other file of the port holds: _review_base_ref, _subagent_model_applies and its two
// receipt words, _opencode_config_content, _export_opencode_overlay, _brain_guard_on, _checks_gate_applies,
// _exclude_odd.
//
// Every file write goes through l.fs (HW_CORE_READONLY records it); every program is run through l.sh.
// ~/.claude.json is edited by python3 in bash (json.load / json.dump(indent=2) / os.replace): the Go below carries
// its own ordered parser and dumper (ceNode) so the bytes it leaves are the bytes json.dump leaves.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type lenvState struct {
	sddAppliedDir string // SDD_APPLIED_DIR: the directory the mode was checked for
	envUnset      map[string]bool
	envWrapped    bool
}

// unsetEnv is bash's `unset VAR`: the dispatch's override goes, the process's value goes, and d.env reads "".
// The process is hw-core's own: the unset is the process's, as bash's was.
func (l *lch) unsetEnv(keys ...string) {
	for _, k := range keys {
		delete(l.envOver, k)
		os.Unsetenv(k)
		if l.envUnset == nil {
			l.envUnset = map[string]bool{}
		}
		l.envUnset[k] = true
	}
	if !l.envWrapped {
		l.envWrapped = true
		inner := l.getenv
		l.getenv = func(k string) string {
			if l.envUnset[k] {
				return ""
			}
			return inner(k)
		}
	}
}

// ceBashWords is the word splitting of an unquoted `$(…)`: IFS is space, tab and newline.
func ceBashWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

func ceB01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ── small helpers of bin/hw this area calls ─────────────────────────────────

// subagentModelApplies is `_subagent_model_applies`.
func (l *lch) subagentModelApplies() bool {
	return l.agent == "claude" && l.subagentModel != "" && l.subagentModel != "inherit" // # MUTATION-ANCHOR: 847-A1-01
}

// subagentModelReceiptValue is `_subagent_model_receipt_value`.
func (l *lch) subagentModelReceiptValue() string {
	switch {
	case l.agent != "claude":
		return "NOT APPLIED — " + orDefaultStr(l.agent, "unknown") + " does not read CLAUDE_CODE_SUBAGENT_MODEL (asked: " + orDefaultStr(l.subagentModel, "inherit") + ")"
	case l.subagentModelApplies():
		return l.subagentModel
	}
	return "inherit"
}

// subagentModelReceiptSource is `_subagent_model_receipt_source`.
func (l *lch) subagentModelReceiptSource() string {
	return "subagent_model " + orDefaultStr(l.subagentModel, "inherit") + " (" + orDefaultStr(l.subagentModelSrc, "default") + ") at dispatch; exported as CLAUDE_CODE_SUBAGENT_MODEL only for claude"
}

// brainGuardOn is `_brain_guard_on`: it applies AND it is on disk.
func (l *lch) brainGuardOn() bool { return l.brainGuardApplies() && l.brainGuardPresent() }

// checksGateApplies is `_checks_gate_applies`: a build leaves checks.md.
func (l *lch) checksGateApplies() bool {
	switch l.briefKindForClose() {
	case "", "build":
		return true // # MUTATION-ANCHOR: 847-A1-02
	}
	return false
}

// opencodeConfigContent is `_opencode_config_content`: the --effort overlay and the engram proxy, deep-merged by jq.
func (l *lch) opencodeConfigContent() (string, int) {
	c, a, b := "", l.opencodeEffortOverlay, l.vdEngramOver
	if l.agent == "opencode" {
		c = l.explorer().opencodeOverlay() // THE CHEAP EXPLORER rides the same layer; merged FIRST, so the primary's effort still wins its key
	}
	if c == "" && a == "" && b == "" {
		return "", 0
	}
	if c == "" {
		c = "null"
	}
	if a == "" {
		a = "null"
	}
	if b == "" {
		b = "null"
	}
	out, rc := l.sh(cmdSpec{name: "jq", args: []string{"-nc", "--argjson", "c", c, "--argjson", "a", a, "--argjson", "b", b, "($c // {}) * ($a // {}) * ($b // {})"}})
	if rc != 0 {
		return trimNL(out), rc
	}
	// jq decoded the explorer's \u0020 back into spaces: spell them again, so the overlay stays ONE word.
	return strings.ReplaceAll(trimNL(out), " ", `\u0020`), rc
}

// exportOpencodeOverlay is `_export_opencode_overlay`: the unset is as load-bearing as the export.
func (l *lch) exportOpencodeOverlay() {
	l.unsetEnv("OPENCODE_CONFIG_CONTENT", "HW_OPENCODE_EFFORT", "HW_OPENCODE_EFFORT_AGENT")
	if l.agent != "opencode" {
		return
	}
	content, rc := l.opencodeConfigContent()
	if rc != 0 {
		panic(exitNow{rc}) // `OPENCODE_CONFIG_CONTENT="$(…)"` is a plain assignment: set -e ends hw
	}
	if content != "" {
		l.setEnv("OPENCODE_CONFIG_CONTENT", content)
	} else {
		l.unsetEnv("OPENCODE_CONFIG_CONTENT")
	}
	if l.opencodeEffortOverlay != "" {
		l.setEnv("HW_OPENCODE_EFFORT", l.effort)
		l.setEnv("HW_OPENCODE_EFFORT_AGENT", l.opencodePrimaryAgent)
	}
}

// reviewBaseRef is `_review_base_ref <workdir>`: the merge-base SHA a Judgment Day target is built against, or
// nothing — silence is the right answer for a directory that is not a checkout or a base that does not resolve.
func (l *lch) reviewBaseRef(wd string) string {
	if wd == "" || !fileExists(wd+"/.git") { // # MUTATION-ANCHOR: 847-A1-03
		return ""
	}
	base := l.taskBase()
	if base == "" {
		return ""
	}
	ref := l.laneBaseRefPrefix() + base
	if _, rc := l.git(false, wd, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); rc != 0 { // # MUTATION-ANCHOR: 847-A1-04
		return ""
	}
	out, _ := l.git(false, wd, "merge-base", "HEAD", ref)
	return trimNL(out)
}

// excludeOdd is `_exclude_odd <worktree>`: odd/ ignored in THIS worktree only, by an untracked odd/.gitignore.
func (l *lch) excludeOdd(wt string) {
	if _, rc := l.git(false, wt, "rev-parse", "--is-inside-work-tree"); rc != 0 {
		return
	}
	if err := l.fs.mkdirAll(wt + "/odd"); err != nil {
		l.warn("cannot create " + wt + "/odd; ODD's feature documents may show in git status")
		return
	}
	if !fileExists(wt + "/odd/.gitignore") {
		if err := l.fs.write(wt+"/odd/.gitignore", []byte("*\n"), 0o666); err != nil {
			l.warn("cannot write " + wt + "/odd/.gitignore; ODD's feature documents may show in git status")
			return
		}
	}
	l.info("odd/ ignored in this worktree only (untracked odd/.gitignore), so ODD's feature documents never reach a commit")
}

// ── _hw_env ──────────────────────────────────────────────────────────────────

// hwEnvArgs is `_hw_env "$cwd"`: the `--env K=V` words herdr is given for a pane. bash prints one string that the
// caller word-splits unquoted, so a value with a blank is split into more words; the same happens here.
func (l *lch) hwEnvArgs(cwd string) []string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	p("--env HW_PROJECT=%s --env HW_TASK=%s --env HW_WORKDIR=%s --env HW_RUN=%s", l.proj, l.task, cwd, l.hwRun)
	if art := l.env("HW_ARTIFACTS"); art != "" {
		p(" --env HW_ARTIFACTS=%s", art)
	}
	p(" --env HW_EXECUTOR_VENDOR=%s", l.agent)
	// Emitted only when the redirect applies, so a pane never carries a config dir its vendor will not read.
	if l.accountApplies && l.accountConfigDir != "" { // # MUTATION-ANCHOR: 847-A1-05
		p(" --env CLAUDE_CONFIG_DIR=%s", l.accountConfigDir)
	}
	if l.agent == "claude" { // # MUTATION-ANCHOR: 847-A1-06
		p(" --env CLAUDE_CODE_DISABLE_AUTO_MEMORY=1")
	}
	if l.subagentModelApplies() {
		p(" --env CLAUDE_CODE_SUBAGENT_MODEL=%s", l.subagentModel)
	}
	// opencode-auto parity: hw passes --auto in the agent args and this variable here.
	if l.agent == "opencode" {
		p(" --env OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true")
	}
	if l.agent == "opencode" && l.opencodeExecutorConfig != "" {
		p(" --env OPENCODE_CONFIG_DIR=%s", l.opencodeExecutorConfig)
	}
	if l.agent == "opencode" {
		if content, _ := l.opencodeConfigContent(); content != "" {
			content, _ = l.opencodeConfigContent()
			p(" --env OPENCODE_CONFIG_CONTENT=%s", content)
		}
	}
	if l.agent == "opencode" && l.opencodeEffortOverlay != "" {
		p(" --env HW_OPENCODE_EFFORT=%s --env HW_OPENCODE_EFFORT_AGENT=%s", l.effort, l.opencodePrimaryAgent)
	}
	if l.agent == "opencode" && l.brainGuardOn() {
		p(" --env OPENCODE_CONFIG=%s", l.brainGuardOpencodeConfig())
	}
	p(" --env ENGRAM_PROJECT=%s", l.engramLabel(l.proj))
	// The brainer's pane in BOTH modes, resolved and not inherited (a stale HERDR_PANE_ID is an address that is gone).
	if pane := l.invokerPane(); pane != "" {
		p(" --env HW_INVOKER_PANE=%s", pane)
	}
	if !l.wantReport {
		p(" --env HW_NO_REPORT=1") // # MUTATION-ANCHOR: 847-A1-07
	}
	p(" --env HW_CHAINING_ENABLED=%s", ceB01(l.keepPane))
	if l.keepPaneSeconds != "" {
		p(" --env HW_CHAINING_LEASE_SECONDS=%s", l.keepPaneSeconds)
	}
	if pane := l.invokerPane(); pane != "" {
		p(" --env HW_CHAINING_LEASE_HOLDER=%s", pane)
	}
	// The invoker wait budget is set by the side that knows how busy its brainer is.
	p(" --env HW_INVOKER_WAIT_MS=%s", orDefaultStr(l.env("HW_INVOKER_WAIT_MS"), "540000"))
	p(" --env HW_NEXT_WAIT_MS=%s", orDefaultStr(l.env("HW_NEXT_WAIT_MS"), "900000"))
	if v := l.env("HW_INVOKER_VENDOR"); v != "" {
		p(" --env HW_INVOKER_VENDOR=%s", v)
	}
	if v := l.env("HW_INVOKER_SESSION"); v != "" {
		p(" --env HW_INVOKER_SESSION=%s", v)
	}
	if v := l.env("HW_INVOKER_ENDPOINT"); v != "" {
		p(" --env HW_INVOKER_ENDPOINT=%s", v)
	}
	if v := l.env("HW_AGENT_PORT"); v != "" {
		p(" --env HW_AGENT_PORT=%s --env HW_AGENT_ENDPOINT=%s", v, l.env("HW_AGENT_ENDPOINT"))
	}
	if v := l.env("HW_PORT_WEB"); v != "" {
		p(" --env HW_PORT_WEB=%s", v)
	}
	if v := l.env("HW_PORT_API"); v != "" {
		p(" --env HW_PORT_API=%s", v)
	}
	if v := l.env("AGENT_BROWSER_NAMESPACE"); v != "" {
		p(" --env AGENT_BROWSER_NAMESPACE=%s --env HW_BROWSER_PROFILE=%s", v, l.env("HW_BROWSER_PROFILE"))
	}
	if v := l.env("HW_REVIEW_BASE_REF"); v != "" {
		p(" --env HW_REVIEW_BASE_REF=%s --env HW_REVIEW_BASE_BRANCH=%s", v, l.env("HW_REVIEW_BASE_BRANCH"))
	}
	// THREE MODES, THREE VALUES, stated and not inferred.
	switch {
	case l.here:
		p(" --env HW_LAUNCH_MODE=here") // # MUTATION-ANCHOR: 847-A1-08
	case l.tab:
		p(" --env HW_LAUNCH_MODE=tab")
	default:
		p(" --env HW_LAUNCH_MODE=space")
	}
	return ceBashWords(b.String())
}

// ── _export_hw_env ───────────────────────────────────────────────────────────

// exportHwEnv is `_export_hw_env "$cwd"`: HW_* into the process, so base_env, the layout documents and the run env
// file read one definition. THE UNSETS ARE AS LOAD-BEARING AS THE EXPORTS: base_env copies this process, and a
// brainer's own CLAUDE_CONFIG_DIR / OPENCODE_CONFIG / auto-memory / subagent model must not reach another vendor.
func (l *lch) exportHwEnv(cwd string) {
	l.invokerPaneInit() // one `herdr pane list` per dispatch, filled in the parent shell
	l.hwWorkdir = cwd
	l.setEnv("HW_PROJECT", l.proj)
	l.setEnv("HW_TASK", l.task)
	l.setEnv("HW_WORKDIR", cwd)
	l.setEnv("HW_EXECUTOR_VENDOR", l.agent)
	if l.accountApplies && l.accountConfigDir != "" { // # MUTATION-ANCHOR: 847-A1-09
		l.setEnv("CLAUDE_CONFIG_DIR", l.accountConfigDir)
	} else {
		l.unsetEnv("CLAUDE_CONFIG_DIR")
	}
	if l.agent == "opencode" && l.brainGuardOn() {
		l.setEnv("OPENCODE_CONFIG", l.brainGuardOpencodeConfig())
	} else {
		l.unsetEnv("OPENCODE_CONFIG")
	}
	// NO AUTO-MEMORY IN AN EXECUTOR: its contract is its brief and its lane's CLAUDE.md.
	if l.agent == "claude" {
		l.setEnv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "1") // # MUTATION-ANCHOR: 847-A1-10
	} else {
		l.unsetEnv("CLAUDE_CODE_DISABLE_AUTO_MEMORY")
	}
	if l.subagentModelApplies() {
		l.setEnv("CLAUDE_CODE_SUBAGENT_MODEL", l.subagentModel)
	} else {
		l.unsetEnv("CLAUDE_CODE_SUBAGENT_MODEL")
	}
	l.setEnv("ENGRAM_PROJECT", l.engramLabel(l.proj))
	// HW_ARTIFACTS: always safe to write and never in `git status`.
	l.setEnv("HW_ARTIFACTS", l.artifactsDirFor(l.proj, l.hwWorkdir))
	l.setEnv("HW_RUN", l.hwRun)
	l.setEnv("HW_INVOKER_VENDOR", l.hwInvokerVendor)
	l.setEnv("HW_INVOKER_SESSION", l.hwInvokerSession)
	l.setEnv("HW_INVOKER_ENDPOINT", l.hwInvokerEndpoint)
	// THE BASE A JUDGMENT DAY TARGET IS BUILT AGAINST, computed once, after the worktree exists. UNSET, NOT EMPTY,
	// when it could not be computed.
	ref := l.reviewBaseRef(l.hwWorkdir)
	if ref != "" {
		l.setEnv("HW_REVIEW_BASE_REF", ref)
		l.setEnv("HW_REVIEW_BASE_BRANCH", l.laneBaseRefPrefix()+l.taskBase())
	} else {
		l.unsetEnv("HW_REVIEW_BASE_REF", "HW_REVIEW_BASE_BRANCH")
	}
	if !l.wantReport {
		l.setEnv("HW_NO_REPORT", "1")
	} else {
		l.unsetEnv("HW_NO_REPORT")
	}
	l.setEnv("HW_CHAINING_ENABLED", ceB01(l.keepPane))
	l.setEnv("HW_CHAINING_LEASE_SECONDS", l.keepPaneSeconds)
	l.setEnv("HW_CHAINING_LEASE_REASON", l.keepPaneWhy)
	l.setEnv("HW_CHAINING_LEASE_HOLDER", l.invokerPane())
	if l.agent == "opencode" {
		l.setEnv("OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS", "true")
		if l.opencodeExecutorConfig != "" {
			l.setEnv("OPENCODE_CONFIG_DIR", l.opencodeExecutorConfig)
			l.setEnv("OPENCODE_EXECUTOR_CONFIG", l.opencodeExecutorConfig)
		}
	}
	l.exportOpencodeOverlay()
	// BROWSER ISOLATION per run: a task can only see its own sessions. The profile is a PATH, offered not applied.
	l.setEnv("AGENT_BROWSER_NAMESPACE", "hw-"+orDefaultStr(l.hwRun, "norun"))
	l.setEnv("HW_BROWSER_PROFILE", l.env("HW_ARTIFACTS")+"/browser-profile")
	// THE EXECUTOR'S OWN ADDRESS, which hw has known since the agent args were built.
	if l.agent == "opencode" && l.opencodeAgentPort != "" {
		l.setEnv("HW_AGENT_PORT", l.opencodeAgentPort)
		l.setEnv("HW_AGENT_ENDPOINT", "http://127.0.0.1:"+l.opencodeAgentPort)
	}
}

// ── _artifacts_dir_for, _ensure_artifacts_dir ────────────────────────────────

// artifactsDirFor is `_artifacts_dir_for <project> <work directory>`: where a task's artifacts go. Keyed on the
// directory as well as the lane, because for `setup` a worktree and a plain work directory are the same path.
func (l *lch) artifactsDirFor(proj, wd string) string {
	if l.c.checkout(proj) == "" { // # MUTATION-ANCHOR: 847-A1-11
		return wd
	}
	if root := l.c.lanePath(proj, "worktree_root", ""); root != "" && root == l.c.work+"/"+proj && !fileExists(wd+"/.git") { // # MUTATION-ANCHOR: 847-A1-12
		return wd
	}
	return wd + "/.artifacts"
}

// ensureArtifactsDir is `_ensure_artifacts_dir "$wt"`: .artifacts exists and is excluded, in the exclude file and
// not in .gitignore (which is versioned, and this setup commits nothing to either repo).
func (l *lch) ensureArtifactsDir(wt string) {
	l.fs.mkdirAll(wt + "/.artifacts")
	out, _ := l.git(false, wt, "rev-parse", "--git-path", "info/exclude")
	ex := trimNL(out)
	if ex == "" {
		return
	}
	if !strings.HasPrefix(ex, "/") {
		ex = wt + "/" + ex
	}
	l.fs.mkdirAll(filepath.Dir(ex))
	for _, line := range []string{".artifacts/", ".hw/"} {
		if _, rc := l.sh(cmdSpec{name: "rg", args: []string{"-qxF", line, ex}, quiet: true}); rc != 0 {
			l.fs.appendTo(ex, []byte(line+"\n"), 0o666)
		}
	}
	l.info("artifacts → " + wt + "/.artifacts (excluded, so it never shows in git status)")
}

// ── _apply_sdd ───────────────────────────────────────────────────────────────

// applySdd is `_apply_sdd <dir> <mode>`: true is bash's status 0.
func (l *lch) applySdd(dir, mode string) bool {
	l.sddAppliedDir = dir
	if mode == "gentle" {
		l.excludeOdd(dir) // gentle writes one untracked file: odd/.gitignore
		return true
	}
	if mode != "speckit" { // # MUTATION-ANCHOR: 847-A1-14
		return true
	}
	if where, ok := l.rtSpeckitOnDiskDir(dir); ok {
		l.ok("SDD mode: speckit (skills in " + where + ")")
		return true
	}
	l.warn("--sdd speckit, but no speckit-* skill is on disk under " + dir + "/.claude/skills or " + l.env("HOME") + "/.claude/skills")
	return false
}

// ── claude_session_env_prep ──────────────────────────────────────────────────

// claudeSessionEnvPrep is `claude_session_env_prep <pane> [<fn>]`: a claude launched by hw is never a child session. The
// pane's shell is told to unset CLAUDE_CODE_CHILD_SESSION and to force session persistence before `agent start`, and
// then runs fn (hw: the explorer's function) when there is one.
// A pane that already runs an agent is left alone: typing into it would land in that agent's prompt.
func (l *lch) claudeSessionEnvPrep(pane, fn string) {
	if pane == "" {
		return
	}
	got, _ := l.herdr(false, true, "", "agent", "get", pane)
	if ceJqAgentPresent(got) { // # MUTATION-ANCHOR: 847-A1-16
		return
	}
	prep := "unset CLAUDE_CODE_CHILD_SESSION"
	prep += "; export CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1"
	prep += "; export CLAUDE_CODE_BASH_EDIT_DIFF=0"
	if fn != "" { // # MUTATION-ANCHOR: 1007-G03
		prep += "; " + fn
	}
	if _, rc := l.sh(cmdSpec{name: "herdr", args: []string{"pane", "run", pane, prep}, mut: true, merge: true}); rc != 0 { // # MUTATION-ANCHOR: 847-A1-17
		l.errf("claude: could not clear CLAUDE_CODE_CHILD_SESSION in %s; Claude may report 'Transcript saving is off'.\n", pane)
	}
}

// ceJqAgentPresent is `jq -e '.result.agent != null' >/dev/null 2>&1` succeeding: the last output is true, and
// nothing in the stream was an error (jq -e exits 4 on no output, 5 on a false/null, 2 on an error).
func ceJqAgentPresent(text string) bool {
	vals, err := parseStream([]byte(text))
	if err != nil || len(vals) == 0 {
		return false
	}
	last := false
	for _, v := range vals {
		r, ok := v.get("result")
		if !ok {
			return false
		}
		a, ok := r.get("agent")
		if !ok {
			return false
		}
		last = a.kind != vNull
	}
	return last
}

// ── _trust_dir ───────────────────────────────────────────────────────────────

// trustDir is `_trust_dir "$dir"`: mark a directory trusted in the claude.json of the ACCOUNT the executor will
// run under. An untrusted workspace makes Claude Code SILENTLY ignore permissions.allow. Backs up before writing,
// never lowers an existing decision to false, and prunes its own backups to the five newest.
func (l *lch) trustDir(dir string) {
	cfg := l.env("HOME") + "/.claude.json"
	// `$CFG_DIR/.claude.json` under a redirect; the unredirected default keeps the legacy ~/.claude.json.
	if l.accountConfigDir != "" {
		cfg = l.accountConfigDir + "/.claude.json" // # MUTATION-ANCHOR: 847-A1-18
	}
	if !isFile(cfg) {
		return
	}
	bak := cfg + ".bak-" + time.Now().Format("20060102-150405")
	if err := ceCopyFile(l.fs, cfg, bak); err != nil { // # MUTATION-ANCHOR: 847-A1-19
		l.warn("could not back up " + cfg + "; leaving trust alone")
		return
	}
	switch l.trustEdit(cfg, dir) {
	case 0:
		l.ok("workspace trusted in " + cfg + " (without this, permissions.allow is silently ignored and Claude Code opens on its folder-trust dialog)")
	case 2: // already trusted, nothing written
		l.info("workspace already trusted in " + cfg) // # MUTATION-ANCHOR: 847-A1-20
	default:
		l.warn("could not mark " + dir + " trusted in " + cfg + " — the agent's allowlist may be ignored there, and it may open on the folder-trust dialog instead of starting")
	}
	l.pruneTrustBackups(cfg, bak)
}

// ceCopyFile is `cp src dst`: the bytes, the source's mode under the umask.
func ceCopyFile(fs *fsx, src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	return fs.write(dst, data, st.Mode().Perm())
}

// trustEdit is the python3 heredoc of _trust_dir: 0 written, 2 already trusted, 1 anything else.
func (l *lch) trustEdit(cfg, dir string) int {
	raw, err := os.ReadFile(cfg)
	if err != nil || !utf8.Valid(raw) {
		return 1
	}
	root, ok := ceParse(string(raw))
	if !ok {
		return 1
	}
	if root.kind != ceObj { // data.setdefault: AttributeError, an uncaught python exception (exit 1)
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
	p := projects.member(dir)
	if p == nil {
		p = &ceNode{kind: ceObj}
		projects.set(dir, p)
	}
	if p.kind != ceObj {
		return 1
	}
	if t := p.member("hasTrustDialogAccepted"); t != nil && t.kind == ceBool && t.b { // # MUTATION-ANCHOR: 847-A1-21
		return 2
	}
	p.set("hasTrustDialogAccepted", &ceNode{kind: ceBool, b: true})
	var sb strings.Builder
	root.dump(&sb, 0)
	tmp := cfg + ".hw-tmp"
	if err := l.fs.write(tmp, []byte(sb.String()), 0o666); err != nil {
		return 1
	}
	if err := l.fs.rename(tmp, cfg); err != nil {
		return 1
	}
	return 0
}

// pruneTrustBackups is `ls -t "$cfg".bak-* | tail -n +6 | xargs rm -f`: the five newest stay. Read-only, the
// backup bash would have just made is counted too (as the newest), so the same files are pruned.
func (l *lch) pruneTrustBackups(cfg, newest string) {
	dir, base := filepath.Split(cfg)
	if dir == "" {
		dir = "."
	}
	ents, _ := os.ReadDir(dir)
	type bk struct {
		path string
		ns   int64
	}
	var all []bk
	seen := false
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), base+".bak-") {
			continue
		}
		fi, err := os.Stat(dir + e.Name())
		if err != nil {
			continue
		}
		if dir+e.Name() == newest {
			seen = true
		}
		all = append(all, bk{dir + e.Name(), fi.ModTime().UnixNano()})
	}
	if !seen && l.fs.readonly {
		all = append(all, bk{newest, time.Now().UnixNano()})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ns != all[j].ns {
			return all[i].ns > all[j].ns
		}
		return all[i].path < all[j].path
	})
	for i := 5; i < len(all); i++ { // # MUTATION-ANCHOR: 847-A1-22
		l.fs.remove(all[i].path)
	}
}

// ── _write_run_env ───────────────────────────────────────────────────────────

// writeRunEnv is `_write_run_env "$workdir" "$extraJSON" "$invoker"`: <workdir>/.hw/<run>/env (KEY='value', one
// per line, mode 0600) and its siblings dispatch, verify and the receipts. It is the second writer of the fact
// `_hw_env` emits as flags, and the one that PERSISTS: `hw status` is judged against it, and so is a pane that comes
// back after a herdr crash (shell/hw-restore-env.zsh reads this grammar).
func (l *lch) writeRunEnv(workdir, extra, invoker string) {
	mode := "space"
	if l.here {
		mode = "here"
	} else if l.tab {
		mode = "tab"
	}
	if extra == "" {
		extra = "{}"
	}
	dir := workdir + "/.hw/" + l.hwRun
	if err := l.fs.mkdirAll(dir); err != nil { // # MUTATION-ANCHOR: 847-A1-23
		l.warn("could not create " + dir + " — this run has no restore env file, so a pane that")
		l.warn("comes back after a herdr crash will have no HW_* environment")
		return
	}
	// The manifest goes beside the env file: "how was this dispatched" stays answerable after the task is closed.
	if l.dispatchText != "" {
		l.fs.write(dir+"/dispatch", []byte(l.dispatchText+"\n"), 0o666)
	}
	// THE SUBAGENTS' MODEL, written for every vendor: "NOT APPLIED" and "inherit" are answers too.
	l.receiptInto(dir, "subagent_model", l.subagentModelReceiptValue(), l.subagentModelReceiptSource())
	// ONLY WHEN THE BRIEF DECLARES THEM: a run with no `plugins` receipt is a run that asked for none.
	if len(l.pluginsDeclared()) > 0 { // # MUTATION-ANCHOR: 847-P-04
		l.receiptInto(dir, "plugins", l.pluginsReceiptValue(), l.pluginsReceiptSource())
	}
	// ONLY WHEN IT RESOLVED: a receipt is a fact on disk and a blank one would be silence dressed as an answer.
	if ref := l.env("HW_REVIEW_BASE_REF"); ref != "" { // # MUTATION-ANCHOR: 847-A1-24
		l.receiptInto(dir, "review_base", ref,
			"git merge-base HEAD "+orDefaultStr(l.env("HW_REVIEW_BASE_BRANCH"), "<base>")+" in "+workdir+" at dispatch — the base a Judgment Day target is built against")
	}
	// THE BOUNDARY done-invoker HOLDS TASK 1 TO, keyed by task: a chained executor shares this receipt.
	if l.designGateApplies() {
		l.receiptInto(dir, "design_boundary_t1", l.briefBoundary(),
			"brief frontmatter boundary: in "+l.brief+" at dispatch — done-invoker requires an APPROVED design judgment")
	}
	if l.checksGateApplies() {
		l.receiptInto(dir, "checks_required_t1", "required",
			"kind build (declared or undeclared) at dispatch — done-invoker requires $HW_ARTIFACTS/checks.md")
	}
	// THE DEPLOYED CHECK done-invoker HOLDS AN ODD RUN TO; an unreadable value records nothing rather than a guess.
	if l.sdd == "gentle" {
		if dc, ok := l.rtGentleDeployedCheck(); ok { // # MUTATION-ANCHOR: 847-A1-25
			l.receiptInto(dir, "deployed_check_t1", dc,
				"brief frontmatter deployed_check: in "+orDefaultStr(l.brief, "<no brief>")+" at dispatch (required when absent) — done-invoker holds a done to it")
		}
	}
	if l.reportToChosen {
		l.receiptInto(dir, "report_route",
			invoker+" — chosen by --report-to (lane postcondition deliberately overridden)",
			"--report-to "+invoker+" launch flag")
	}
	// THE VERIFICATION IS PINNED HERE, AT DISPATCH, AND NOT READ AGAIN: what runs at close is what a human saw.
	if l.brief != "" {
		var cmds string
		if isReadable(l.brief) {
			cmds, _ = l.sh(cmdSpec{name: "awk", args: []string{brAwkVerify, l.brief}, quiet: true})
		}
		l.fs.write(dir+"/verify", []byte(cmds), 0o666)
	}

	chaining := orDefaultStr(l.env("HW_CHAINING_ENABLED"), ceB01(l.keepPane))
	chainingSecs := orDefaultStr(l.env("HW_CHAINING_LEASE_SECONDS"), l.keepPaneSeconds)
	chainingWhy := orDefaultStr(l.env("HW_CHAINING_LEASE_REASON"), l.keepPaneWhy)
	holder := l.env("HW_CHAINING_LEASE_HOLDER")
	env := func(k string) string { return l.env(k) }

	values, bad := ceParseExtra(l, extra)
	if bad == 1 { // a python exception: no file, the warning below
		l.warn("could not write " + dir + "/env — a pane that comes back after a herdr crash")
		l.warn("will have no HW_* environment. Use 'herdr update --handoff' to restart.")
		return
	}
	// The lane extras first, then hw's own, so hw always wins a name clash. An empty value is dropped below, so
	// `default` persists as an ABSENT name, which is what a restored pane needs to see.
	for k, v := range map[string]string{
		"HW_PROJECT":                      l.proj,
		"HW_TASK":                         l.task,
		"HW_WORKDIR":                      workdir,
		"HW_RUN":                          l.hwRun,
		"HW_ARTIFACTS":                    env("HW_ARTIFACTS"),
		"ENGRAM_PROJECT":                  l.engramLabel(l.proj),
		"HW_EXECUTOR_VENDOR":              env("HW_EXECUTOR_VENDOR"),
		"HW_INVOKER_VENDOR":               env("HW_INVOKER_VENDOR"),
		"HW_INVOKER_SESSION":              env("HW_INVOKER_SESSION"),
		"HW_INVOKER_ENDPOINT":             env("HW_INVOKER_ENDPOINT"),
		"HW_AGENT_PORT":                   env("HW_AGENT_PORT"),
		"HW_AGENT_ENDPOINT":               env("HW_AGENT_ENDPOINT"),
		"HW_PORT_WEB":                     env("HW_PORT_WEB"),
		"HW_PORT_API":                     env("HW_PORT_API"),
		"AGENT_BROWSER_NAMESPACE":         env("AGENT_BROWSER_NAMESPACE"),
		"HW_BROWSER_PROFILE":              env("HW_BROWSER_PROFILE"),
		"HW_REVIEW_BASE_REF":              env("HW_REVIEW_BASE_REF"),
		"HW_REVIEW_BASE_BRANCH":           env("HW_REVIEW_BASE_BRANCH"),
		"CLAUDE_CONFIG_DIR":               env("CLAUDE_CONFIG_DIR"),
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY": env("CLAUDE_CODE_DISABLE_AUTO_MEMORY"), // # MUTATION-ANCHOR: 847-A1-26
		"CLAUDE_CODE_SUBAGENT_MODEL":      env("CLAUDE_CODE_SUBAGENT_MODEL"),
		"OPENCODE_CONFIG_DIR":             env("OPENCODE_EXECUTOR_CONFIG"),
		"OPENCODE_CONFIG":                 env("OPENCODE_CONFIG"),
		"HW_NEXT_WAIT_MS":                 orDefaultStr(env("HW_NEXT_WAIT_MS"), "900000"),
		"OPENCODE_CONFIG_CONTENT":         env("OPENCODE_CONFIG_CONTENT"),
		"HW_OPENCODE_EFFORT":              env("HW_OPENCODE_EFFORT"),
		"HW_OPENCODE_EFFORT_AGENT":        env("HW_OPENCODE_EFFORT_AGENT"),
	} {
		values.set(k, v)
	}
	if invoker != "" {
		values.set("HW_INVOKER_PANE", invoker)
	}
	if v := env("HW_NO_REPORT"); v != "" {
		values.set("HW_NO_REPORT", v)
	}
	values.set("HW_CHAINING_ENABLED", chaining)
	if chainingSecs != "" {
		values.set("HW_CHAINING_LEASE_SECONDS", chainingSecs)
	}
	if chainingWhy != "" {
		values.set("HW_CHAINING_LEASE_REASON", chainingWhy)
	}
	if holder != "" {
		values.set("HW_CHAINING_LEASE_HOLDER", holder)
	}
	// The discriminator run_is_here() reads: a missing value has to mean "legacy run" and nothing else.
	values.set("HW_LAUNCH_MODE", mode)

	data, ok := l.runEnvBytes(values, dir+"/env")
	if ok && l.writeEnvFile(dir+"/env", data) == nil {
		l.info("restore env → " + dir + "/env (read back by shell startup after a herdr crash)")
		return
	}
	l.warn("could not write " + dir + "/env — a pane that comes back after a herdr crash")
	l.warn("will have no HW_* environment. Use 'herdr update --handoff' to restart.")
}

// isReadable is `[ -r path ]`.
func isReadable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// ceEnvValues is the python dict of _write_run_env: string values, in no order (the file is written sorted).
type ceEnvValues struct {
	m      map[string]string
	nonStr bool // a lane extra that is not a string: python's `"\n" in value` raises
}

func (v *ceEnvValues) set(k, val string) {
	if v.m == nil {
		v.m = map[string]string{}
	}
	v.m[k] = val
}

// ceParseExtra reads HW_ENV_EXTRA. 0: usable (an invalid JSON text is reported on stderr and replaced by {}, as the
// python does); 1: the python would raise.
func ceParseExtra(l *lch, extra string) (*ceEnvValues, int) {
	out := &ceEnvValues{m: map[string]string{}}
	node, ok := ceParse(extra)
	if !ok {
		// python prints the JSONDecodeError text; its position words are not reproduced here (see the report).
		l.errf("HW_ENV_EXTRA is not a JSON object (%s)\n", "Expecting value")
		return out, 0
	}
	if node.kind != ceObj {
		return out, 1 // dict(non-mapping): TypeError, uncaught
	}
	for _, m := range node.obj {
		if m.v.kind != ceStr {
			out.nonStr = true
			continue
		}
		out.m[m.k] = m.v.s
	}
	return out, 0
}

// runEnvBytes is the tail of the python of _write_run_env: sorted keys, empty values dropped, a name outside
// [A-Za-z0-9_] and a value with a newline skipped with a word on stderr, KEY='value' with ' as '\”.
func (l *lch) runEnvBytes(v *ceEnvValues, out string) ([]byte, bool) {
	run := v.m["HW_RUN"]
	var b strings.Builder
	b.WriteString("# written by hw — the environment of run " + run + ".\n")
	b.WriteString("# Reconstruct it with:  set -a; . env; set +a\n")
	b.WriteString("# Grammar is fixed: one KEY='value' per line, inner ' as '\\''.\n")
	keys := make([]string, 0, len(v.m))
	for k := range v.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := v.m[k]
		if val == "" {
			continue
		}
		if !ceIsAlnumName(k) {
			l.errf("skipping env name %s: not [A-Za-z0-9_]\n", cePyRepr(k))
			continue
		}
		if strings.Contains(val, "\n") {
			l.errf("skipping %s: its value contains a newline, which the one-line grammar cannot carry\n", k)
			continue
		}
		b.WriteString(k + "='" + strings.ReplaceAll(val, "'", `'\''`) + "'\n")
	}
	if v.nonStr { // python: TypeError on the first non-string extra
		return nil, false
	}
	return []byte(b.String()), true
}

// writeEnvFile is the O_CREAT|O_TRUNC 0600 write and the chmod 0600 that follows it.
func (l *lch) writeEnvFile(path string, data []byte) error {
	if err := l.fs.write(path, data, 0o600); err != nil { // # MUTATION-ANCHOR: 847-A1-27
		return err
	}
	if !l.fs.readonly {
		return os.Chmod(path, 0o600)
	}
	return nil
}

// ceIsAlnumName is `key.replace("_", "").isalnum()`.
func ceIsAlnumName(k string) bool {
	s := strings.ReplaceAll(k, "_", "")
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

// cePyRepr is repr(str).
func cePyRepr(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			b.WriteString(fmt.Sprintf(`\x%02x`, r))
		case r < 0x7f || unicode.IsPrint(r):
			b.WriteRune(r)
		case r < 0x100:
			b.WriteString(fmt.Sprintf(`\x%02x`, r))
		case r < 0x10000:
			b.WriteString(fmt.Sprintf(`\u%04x`, r))
		default:
			b.WriteString(fmt.Sprintf(`\U%08x`, r))
		}
	}
	b.WriteByte(q)
	return b.String()
}

// ── an ordered JSON, as python's json.load / json.dump(indent=2) see it ─────

type ceKind int

const (
	ceNull ceKind = iota
	ceBool
	ceNum
	ceStr
	ceArr
	ceObj
)

type ceMember struct {
	k string
	v *ceNode
}

type ceNode struct {
	kind ceKind
	b    bool
	s    string // a string (lone surrogates as 3-byte sequences), or a number as json.dump prints it
	arr  []*ceNode
	obj  []ceMember
}

func (n *ceNode) member(k string) *ceNode {
	for _, m := range n.obj {
		if m.k == k {
			return m.v
		}
	}
	return nil
}

// set is d[k] = v: an existing key keeps its place, a new one goes last.
func (n *ceNode) set(k string, v *ceNode) {
	for i := range n.obj {
		if n.obj[i].k == k {
			n.obj[i].v = v
			return
		}
	}
	n.obj = append(n.obj, ceMember{k, v})
}

type ceParser struct {
	d     string
	i     int
	depth int
}

// ceParse is json.loads: the value, or false where python raises. NaN, Infinity and -Infinity are accepted.
func ceParse(s string) (*ceNode, bool) {
	if !utf8.ValidString(s) {
		return nil, false
	}
	p := &ceParser{d: s}
	p.ws()
	n, ok := p.value()
	if !ok {
		return nil, false
	}
	p.ws()
	if p.i != len(p.d) {
		return nil, false
	}
	return n, true
}

func (p *ceParser) ws() {
	for p.i < len(p.d) && (p.d[p.i] == ' ' || p.d[p.i] == '\t' || p.d[p.i] == '\n' || p.d[p.i] == '\r') {
		p.i++
	}
}

func (p *ceParser) lit(w string) bool {
	if strings.HasPrefix(p.d[p.i:], w) {
		p.i += len(w)
		return true
	}
	return false
}

func (p *ceParser) value() (*ceNode, bool) {
	if p.i >= len(p.d) {
		return nil, false
	}
	switch c := p.d[p.i]; {
	case c == '"':
		s, ok := p.str()
		return &ceNode{kind: ceStr, s: s}, ok
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == 'n' && p.lit("null"):
		return &ceNode{kind: ceNull}, true
	case c == 't' && p.lit("true"):
		return &ceNode{kind: ceBool, b: true}, true
	case c == 'f' && p.lit("false"):
		return &ceNode{kind: ceBool}, true
	case c == 'N' && p.lit("NaN"):
		return &ceNode{kind: ceNum, s: "NaN"}, true
	case c == 'I' && p.lit("Infinity"):
		return &ceNode{kind: ceNum, s: "Infinity"}, true
	case c == '-' && p.lit("-Infinity"):
		return &ceNode{kind: ceNum, s: "-Infinity"}, true
	}
	return p.number()
}

func (p *ceParser) number() (*ceNode, bool) {
	start := p.i
	digits := func() int {
		n := 0
		for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	if p.i < len(p.d) && p.d[p.i] == '-' {
		p.i++
	}
	switch {
	case p.i < len(p.d) && p.d[p.i] == '0':
		p.i++
	case p.i < len(p.d) && p.d[p.i] >= '1' && p.d[p.i] <= '9':
		digits()
	default:
		return nil, false
	}
	isFloat := false
	if p.i+1 < len(p.d) && p.d[p.i] == '.' && p.d[p.i+1] >= '0' && p.d[p.i+1] <= '9' {
		p.i++
		digits()
		isFloat = true
	}
	if p.i < len(p.d) && (p.d[p.i] == 'e' || p.d[p.i] == 'E') {
		j := p.i + 1
		if j < len(p.d) && (p.d[j] == '+' || p.d[j] == '-') {
			j++
		}
		if j < len(p.d) && p.d[j] >= '0' && p.d[j] <= '9' {
			p.i = j
			digits()
			isFloat = true
		}
	}
	text := p.d[start:p.i]
	if !isFloat {
		if text == "-0" {
			text = "0"
		}
		return &ceNode{kind: ceNum, s: text}, true
	}
	f, _ := strconv.ParseFloat(text, 64)
	switch {
	case f > 1.7976931348623157e308:
		return &ceNode{kind: ceNum, s: "Infinity"}, true
	case f < -1.7976931348623157e308:
		return &ceNode{kind: ceNum, s: "-Infinity"}, true
	}
	return &ceNode{kind: ceNum, s: pyFloatRepr(f)}, true
}

func ceHex4(s string) (rune, bool) {
	if len(s) < 4 {
		return 0, false
	}
	var r rune
	for i := 0; i < 4; i++ {
		c := s[i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

// ceAppendRune is a rune into a string that may hold a lone surrogate (as 3 bytes, python's str can).
func ceAppendRune(b *strings.Builder, r rune) {
	if r >= 0xd800 && r < 0xe000 {
		b.WriteByte(0xe0 | byte(r>>12))
		b.WriteByte(0x80 | byte((r>>6)&0x3f))
		b.WriteByte(0x80 | byte(r&0x3f))
		return
	}
	b.WriteRune(r)
}

func (p *ceParser) str() (string, bool) {
	p.i++
	var b strings.Builder
	for {
		if p.i >= len(p.d) {
			return "", false
		}
		c := p.d[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), true
		case c < 0x20:
			return "", false // strict
		case c != '\\':
			b.WriteByte(c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.d) {
			return "", false
		}
		e := p.d[p.i]
		p.i++
		switch e {
		case '"', '\\', '/':
			b.WriteByte(e)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, ok := ceHex4(p.d[p.i:])
			if !ok {
				return "", false
			}
			p.i += 4
			if r >= 0xd800 && r <= 0xdbff && strings.HasPrefix(p.d[p.i:], `\u`) {
				lo, ok := ceHex4(p.d[p.i+2:])
				if !ok {
					return "", false
				}
				if lo >= 0xdc00 && lo <= 0xdfff {
					r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
					p.i += 6
				}
			}
			ceAppendRune(&b, r)
		default:
			return "", false
		}
	}
}

func (p *ceParser) array() (*ceNode, bool) {
	p.i++
	n := &ceNode{kind: ceArr}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == ']' {
		p.i++
		return n, true
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 900 {
		return nil, false // RecursionError
	}
	for {
		p.ws()
		v, ok := p.value()
		if !ok {
			return nil, false
		}
		n.arr = append(n.arr, v)
		p.ws()
		if p.i >= len(p.d) {
			return nil, false
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return n, true
		default:
			return nil, false
		}
	}
}

func (p *ceParser) object() (*ceNode, bool) {
	p.i++
	n := &ceNode{kind: ceObj}
	p.ws()
	if p.i < len(p.d) && p.d[p.i] == '}' {
		p.i++
		return n, true
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 900 {
		return nil, false
	}
	for {
		p.ws()
		if p.i >= len(p.d) || p.d[p.i] != '"' {
			return nil, false
		}
		k, ok := p.str()
		if !ok {
			return nil, false
		}
		p.ws()
		if p.i >= len(p.d) || p.d[p.i] != ':' {
			return nil, false
		}
		p.i++
		p.ws()
		v, ok := p.value()
		if !ok {
			return nil, false
		}
		n.set(k, v) // a duplicate key: the last value wins, the first position stays
		p.ws()
		if p.i >= len(p.d) {
			return nil, false
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return n, true
		default:
			return nil, false
		}
	}
}

// dump is json.dump(indent=2): "," between items, ": " after a key, every character outside ' '..'~' as \uXXXX,
// no trailing newline.
func (n *ceNode) dump(sb *strings.Builder, level int) {
	pad := func(l int) { sb.WriteString(strings.Repeat("  ", l)) }
	switch n.kind {
	case ceNull:
		sb.WriteString("null")
	case ceBool:
		if n.b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case ceNum:
		sb.WriteString(n.s)
	case ceStr:
		ceDumpStr(sb, n.s)
	case ceArr:
		if len(n.arr) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[\n")
		for i, e := range n.arr {
			if i > 0 {
				sb.WriteString(",\n")
			}
			pad(level + 1)
			e.dump(sb, level+1)
		}
		sb.WriteString("\n")
		pad(level)
		sb.WriteString("]")
	case ceObj:
		if len(n.obj) == 0 {
			sb.WriteString("{}")
			return
		}
		sb.WriteString("{\n")
		for i, m := range n.obj {
			if i > 0 {
				sb.WriteString(",\n")
			}
			pad(level + 1)
			ceDumpStr(sb, m.k)
			sb.WriteString(": ")
			m.v.dump(sb, level+1)
		}
		sb.WriteString("\n")
		pad(level)
		sb.WriteString("}")
	}
}

func ceDumpStr(sb *strings.Builder, s string) {
	u := func(r int) { sb.WriteString(fmt.Sprintf(`\u%04x`, r)) }
	sb.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 && s[i] == 0xed && i+2 < len(s) && s[i+1] >= 0xa0 {
			r = rune(0xd000 | int(s[i+1]&0x3f)<<6 | int(s[i+2]&0x3f)) // a lone surrogate
			size = 3
		}
		i += size
		switch {
		case r == '"':
			sb.WriteString(`\"`)
		case r == '\\':
			sb.WriteString(`\\`)
		case r == '\n':
			sb.WriteString(`\n`)
		case r == '\r':
			sb.WriteString(`\r`)
		case r == '\t':
			sb.WriteString(`\t`)
		case r == '\b':
			sb.WriteString(`\b`)
		case r == '\f':
			sb.WriteString(`\f`)
		case r >= ' ' && r <= '~':
			sb.WriteRune(r)
		case r >= 0x10000:
			r -= 0x10000
			u(0xd800 + int(r>>10))
			u(0xdc00 + int(r&0x3ff))
		default:
			u(int(r))
		}
	}
	sb.WriteByte('"')
}
