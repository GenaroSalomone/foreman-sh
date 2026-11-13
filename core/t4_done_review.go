package main

// The merge gate of `hw done` (review-by-default design §3.4 «The merge gate», §6 «Brief 4», §8 C1 C2 C4).
//
// «Merge it» is printed only when one of these holds at the branch head as it is NOW:
//
//	no code        0 code-only lines (the measure of `hw review`, recomputed here, never read from a receipt)
//	review-skip    the build's brief says `review: none` and gives a `review-skip:` reason
//	approved       the reviewer's review.md says `verdict: approve  head: <sha>` and <sha> is the head
//	accepted       `hw done … --accept-review "<why>"`, a brainer's override, refused inside an executor
//
// Everything else is «NOT MERGEABLE»: the absence of a verdict (hw review never ran, failed, or the reviewer left no review.md)
// is a denial, never «nothing owed» (C1). Owed follows the diff, not the brief's kind (C2): any branch with code-only lines owes
// the review or a recorded skip, and a review run's own branch is never offered. On a product lane the PR's head must be the
// reviewed SHA (C4), and `gh pr ready <n>` is printed only for a draft PR at that head. The gh calls are read-only.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// reviewVerdictRe is the first line of review.md. Strict on purpose: the template's own «approve | changes-needed» is no verdict.
var reviewVerdictRe = regexp.MustCompile(`^verdict:\s*(approve|changes-needed)\s+head:\s*([0-9a-f]{7,40})\s*$`)

