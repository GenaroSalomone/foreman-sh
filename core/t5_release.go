package main

// release: prod — the dispatch half of the release standard (brain/setup/briefs/release-prod-standard.md). A brief that writes
// prod data or deploys to prod is dispatched only when staging and prod differ in nothing nobody ruled on, and is closed
// (done-invoker, core/invokers/release.go) only with its screens and its QA. The comparison is core/internal/releasediff.

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"hwcore/internal/releasediff"
)

func releaseList(v string) []string {
	return strings.Fields(strings.NewReplacer("[", "", "]", "", ",", " ").Replace(v))
}

// checkRelease is called after checkAuthorizes: it dies before anything is built.
func (d *dsp) checkRelease() {
	rel, reh := d.briefDecl("release"), d.briefDecl("rehearsal-of")
	if rel == "" && reh == "" {
		return
	}
	cfg := releasediff.FromEnv(d.c.brain)
	if reh != "" {
		d.checkRehearsal(cfg, reh)
	}
	if rel == "" {
		return
	}
	if rel != "prod" {
		d.die("the brief declares release: " + rel + " — the only value is prod. NOTHING was built.")
	}
	from, to := d.briefDecl("env-from"), d.briefDecl("env-to")
	diffDecl, screens, qa := d.briefDecl("env-diff"), releaseList(d.briefDecl("screens")), d.briefDecl("qa")
	for k, v := range map[string]string{"env-from": from, "env-to": to, "env-diff": diffDecl, "qa": qa} {
		if v == "" {
			d.die("release: prod requires " + k + ": in the brief's frontmatter (env-from, env-to, env-diff, screens, qa). NOTHING was built.")
		}
	}
	if len(screens) == 0 {
		d.die("release: prod requires screens: [<name>, …] — the views a person opens to call it done. NOTHING was built.")
	}
	for _, s := range screens {
		if !releasediff.ValidScreen(s) {
			d.die("screens: name " + s + " is not a relative path under $HW_ARTIFACTS/screens. NOTHING was built.")
		}
	}
	d.requireReleaseAuth(from, to)
	ruled, err := releasediff.Rulings(diffDecl)
	if err != nil {
		d.die(err.Error() + ". NOTHING was built.")
	}
	diff, err := releasediff.Diff(cfg, from, to)
	if err != nil {
		d.die("cannot compute the env diff: " + err.Error() + ". NOTHING was built; there is no names-only fallback.")
	}
	if err := releasediff.CheckDispatch(diff, ruled); err != nil {
		d.die(err.Error() + ". NOTHING was built.")
	}
	lane, task := d.proj, qa
	if l, t, ok := strings.Cut(qa, "/"); ok {
		lane, task = l, t
	}
	if !isFile(d.c.brain + "/" + lane + "/briefs/" + task + ".md") {
		d.die("qa: " + qa + " names no brief (looked for " + d.c.brain + "/" + lane + "/briefs/" + task + ".md). NOTHING was built.")
	}
	b, _ := json.Marshal(releasediff.Spec{From: from, To: to, Ruled: ruled, Screens: screens, QALane: lane, QATask: task, Start: time.Now().Unix()})
	d.releaseJSON, d.releaseQA = string(b), [2]string{lane, task}
	d.brAppendDeclared("release")
}

func (d *dsp) requireReleaseAuth(uuids ...string) {
	if !releasediff.Authorized(d.brAuthorizes, releasediff.FromEnv(d.c.brain).Host, uuids...) {
		d.die("the brief's authorizes: does not cover the env read (the SSH host, printenv in the backend containers, and both apps by uuid). Without it the diff cannot be computed, and names-only is not a fallback. NOTHING was built.")
	}
}

// checkRehearsal: a rehearsal of a release runs on an env that matches the release's target, or on one whose every difference was ruled not-needed.
func (d *dsp) checkRehearsal(cfg releasediff.Cfg, relTask string) {
	from := d.briefDecl("env-from")
	if from == "" {
		d.die("rehearsal-of: needs the rehearsal's own env-from: (the app it runs on). NOTHING was built.")
	}
	path := d.c.brain + "/" + d.proj + "/briefs/" + relTask + ".md"
	if !isFile(path) {
		d.die("rehearsal-of: " + relTask + " names no brief (" + path + "). NOTHING was built.")
	}
	var to, ruledDecl string
	for _, l := range strings.Split(brBriefDeclares(path), "\n") {
		k, v := brReadKV(l)
		switch k {
		case "env-to":
			to = v
		case "env-diff":
			ruledDecl = v
		}
	}
	if to == "" || ruledDecl == "" {
		d.die("rehearsal-of: " + relTask + " is not a release: prod brief (no env-to / env-diff). NOTHING was built.")
	}
	d.requireReleaseAuth(from, to)
	ruled, err := releasediff.Rulings(ruledDecl)
	if err != nil {
		d.die(err.Error() + " (in " + relTask + "). NOTHING was built.")
	}
	diff, err := releasediff.Diff(cfg, from, to)
	if err != nil {
		d.die("cannot compute the rehearsal's env diff: " + err.Error() + ". NOTHING was built.")
	}
	if err := releasediff.CheckRehearsal(diff, ruled); err != nil {
		d.die(err.Error() + ". NOTHING was built.")
	}
}

