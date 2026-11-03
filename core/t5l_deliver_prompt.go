package main

// Area C — the facts of the executor's prompt at launch (bin/hw: _specialist_agents, _brief_qa_override, _brief_fm_value).
// The text is t5l_prompt.go's: this file only says what hw knew when it wrote it.

import (
	"os"
	"strings"
)

// briefFmValue is _brief_fm_value: the frontmatter value of one key, "" when there is none.
func briefFmValue(key, path string) string {
	if !isFile(path) {
		return ""
	}
	out, _ := brExec("", "awk", "-v", "want="+key, `NR==1 { if ($0 != "---") exit; next }
       $0 == "---" { exit }
       tolower($0) ~ "^[[:space:]]*" want "[[:space:]]*:" {
         sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]+$/, "");
         if ($0 ~ /^".*"$/ || $0 ~ /^'.*'$/) $0 = substr($0, 2, length($0) - 2);
         print; exit }`, path)
	return bashSub(out)
}

// specialistAgents is _specialist_agents: the specialist subagents on disk in the work directory.
func (l *lch) specialistAgents(dir string) string {
	if st, err := os.Stat(dir + "/.claude/agents"); err != nil || !st.IsDir() {
		return ""
	}
	out, _ := l.sh(cmdSpec{name: "fd", args: []string{"-t", "f", "-t", "l", "-e", "md", ".", dir + "/.claude/agents"}, quiet: true})
	var names []string
	for _, ln := range strings.Split(out, "\n") {
		if ln == "" {
			continue
		}
		if i := strings.LastIndexByte(ln, '/'); i >= 0 { // sd '.*/' ''
			ln = ln[i+1:]
		}
		ln = strings.TrimSuffix(ln, ".md") // sd '\.md$' ''
		switch {                           // rg -v '^(sdd-|speckit-|review-|jd-)'
		case strings.HasPrefix(ln, "sdd-"), strings.HasPrefix(ln, "speckit-"), strings.HasPrefix(ln, "review-"), strings.HasPrefix(ln, "jd-"):
			continue
		}
		names = append(names, ln)
	}
	if len(names) > 1 {
		names = sortLocale(names, false)
	} else if len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Join(names, " "), " ")
}

// briefQaOverride is _brief_qa_override: what, if anything, in this brief asks for a browser or a deployed check.
func (l *lch) briefQaOverride() string {
	f := l.brief
	if !isFile(f) {
		return ""
	}
	why := ""
	req := strings.ReplaceAll(briefFmValue("requires", f), " ", "")
	if strings.Contains(","+req+",", ",browser,") || strings.Contains(","+req+",", ",claude-in-chrome,") {
		why = "requires: " + req
	}
	dc := briefFmValue("deployed_check", f)
	if why == "" {
		if dc == "required" {
			why = "deployed_check: required"
		}
	}
	return why
}

// orDefault2 is ${v:-def}.
func orDefault2(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// promptFacts is what the launch knew when it wrote the prompt: the preamble's facts in the order the old text read them
// (the calls the goldens record stay in that order), then the frame's.
func (l *lch) promptFacts() *promptFacts {
	f := &promptFacts{lane: l.proj, task: l.task, binDir: l.binDir(), brief: l.brief, label: orDefault2(l.env("ENGRAM_PROJECT"), "brain"), session: l.hwEngramSession, briefText: l.briefText}
	f.specialists = l.specialistAgents(l.workdir())
	if l.piPromptApplies(f.specialists) { // a pi executor cannot delegate to .claude/agents: it is told so (t6_pi_launch.go)
		f.piNote = l.piSpecialistNote(f.specialists)
	}
	f.vendor = l.env("HW_EXECUTOR_VENDOR")
	if f.vendor == "" {
		f.vendor = l.agent
	}
	if t, ok := l.explorer().tier(f.vendor); ok {
		f.explorer = &t
	}
	f.model = orDefault2(l.env("HW_PI_ASKED_MODEL"), l.model)
	if l.proj != "" {
		f.laneNote = bashSub(l.c.t.get(l.proj, "brief_note"))
	}
	if f.laneNote != "" {
		f.laneQA = l.briefQaOverride()
	}
	f.qaTarget = l.proj != "" && l.c.t.get(l.proj, "qa_target") != ""
	f.portWeb = l.env("HW_PORT_WEB")
	f.build = l.checksGateApplies()
	f.jd = l.judgmentDayApplies()
	piSkill, _ := l.piSkillDirFor("judgment-day")
	f.jdJudges, f.jdSkill, f.jdSkillPath = jdFacts(f.vendor, l.accountConfigDir, l.env("HOME"), l.workdir(), piSkill)
	f.fastGate = isFile(l.workdir() + "/setup/test-hw")
	f.reviewMode = l.reviewMode()
	f.designGate = l.designGateApplies()
	if f.designGate {
		f.boundary = l.briefBoundary()
	}
	f.jdRequired = l.reviewDeclared() && f.reviewMode == "judgment-day"
	f.plugins = l.pluginsDeclared()
	f.pluginsOnly = l.pluginsReviewApplies()
	f.secrets = l.brSecrets
	f.requestedBy = l.requestedBy
	if l.brAuthorizes != "" {
		f.authorizes = strings.Split(l.brAuthorizes, "\n")
	}
	f.operator = l.brOperator()
	return f
}
