package main

// The message `hw next` delivers: _next_message (lib/hw/next.sh). It fills promptFacts from the RUN and renders them with
// the launch's own renderer (t5l_prompt.go): there is no second copy of the preamble to keep in step.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// nextLaneField is lane_get for the fields the Go lane table does not keep: brief_note, qa_target
// (true -> "1"), base_ref_prefix (default "origin/") and base, read from the table file as
// project-spaces.sh emits them. "" for a lane the table does not know.
func (a *app) nextLaneField(lane, key string) string {
	if !a.c.t.known(lane) {
		return ""
	}
	data, err := os.ReadFile(a.c.t.file)
	if err != nil {
		return ""
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return ""
	}
	v, ok := nextPath(vals[0], "lanes", lane, key)
	switch key {
	case "qa_target":
		if ok && v.kind == vBool && v.b {
			return "1"
		}
		return ""
	case "base_ref_prefix":
		if !ok || !v.truthy() {
			return "origin/"
		}
	default:
		if !ok || !v.truthy() {
			return ""
		}
	}
	if v.kind == vStr {
		return v.s
	}
	return v.toJSON()
}

// nextMsgEnv is what _next_message's subshell rebuilds from the RUN, never from the brainer's shell.
type nextMsgEnv struct {
	brief, workdir, engram, baseRef, baseBranch, vendor, portWeb, proj, task, session, rundir string
	reviewMode                                                                                string   // _brief_review_mode of the NEW brief
	plugins                                                                                   []string // _plugins_declared of the NEW brief
}

// nextMessage is _next_message: the facts of this re-task, rendered by the same renderer as the launch's prompt.
// sdd is the framework mode the re-task sets ("" on a dry run, which sends no entry line).
func (a *app) nextMessage(s *nextState, nextSeq, sdd string) string {
	return renderPrompt(a.nextPromptFacts(s, nextSeq, sdd))
}

// nextPromptFacts reads what the RUN knew: its env and its receipt.
func (a *app) nextPromptFacts(s *nextState, nextSeq, sdd string) *promptFacts {
	e := nextMsgEnv{brief: s.brief, workdir: s.wtdir, reviewMode: nextReviewMode(s.brief), plugins: nextPluginsDeclared(s.brief), rundir: s.rundir}
	r := e.rundir
	if v, rc := a.nextRunEnv(r, "ENGRAM_PROJECT", false); rc == 0 {
		e.engram = bashSub(v) // # MUTATION-ANCHOR: 862-G17
	} else {
		e.engram = "brain"
	}
	e.baseRef = a.nextEnvVal(r, "HW_REVIEW_BASE_REF")
	e.baseBranch = a.nextEnvVal(r, "HW_REVIEW_BASE_BRANCH")
	e.vendor = a.nextEnvVal(r, "HW_EXECUTOR_VENDOR")
	e.portWeb = a.nextEnvVal(r, "HW_PORT_WEB")
	e.proj = a.nextEnvVal(r, "HW_PROJECT")
	e.task = a.nextEnvVal(r, "HW_TASK")
	e.session = nextEngramSession(r + "/receipt.jsonl")
	if strings.HasPrefix(e.session, "none") {
		e.session = ""
	}
	f := a.nextEnvFacts(e)
	f.retask, f.retaskSeq, f.sdd, f.briefText = true, nextSeq, sdd, s.text
	// the run's HW_TASK is task 1's name; task N is named by its own brief (a bare text keeps the run's) and saves under its own topic_key
	if s.brief != "" {
		if n := strings.TrimSuffix(nextBasename(s.brief), ".md"); n != "" {
			f.task = n
		}
	}
	f.task += "-t" + nextSeq
	f.secrets = a.nextSecrets(s.brief)
	f.requestedBy, f.authorizes = s.requestedBy, s.authorizes
	if sdd != "" {
		f.sddEntry = nextFrameworkEntry(sdd)
	}
	f.entryLine = sddEntryRetaskText(f)
	return f
}

// nextRunningModel is the model the session runs, from the receipt hw wrote for it ("" when the receipt could not read one).
func nextRunningModel(rundir string) string {
	m := nextReceiptValue(rundir+"/receipt.jsonl", "model_running")
	if strings.HasPrefix(m, "none") || strings.HasPrefix(m, "n/a") {
		return ""
	}
	return m
}