// qaState is the ledger's state for the QA task: never-dispatched, in-progress, done, or "" when it names nothing.
func (d *dsp) qaState(lane, task string) string {
	ldir := d.env("HW_LEDGER_DIR")
	if ldir == "" {
		ldir = nextOr(d.c.work, d.c.home+"/work") + "/.hw-ledger"
	}
	rows := ledgerCollect(pyRealpath(d.c.brain), ldir, []string{lane}, "", task, "")
	if len(rows) == 0 {
		return ""
	}
	return rows[0].state
}

// dispatchQA launches the release's QA brief beside it, unless it is already live or has reported.
func (d *dsp) dispatchQA() {
	if d.releaseJSON == "" {
		return
	}
	lane, task := d.releaseQA[0], d.releaseQA[1]
	switch st := d.qaState(lane, task); st {
	case "never-dispatched":
	case "in-progress":
		d.ok("QA " + lane + ":" + task + " is live — not dispatched again")
		return
	default: // done (a report from before this release does not count: the close wants one after its start) or unknown
		d.warn("QA " + lane + ":" + task + " is " + orDefaultStr(st, "not in the ledger (a _-prefixed brief is invisible to it)") + ": not dispatched. The release closes only on a QA report made after this dispatch — re-task or re-dispatch it.")
		return
	}
	if d.dispatchDry {
		d.ok("QA " + lane + ":" + task + " would be dispatched with this release (hw " + strings.Join(d.qaArgs(lane, task), " ") + ")")
		return
	}
	args := d.qaArgs(lane, task)
	cmd := exec.Command(d.c.brain+"/bin/hw", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		d.warn("the release launched but its QA (hw " + lane + " " + task + ") did not: " + err.Error() + ". The release cannot close until that QA reports done — dispatch it by hand.")
		return
	}
	d.ok("QA " + lane + ":" + task + " dispatched with this release")
}

// The release's flags its QA carries: what a person launching that QA brief by hand with them would type. Each takes a value
// except --fresh and --no-reuse-check. The rest are the release's own (placement, worktree, database, report, lease, reasons).
var qaCarriedValue = map[string]string{"--account": "", "--base": "base", "--sdd": "sdd", "--model": "model", "--effort": "effort", "--agent": "agent"} // flag → the brief key that declares it
var qaCarriedBool = map[string]bool{"--fresh": true, "--no-reuse-check": true}

// noteQACarry is called by parseFlags at every flag position: it remembers a carried flag and its value.
func (d *dsp) noteQACarry(rest []string) {
	if _, ok := qaCarriedValue[rest[0]]; ok && len(rest) > 1 {
		d.qaCarry = append(d.qaCarry, [2]string{rest[0], rest[1]})
	} else if qaCarriedBool[rest[0]] {
		d.qaCarry = append(d.qaCarry, [2]string{rest[0], ""})
	}
}

// qaArgs is the argument list of the QA's `hw`: lane, task, the release's carried flags, then --here and --requested-by as before.
// A flag whose key the QA brief declares with a different value is not carried: that declaration is the QA's own, and its
// contract check holds it. Without this the QA of a release run with --base main resolved the lane's base and was refused
// by the brief's own `base: main` (measured 2026-11-10).
func (d *dsp) qaArgs(lane, task string) []string {
	args := []string{lane, task}
	declared := map[string]string{}
	for _, l := range strings.Split(brBriefDeclares(d.c.brain+"/"+lane+"/briefs/"+task+".md"), "\n") {
		if k, v := brReadKV(l); k != "" {
			declared[k] = v
		}
	}
	for _, f := range d.qaCarry {
		if key := qaCarriedValue[f[0]]; key != "" {
			if v, ok := declared[key]; ok && v != f[1] {
				continue
			}
		}
		args = append(args, f[0])
		if !qaCarriedBool[f[0]] {
			args = append(args, f[1])
		}
	}
	if d.here {
		args = append(args, "--here")
	}
	if d.requestedBy != "" {
		args = append(args, "--requested-by", d.requestedBy)
	}
	return args
}