// reviewVerdictOf parses review.md's first line.
func reviewVerdictOf(text string) (verdict, sha string, ok bool) {
	first, _, _ := strings.Cut(text, "\n")
	m := reviewVerdictRe.FindStringSubmatch(strings.TrimSpace(first))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// reviewSameSHA is true when the reviewed SHA (full or abbreviated, at least 7) is the head.
func reviewSameSHA(reviewed, head string) bool {
	return len(reviewed) >= 7 && head != "" && strings.HasPrefix(head, reviewed)
}

func reviewShort(sha string) string {
	if len(sha) > reviewShortLen {
		return sha[:reviewShortLen]
	}
	return sha
}

// mergeGate is the answer: allow prints «Merge it», otherwise deny is the first line of NOT MERGEABLE and why/how follow it.
type mergeGate struct {
	allow    bool
	never    bool     // the branch is a review run's own: never offered
	notes    []string // what held, one line each
	deny     string
	why, how []string
	readyPR  int // > 0: print `gh pr ready <n>`
	accepted string
}

// ghPR is what `gh pr view <branch> --json number,isDraft,headRefOid` said.
type ghPR struct {
	state   string // "ok", "none" (the branch has no PR) or "unknown" (gh could not say)
	number  int
	draft   bool
	oid     string
	problem string
}

// reviewPRof reads the branch's PR, read-only. A branch with no PR is "none"; any other failure is "unknown", which the
// gate reads as a denial (it must never fail open on a PR it could not read).
func (d *doneRun) reviewPRof(main, branch string) ghPR {
	gh := d.getenv("HW_GH")
	if gh == "" {
		gh = "gh"
	}
	ghPath, err := doneLookPath(gh)
	if err != nil {
		return ghPR{state: "unknown", problem: "gh is not installed or not on PATH"}
	}
	argv := []string{"pr", "view", branch, "--json", "number,isDraft,headRefOid"}
	if p, err := doneLookPath("timeout"); err == nil {
		argv = append([]string{ghPath}, argv...)
		ghPath, argv = p, append([]string{"60"}, argv...)
	}
	r := extRun(extCall{Dir: main, Name: ghPath, Args: argv, Merge: true})
	if r.RC != 0 {
		if strings.Contains(strings.ToLower(r.Out), "no pull requests found") {
			return ghPR{state: "none"}
		}
		return ghPR{state: "unknown", problem: "`gh pr view " + branch + "` failed (" + strconv.Itoa(r.RC) + "): " + strings.TrimSpace(firstLine(strings.TrimSpace(r.Out)))}
	}
	n, _ := strconv.Atoi(jqRawOr(r.Out, "", "number"))
	oid := jqRawOr(r.Out, "", "headRefOid")
	if n == 0 || oid == "" {
		return ghPR{state: "unknown", problem: "`gh pr view " + branch + "` answered with no number or headRefOid"}
	}
	return ghPR{state: "ok", number: n, draft: len(jqRaw(r.Out, "isDraft")) > 0, oid: oid}
}

// reviewGate decides. accept is the `--accept-review` reason ("" when not given). rundir is the run being closed.
func (d *doneRun) reviewGate(proj, task, main, wt, branch, rundir, accept string) mergeGate {
	a := d.app
	g := mergeGate{}
	headOut, rc := doneGit(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	head := bashSub(headOut)
	if rc != 0 || head == "" {
		g.deny = "NOT MERGEABLE: " + branch + " does not resolve in " + main
		return g
	}
	runDir := a.reviewRunDir(wt)
	if runDir == "" {
		runDir = strings.TrimSuffix(rundir, "/")
	}
	brief := a.reviewBuildBrief(proj, task)

	// C2: a review run's own branch is never offered for merge, whatever its diff
	if of := nextFmValue("review_of", brief); of != "" {
		g.never, g.deny = true, "NOT OFFERED: "+branch+" is a review run (review_of "+of+"): it is never merged. Merge the build it reviewed."
		return g
	}
	if runDir != "" {
		if of, _ := doneReceiptLast(runDir+"/receipt.jsonl", "review_of_t1"); of != "" {
			g.never, g.deny = true, "NOT OFFERED: "+branch+" is a review run (review_of_t1 "+of+"): it is never merged. Merge the build it reviewed."
			return g
		}
	}

	// C1: recompute at the head now. A measure that cannot be made is an owed review, not a free pass.
	owed, measured := true, ""
	if base, _ := a.reviewBaseOf(main, proj, head, brief, runDir); base == "" {
		measured = "could not measure the diff: no merge-base with the lane's base and no review_base in the receipt"
	} else if num, ok := gitRun(main, nil, "diff", "--no-renames", "--numstat", "-z", base+".."+head); !ok {
		measured = "could not measure the diff: `git diff --numstat " + reviewShort(base) + ".." + reviewShort(head) + "` failed"
	} else if code, _, _ := reviewMeasure(string(num)); code == 0 {
		owed = false
		g.notes = append(g.notes, "review: none owed — no code changed (0 code-only lines at "+reviewShort(head)+")")
	} else {
		measured = strconv.Itoa(code) + " code-only line(s) at " + reviewShort(head)
	}

	approved := false // the approval itself is the evidence, so the PR must agree with it
	switch {
	case !owed:
		g.allow = true
	case reviewSkipOf(brief) != "":
		g.allow = true
		g.notes = append(g.notes, "review: none — review-skip recorded in "+brief+": "+reviewSkipOf(brief))
	case d.reviewVerdictCheck(&g, proj, task, head, runDir, measured) == "":
		g.allow, approved = true, true
		g.notes = append(g.notes, "review: approved at "+reviewShort(head)+" ("+measured+")")
	}

	// C4: on a product lane the PR is what GitHub shows, so its head must be the bytes that were judged
	pr := ghPR{state: "none"}
	if a.c.t.get(proj, "base_ref_prefix") != "" && (g.allow || accept != "") {
		pr = d.reviewPRof(main, branch)
		switch {
		case !approved:
		case pr.state == "unknown":
			g.allow = false
			g.deny = "NOT MERGEABLE: review approved at " + reviewShort(head) + " but the PR's head could not be compared"
			g.why = []string{"the PR could not be read: " + pr.problem}
		case pr.state == "ok" && !reviewSameSHA(pr.oid, head):
			g.allow = false
			g.deny = "NOT MERGEABLE: review approved at " + reviewShort(head) + ", PR #" + strconv.Itoa(pr.number) + " headRefOid is " + reviewShort(pr.oid)
			g.why = []string{"GitHub shows " + reviewShort(pr.oid) + "; the review judged " + reviewShort(head) + ". Push the branch (or fetch it) until they agree, then re-run `hw review`."}
		}
	}

	if !g.allow {
		g.how = []string{
			"`hw review " + proj + " " + task + "` dispatches the independent review; `hw next` re-tasks the builder after changes-needed.",
			"A brainer, never an executor, may override: `hw done " + proj + " " + task + " --accept-review \"<why>\"` (recorded in the receipt).",
		}
		if accept != "" {
			g.allow, g.accepted = true, accept
			g.notes = append(g.notes, "review ACCEPTED by --accept-review: "+accept+" (overrides: "+strings.TrimPrefix(g.deny, "NOT MERGEABLE: ")+")")
			if runDir != "" {
				a.receiptInto(runDir, "review_accepted", accept, "hw done "+proj+" "+task+" --accept-review at "+head+" — overrides: "+g.deny)
			}
			g.deny, g.why, g.how = "", nil, nil
		}
	}
	if g.allow && pr.state == "ok" && pr.draft {
		g.readyPR = pr.number
	}
	return g
}

// reviewSkipOf is the brief's `review-skip:` reason when its `review:` is exactly `none` (the one recorded opt-out), else "".
func reviewSkipOf(brief string) string {
	if brief == "" || reviewRawOf(nextFmValue("review", brief)) != "none" {
		return ""
	}
	return strings.TrimSpace(nextFmValue("review-skip", brief))
}

// reviewVerdictCheck reads the reviewer's review.md and returns "" when it approves the head, else fills g.deny/g.why and says why.
func (d *doneRun) reviewVerdictCheck(g *mergeGate, proj, task, head, runDir, measured string) string {
	a := d.app
	deny := func(verdict, at, why string) string {
		g.deny = "NOT MERGEABLE: review " + verdict + " at " + at + ", head is " + reviewShort(head)
		g.why = []string{why + " (owed: " + measured + ")"}
		return why
	}
	run := ""
	if runDir != "" {
		run, _ = doneReceiptLast(runDir+"/receipt.jsonl", "review_run")
	}
	if run == "" {
		return deny("none", "—", "no review was dispatched: `hw review` never recorded a review_run for "+proj+":"+task+" (it failed, or was never run)")
	}
	rtask := strings.TrimSuffix(doneBasename(run), ".md")
	rwt := a.workTree(proj, rtask)
	art := a.reviewArtifactsOf(rwt)
	if art == "" {
		art = rwt + "/.artifacts"
	}
	raw, err := os.ReadFile(art + "/review.md")
	if err != nil {
		return deny("none", "—", "the review run "+rtask+" has left no review.md (looked in "+art+")")
	}
	verdict, sha, ok := reviewVerdictOf(string(raw))
	if !ok {
		return deny("unreadable", "—", art+"/review.md does not start with `verdict: approve|changes-needed  head: <sha>`")
	}
	if verdict != "approve" || !reviewSameSHA(sha, head) {
		return deny(verdict, reviewShort(sha), "the latest review.md is "+art+"/review.md")
	}
	return ""
}