// nextEnvFacts is the preamble's facts of a re-task, from the run's env as nextPromptFacts read it.
func (a *app) nextEnvFacts(e nextMsgEnv) *promptFacts {
	f := &promptFacts{lane: e.proj, task: e.task, operator: a.nextOperator(), binDir: a.nextBinDir(), brief: e.brief, label: e.engram, session: e.session,
		specialists: nextSpecialists(e.workdir), vendor: e.vendor, portWeb: e.portWeb,
		build: nextIsBuild(e.brief), reviewMode: e.reviewMode, plugins: e.plugins}
	if f.vendor == "" {
		f.vendor = a.getenv("AGENT")
	}
	l := &lch{dsp: &dsp{app: a, proj: e.proj, envOver: map[string]string{
		"HW_EXECUTOR_VENDOR": f.vendor, "HW_WORKDIR": e.workdir, "HW_PI_ASKED_MODEL": a.nextEnvVal(e.rundir, "HW_PI_ASKED_MODEL")}}}
	if f.vendor == "pi" && l.piPromptApplies(f.specialists) {
		f.piNote = l.piSpecialistNote(f.specialists)
	}
	piSkill, _ := l.piSkillDirFor("judgment-day")
	f.jdJudges, f.jdSkill, f.jdSkillPath = jdFacts(f.vendor, a.nextEnvVal(e.rundir, "CLAUDE_CONFIG_DIR"), a.getenv("HOME"), e.workdir, piSkill)
	f.fastGate = isFile(e.workdir + "/setup/test-hw")
	// DELEGATE BY MODEL TIER: only when the session being re-tasked was LAUNCHED with the explorer (its receipt says
	// so), so the prompt never names an agent that session does not have.
	if strings.HasPrefix(nextReceiptValue(e.rundir+"/receipt.jsonl", "explorer"), explorerName+" on ") {
		if t, ok := a.explorerFor(e.proj).tier(f.vendor); ok {
			f.explorer = &t
		}
	}
	f.model = nextRunningModel(e.rundir)
	if e.proj != "" {
		f.laneNote = bashSub(a.nextLaneField(e.proj, "brief_note"))
	}
	if f.laneNote != "" {
		f.laneQA = a.nextQAOverride(e)
	}
	f.qaTarget = e.proj != "" && a.nextLaneField(e.proj, "qa_target") != ""
	f.jd = e.baseRef != "" && e.reviewMode != "plugins" && e.reviewMode != "none" && f.build // # MUTATION-ANCHOR: 1020-G06
	f.designGate = nextDesignGate(e.brief)
	if f.designGate {
		f.boundary = nextFmValue("boundary", e.brief)
	}
	f.jdRequired = nextJudgmentDayRequired(e.brief)
	f.pluginsOnly = e.reviewMode == "plugins" && f.build // # MUTATION-ANCHOR: 1020-G07
	return f
}

// nextEngramSession is `jq -r 'select(.key=="engram_session") | .value' receipt.jsonl | tail -1`.
func nextEngramSession(path string) string { return nextReceiptValue(path, "engram_session") }

// nextReceiptValue is `jq -r 'select(.key==<key>) | .value' receipt.jsonl | tail -1`.
func nextReceiptValue(path, key string) string {
	vals, _ := readStream(path)
	var outs []string
	for _, v := range vals {
		if v.kind != vObj {
			if v.kind == vNull {
				continue
			}
			break // .key of a non-object: jq stops there
		}
		if k, ok := v.get("key"); ok && k.kind == vStr && k.s == key {
			val, ok := v.get("value")
			if !ok {
				outs = append(outs, "null")
				continue
			}
			outs = append(outs, val.raw())
		}
	}
	return bashSub(lastLine(outs))
}

// nextQAOverride is _brief_qa_override.
func (a *app) nextQAOverride(e nextMsgEnv) string {
	f := e.brief
	if f == "" || !isFile(f) {
		return ""
	}
	req := strings.ReplaceAll(nextFmValue("requires", f), " ", "")
	why := ""
	if strings.Contains(","+req+",", ",browser,") || strings.Contains(","+req+",", ",claude-in-chrome,") {
		why = "requires: " + req
	}
	if why == "" {
		dc := nextFmValue("deployed_check", f)
		if dc == "required" {
			why = "deployed_check: required"
		}
	}
	return why
}

// nextSpecialists is _specialist_agents: the *.md (any case) files and links under <dir>/.claude/agents, not
// hidden, by name without .md, minus the framework's own (sdd-, speckit-, review-, jd-), sorted, one line.
func nextSpecialists(dir string) string {
	root := dir + "/.claude/agents"
	if !isDir(root) {
		return ""
	}
	var names []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") || len(d.Name()) <= 3 {
			return nil
		}
		n := strings.TrimSuffix(d.Name(), ".md")
		for _, pre := range []string{"sdd-", "speckit-", "review-", "jd-"} {
			if strings.HasPrefix(n, pre) {
				return nil
			}
		}
		names = append(names, n)
		return nil
	})
	sort.Strings(names)
	return strings.Join(names, " ")
}
