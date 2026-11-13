package main

// `hw review <lane> <task> [--dry-run] [--here] [--at <sha>]`: the independent review of a build (design: brain/archive/setup/
// review-by-default-design/design.md §2, §3, §6 «Brief 1», §8 C1 C3 S1 S2). It measures the build's diff, picks the
// tier, writes the reviewer's brief from docs/REVIEW-BRIEF.md and dispatches a fresh claude opus executor on a branch
// cut at the reviewed SHA. It is a brainer verb: an executor reaches only its `--dry-run`.
//
//	tier 0   no code-only line changed        no reviewer; review_head is recorded all the same
//	tier S   1..400 code-only lines           thermo, test-review
//	tier L   more than 400                    + ponytail-review, pstack:principle-minimize-reader-load, pstack:principle-subtract-before-you-add
//	+JD      the build brief's review: names judgment-day (high risk, declared by the brainer)   the reviewer also runs Judgment Day
//
// What the build's receipt gains: review_head and review_tier always, review_run (the reviewer's brief) when one was
// dispatched. `--at <sha>` is the exception: a blind review of a past head, for benchmarking, records nothing in the build's
// receipt, so its verdict can never satisfy `hw done`'s merge gate. review_owed=1 is the build dispatch's (t5l_env.go): a failed `hw review` leaves it standing, so the
// absence of a verdict is never read as "nothing owed" (C1).

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// reviewSmallMax is the largest diff (code-only lines) that gets the lean pass: above setup's p90 (375), below a product lane's median (665).
const reviewSmallMax = 400

// reviewDocOnly is the S2 allowlist: the paths whose change is prose a reader reads and nothing runs. Everything else is
// code, including `.claude/**`, `_agents/`, `_skills/`, `plugins/`, `CLAUDE.md`, `*.golden` and `testdata/`: a markdown file
// an agent obeys is executable. The two templates under docs/ are executable too.
func reviewDocOnly(path string) bool {
	switch path {
	case "docs/REVIEW-BRIEF.md", "docs/BRIEF-TEMPLATE.md":
		return false
	}
	if strings.HasPrefix(path, "docs/") || strings.HasPrefix(path, "setup/export/overlay/changes/") {
		return true
	}
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1 && i < 2; i++ { // a directory at the root or one lane below it: setup/decisions/, <lane>/briefs/
		switch parts[i] {
		case "decisions", "briefs", "changelog":
			return true
		}
	}
	return false
}

// reviewMeasure reads `git diff --no-renames --numstat -z` and counts code-only lines. A binary file (`-`) counts as owed:
// one line, so it can never leave the diff at tier 0.
func reviewMeasure(numstatZ string) (code int, codeFiles []string, docLines int) {
	for _, rec := range strings.Split(numstatZ, "\x00") {
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			continue
		}
		n := 1
		if f[0] != "-" || f[1] != "-" {
			a, e1 := strconv.Atoi(f[0])
			d, e2 := strconv.Atoi(f[1])
			if e1 != nil || e2 != nil {
				continue
			}
			n = a + d
		}
		if reviewDocOnly(f[2]) {
			docLines += n
			continue
		}
		code += n
		codeFiles = append(codeFiles, f[2])
	}
	return code, codeFiles, docLines
}

// reviewTierOf is 0, S or L by code-only lines.
func reviewTierOf(code int) string {
	switch {
	case code <= 0:
		return "0"
	case code <= reviewSmallMax:
		return "S"
	}
	return "L"
}

// reviewTierList is the `review:` list of the reviewer's brief: the plugins its tier loads, then judgment-day when the build declared it.
func reviewTierList(tier string, jd bool) []string { return reviewTierListWith(tier, jd, nil) }

// reviewTierListWith adds the plugin items the build's `review:` named (S4: reviewExtraPlugins), once each, before judgment-day.
func reviewTierListWith(tier string, jd bool, extra []string) []string {
	var out []string
	switch tier {
	case "S":
		out = []string{"thermo", "tests"}
	case "L":
		out = []string{"thermo", "tests", "ponytail", "pstack"}
	default:
		return nil
	}
	for _, n := range extra {
		if !has(out, n) {
			out = append(out, n)
		}
	}
	if jd {
		out = append(out, "judgment-day")
	}
	return out
}

// reviewerPieces is the pieces a reviewer proves, by full name, from its `review:` list. In a review_of brief `pstack` stands
// for its two principles (a Skill call each, like every other piece), not for the four reviewers it carries on a build.
func reviewerPieces(list []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, it := range list {
		switch it {
		case "judgment-day":
		case "pstack":
			add("pstack:principle-minimize-reader-load")
			add("pstack:principle-subtract-before-you-add")
		default:
			for _, q := range reviewSkillsQualified([]string{it}) {
				add(q)
			}
		}
	}
	return out
}

// reviewShortLen is the SHA prefix a reviewer's task name carries; herdr caps a task name at 32 characters.
const (
	reviewShortLen = 7
	reviewTaskMax  = 32
)

// reviewTaskName is `<task>-review-<short sha>`, the build's name cut so the whole fits the cap. The SHA keeps it unique
// per dispatch (C3) however much of the name goes.
func reviewTaskName(task, short string) string { return reviewTaskNameOf(task, "-review-", short) }

// reviewAtTaskName is `<task>-review-at-<short sha>`: a past head's reviewer, which never collides with the review of the same SHA as a tip.
func reviewAtTaskName(task, short string) string { return reviewTaskNameOf(task, "-review-at-", short) }

func reviewTaskNameOf(task, infix, short string) string {
	suffix := infix + short
	if room := reviewTaskMax - len(suffix); len(task) > room {
		task = strings.TrimRight(task[:room], "-")
	}
	return task + suffix
}

// reviewTierLabel is what review_tier holds: `S`, `L+JD`, `0`.
func reviewTierLabel(tier string, jd bool) string {
	if jd && tier != "0" {
		return tier + "+JD"
	}
	return tier
}

// ── the functions a diff changed (§3.5) ─────────────────────────────────────

var (
	reviewHunkRe = regexp.MustCompile(`^@@ [^@]* @@ ?(.*)$`)
	reviewDefRes = []*regexp.Regexp{
		regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][A-Za-z0-9_$]*)`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`),
	}
)

// reviewDefName is the function a line defines, "" when it defines none.
func reviewDefName(line string) string {
	for _, re := range reviewDefRes {
		if m := re.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

// reviewFunctionsOf reads `git diff -U0` and lists "name\tpath", once each, sorted: the function a hunk header names (the
// attributes file hands git a driver per language) and the definitions the hunk adds. A name hw cannot extract is not owed.
func reviewFunctionsOf(diff string) []string {
	seen := map[string]bool{}
	file := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if line == "+++ /dev/null" {
				file = ""
			}
		case strings.HasPrefix(line, "@@"):
			if m := reviewHunkRe.FindStringSubmatch(line); m != nil && file != "" {
				if n := reviewDefName(m[1]); n != "" {
					seen[n+"\t"+file] = true
				}
			}
		case strings.HasPrefix(line, "+") && file != "":
			if n := reviewDefName(line[1:]); n != "" {
				seen[n+"\t"+file] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// reviewJSDriver is the funcname pattern of the one language git has no driver for; the .go and .py ones are git's own.
const reviewJSDriver = `^[ \t]*((export[ \t]+)?(async[ \t]+)?function[ \t*].*|(export[ \t]+)?(const|let|var)[ \t]+[A-Za-z_$][A-Za-z0-9_$]*[ \t]*=.*=>.*|class[ \t].*)$`

// ── the verb ────────────────────────────────────────────────────────────────

type reviewPlan struct {
	lane, task, branch, head, base, baseHow string
	wt                                      string
	code                                    int
	codeFiles                               []string
	docLines                                int
	tier                                    string
	jd                                      bool
	list, pieces, extra                     []string
	rtask, rbranch, rbase                   string
	briefPath                               string
	functions                               []string
	buildBrief, requestedBy                 string
	buildRunDir                             string
	dirty                                   string
	at                                      bool // --at: a past head; the build's receipt is not touched
}

const reviewUsage = "hw review <lane> <task> [--dry-run] [--here] [--no-report] [--at <sha>]"

func (a *app) cmdReview() int {
	dry, here, noReport := false, false, false
	at := ""
	var pos []string
	for i := 0; i < len(a.args); i++ {
		x := a.args[i]
		switch {
		case x == "--at" || strings.HasPrefix(x, "--at="):
			if x == "--at" && i+1 < len(a.args) {
				i++
				at = a.args[i]
			} else {
				at = strings.TrimPrefix(x, "--at=")
			}
			if at == "" || strings.HasPrefix(at, "-") {
				a.die("hw review: --at needs a commit (usage: " + reviewUsage + ")")
			}
			continue
		}
		switch x {
		case "--dry-run":
			dry = true
		case "--here":
			here = true
		case "--no-report": // dispatch's own flag, carried: a rehearsal from a shell that is no brainer's pane has nowhere to report
			noReport = true
		default:
			if strings.HasPrefix(x, "-") {
				a.die("hw review: unknown option " + x + " (usage: " + reviewUsage + ")")
			}
			pos = append(pos, x)
		}
	}
	if len(pos) != 2 {
		a.die("usage: " + reviewUsage)
	}
	p := a.reviewPlan(pos[0], pos[1], at)
	a.reviewPrint(p, dry)

	tmp, err := os.MkdirTemp("", "hw-review-")
	if err != nil {
		a.die("hw review: cannot make a scratch directory: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	if p.tier == "0" {
		if p.at {
			a.ok("no code changed at " + p.head + ": no reviewer dispatched, and nothing recorded (--at).")
			return 0
		}
		if dry {
			a.info("dry run: would record review_head=" + p.head + " and review_tier=0 in the build's receipt; no reviewer.")
			return 0
		}
		a.reviewRecord(p, false)
		a.ok("no code changed: review_head=" + p.head + " recorded, no reviewer dispatched.")
		return 0
	}

	brief, err := a.reviewBriefText(p)
	if err != nil {
		a.die("hw review: " + err.Error())
	}
	path := p.briefPath
	if dry { // a dry run creates nothing in the brain: the dispatch's own check reads a scratch copy
		path = filepath.Join(tmp, p.rtask+".md")
	} else if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		a.die("hw review: " + err.Error())
	}
	if err := os.WriteFile(path, []byte(brief), 0o666); err != nil {
		a.die("hw review: cannot write the reviewer's brief " + path + ": " + err.Error())
	}
	args := []string{p.lane, p.rtask, "--agent", "claude", "--model", "opus", "--effort", "high", "--base", p.rbase, "--brief", path}
	if !noReport { // --keep-pane needs a report to chain after, so a no-report rehearsal drops it
		args = append(args, "--keep-pane", "--why", "the re-check: the brainer re-tasks this reviewer with hw next after the builder's one fix round (review-by-default design §3.4)")
	}
	if here {
		args = append(args, "--here")
	}
	if noReport {
		args = append(args, "--no-report")
	}
	if dry {
		args = append(args, "--dry-run")
	}
	a.step("dispatch  hw " + strings.Join(args, " "))
	cmd := exec.Command(a.c.brain+"/bin/hw", args...)
	cmd.Stdout, cmd.Stderr = a.out, a.err
	if err := cmd.Run(); err != nil {
		if p.at { // a past head never touches the build's receipt: do not claim anything about it
			a.die("the reviewer's dispatch failed (" + err.Error() + "): this was a review of a past head, so the build's receipt is untouched and nothing was recorded. Fix the cause above and run `hw review " + p.lane + " " + p.task + " --at " + p.head + "` again.")
		}
		a.die("the reviewer's dispatch failed (" + err.Error() + "): review_owed=1 still stands in the build's receipt and nothing else was recorded. Fix the cause above and run `hw review " + p.lane + " " + p.task + "` again.")
	}
	if dry {
		return 0
	}
	if !p.at {
		a.reviewRecord(p, true)
	}
	a.reviewWriteFunctions(p)
	return 0
}

// reviewPlan measures the build and fixes everything the dispatch will say, or dies. A non-empty `at` is --at: the head is that
// commit instead of the branch tip, there is no origin comparison (the reviewer is cut at the SHA itself) and no receipt to touch.
func (a *app) reviewPlan(laneArg, task, at string) *reviewPlan {
	lane := a.c.t.canonProject(laneArg)
	if !a.c.t.known(lane) {
		a.die("hw review: " + laneArg + " is not a lane in projects.json")
	}
	if strings.ContainsAny(task, "/ ") || task == "" {
		a.die("hw review: a task name has no path separator or space (got: " + task + ")")
	}
	checkout := a.c.checkout(lane)
	if checkout == "" || !isDir(checkout) {
		a.die("hw review: " + lane + " has no checkout to read a branch from")
	}
	p := &reviewPlan{lane: lane, task: task}
	p.branch = a.c.lanePath(lane, "branch", task)
	if p.branch == "" {
		p.branch = "task/" + task
	}
	p.wt = a.workTree(lane, task)
	// the head: the branch tip, which is what the worktree holds when the build committed; under --at, the commit asked for
	if at != "" {
		out, ok := gitRun(checkout, nil, "rev-parse", "--verify", "--quiet", at+"^{commit}")
		if !ok {
			a.die("hw review: --at " + at + " is not a commit in " + checkout + " — nothing to review (it is cut locally: no fetch, no push)")
		}
		p.head, p.at = strings.TrimSpace(string(out)), true
	} else {
		out, ok := gitRun(checkout, nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+p.branch+"^{commit}")
		if !ok {
			a.die("hw review: " + lane + ":" + task + " has no branch " + p.branch + " in " + checkout + " — nothing to review (a merged and reaped task is gone)")
		}
		p.head = strings.TrimSpace(string(out))
	}
	if !p.at && (isDir(p.wt+"/.git") || isFile(p.wt+"/.git")) {
		if st, ok := gitRun(p.wt, nil, "status", "--porcelain"); ok && strings.TrimSpace(string(st)) != "" {
			p.dirty = strconv.Itoa(len(strings.Split(strings.TrimSpace(string(st)), "\n"))) + " path(s) uncommitted in " + p.wt + ": the review judges the committed head only"
		}
	}
	prefix := a.c.t.get(lane, "base_ref_prefix")
	p.rbase = p.head
	if prefix != "" && !p.at { // C3: the reviewer is cut from the PR branch, so the PR branch must BE the head being stamped
		ref := prefix + "task/" + task
		r, ok := gitRun(checkout, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		remote := strings.TrimSpace(string(r))
		if !ok || remote != p.head {
			shown := "missing"
			if ok {
				shown = remote
			}
			a.die("hw review: " + ref + " is " + shown + " but the local head of " + p.branch + " is " + p.head + ". The reviewer is cut from " + ref + " (the PR branch), so push the branch, or fetch it, until they agree: a review stamped with a SHA the reviewer's tree is not at is not a review of that SHA.")
		}
		p.rbase = "task/" + task
	}
	p.buildBrief = a.reviewBuildBrief(lane, task)
	if nextFmValue("review_of", p.buildBrief) != "" {
		a.die("hw review: " + task + " is itself a review run (its brief declares review_of): a review is not reviewed, and its branch is never offered for merge")
	}
	p.requestedBy = nextFmValue("requested_by", p.buildBrief)
	review := reviewRawOf(nextFmValue("review", p.buildBrief))
	mode, _ := reviewModeOf(review)
	p.jd = mode == "judgment-day"
	p.buildRunDir = a.reviewRunDir(p.wt)

	base, how := a.reviewBaseOf(checkout, lane, p.head, p.buildBrief, p.buildRunDir)
	if base == "" {
		a.die("hw review: no base to measure " + p.branch + " against (no review_base in its receipt and no merge-base with " + prefix + a.c.t.get(lane, "base") + ")")
	}
	p.base, p.baseHow = base, how

	num, _ := gitRun(checkout, nil, "diff", "--no-renames", "--numstat", "-z", base+".."+p.head)
	p.code, p.codeFiles, p.docLines = reviewMeasure(string(num))
	p.tier = reviewTierOf(p.code)
	p.extra = reviewExtraPlugins(review, pluginsOfValue(nextDecl(nextBriefDeclares(p.buildBrief), "plugins")))
	p.list = reviewTierListWith(p.tier, p.jd, p.extra)
	p.pieces = reviewerPieces(p.list)
	short := p.head
	if len(short) > reviewShortLen {
		short = short[:reviewShortLen]
	}
	p.rtask = reviewTaskName(task, short)
	if p.at {
		p.rtask = reviewAtTaskName(task, short)
	}
	p.rbranch = a.c.lanePath(lane, "branch", p.rtask)
	if p.rbranch == "" {
		p.rbranch = "task/" + p.rtask
	}
	p.briefPath = a.c.brain + "/" + lane + "/briefs/" + p.rtask + ".md"
	if p.tier != "0" {
		p.functions = a.reviewFunctions(checkout, p)
	}
	return p
}

// reviewBaseOf is the base a branch's diff is measured from: the merge-base with the lane's base NOW (a branch that merged main in
// has a later fork point than the one recorded at its dispatch, and base..head must not count main's commits). Once the branch is
// merged that is the head itself, and the merge-base recorded at the build's dispatch, while it is still an ancestor of the head, is
// the only fork point left. "" when there is none. `hw done` measures with it too, so the two never disagree about what is code.
func (a *app) reviewBaseOf(checkout, lane, head, buildBrief, runDir string) (base, how string) {
	prefix := a.c.t.get(lane, "base_ref_prefix")
	lb := nextFmValue("base", buildBrief)
	if lb == "" {
		lb = a.c.t.get(lane, "base")
	}
	for _, ref := range []string{prefix + lb, lb} {
		if m, ok := gitRun(checkout, nil, "merge-base", head, ref); ok && strings.TrimSpace(string(m)) != "" {
			base, how = strings.TrimSpace(string(m)), "git merge-base "+ref+" at the head"
			break
		}
	}
	if (base == "" || base == head) && runDir != "" {
		if b, _ := doneReceiptLast(runDir+"/receipt.jsonl", "review_base"); b != "" {
			if _, ok := gitRun(checkout, nil, "merge-base", "--is-ancestor", b, head); ok {
				base, how = b, "review_base of the build's receipt — the branch is already in "+lb+", so its merge-base now is the head"
			}
		}
	}
	return base, how
}

// reviewBuildBrief is brain/<lane>/briefs/<task>.md, "" when the build had none.
func (a *app) reviewBuildBrief(lane, task string) string {
	if p := a.c.brain + "/" + lane + "/briefs/" + task + ".md"; isFile(p) {
		return p
	}
	return ""
}

// reviewRunDir is the newest run directory of a worktree that holds a receipt.
func (a *app) reviewRunDir(wt string) string {
	for _, r := range runsNewestFirst(wt + "/.hw") {
		if isFile(wt + "/.hw/" + r + "/receipt.jsonl") {
			return wt + "/.hw/" + r
		}
	}
	return ""
}

// reviewFunctions is `git diff -U0` of the code files with the brain's attributes file, read-only on the product repo.
func (a *app) reviewFunctions(checkout string, p *reviewPlan) []string {
	if len(p.codeFiles) == 0 {
		return nil
	}
	args := []string{"-c", "core.attributesFile=" + a.c.brain + "/setup/review/gitattributes", "-c", "diff.jsfn.xfuncname=" + reviewJSDriver,
		"diff", "-U0", "--no-renames", "--no-color", p.base + ".." + p.head, "--"}
	args = append(args, p.codeFiles...)
	out, ok := gitRun(checkout, nil, args...)
	if !ok {
		return nil
	}
	return reviewFunctionsOf(string(out))
}

func (a *app) reviewPrint(p *reviewPlan, dry bool) {
	a.step("hw review " + p.lane + ":" + p.task + func() string {
		if dry {
			return "  (dry run)"
		}
		return ""
	}())
	a.printf("  build        %s @ %s\n", p.branch, p.head)
	if p.at {
		a.printf("  past head    --at %s: a benchmark review cut locally (no push, no origin ref); the build's receipt is NOT touched (no review_head, review_tier or review_run), so this verdict never satisfies `hw done`'s merge gate\n", p.head)
	}
	if p.dirty != "" {
		a.warn(p.dirty)
	}
	a.printf("  base         %s  (%s)\n", p.base, p.baseHow)
	a.printf("  diff         %d code-only line(s) in %d file(s); %d doc-only line(s) not counted\n", p.code, len(p.codeFiles), p.docLines)
	a.printf("  tier         %s  (0: none · S: 1-%d · L: more)%s\n", reviewTierLabel(p.tier, p.jd), reviewSmallMax, func() string {
		if p.jd && p.tier == "0" {
			return " — the brief lists judgment-day, but no code changed, so there is nothing to judge"
		}
		return ""
	}())
	if p.tier == "0" {
		return
	}
	a.printf("  pieces       %s\n", strings.Join(p.pieces, ", "))
	if p.jd {
		a.printf("  + JD         the build brief's review: names judgment-day: the reviewer orchestrates the two judges\n")
	}
	if len(p.extra) > 0 {
		a.printf("  + plugins    the build's review: adds %s to the reviewer's tier\n", strings.Join(p.extra, ", "))
	}
	a.printf("  reviewer     task %s on %s, cut from %s (the %s)\n", p.rtask, p.rbranch, p.rbase, func() string {
		if p.rbase == p.head {
			return "reviewed SHA"
		}
		return "PR branch, which is the reviewed SHA"
	}())
	a.printf("  brief        %s\n", p.briefPath)
	a.printf("  functions    %d changed function(s) listed into the reviewer's $HW_ARTIFACTS/functions.txt\n", len(p.functions))
}

// reviewRecord appends the build's receipt lines. review_owed=1 is the build dispatch's own and is left alone.
func (a *app) reviewRecord(p *reviewPlan, dispatched bool) {
	dir := p.buildRunDir
	if dir == "" {
		a.warn("no run directory with a receipt under " + p.wt + "/.hw: review_head / review_tier were NOT recorded")
		return
	}
	src := "hw review " + p.lane + " " + p.task + " — the branch tip " + p.branch + " measured against " + p.base
	if dispatched {
		a.receiptInto(dir, "review_run", p.briefPath, "hw review dispatched "+p.lane+":"+p.rtask+" with this brief")
	}
	a.receiptInto(dir, "review_head", p.head, src)
	a.receiptInto(dir, "review_tier", reviewTierLabel(p.tier, p.jd), src+" — "+strconv.Itoa(p.code)+" code-only line(s)")
}

// reviewArtifactsOf is the $HW_ARTIFACTS of a worktree's newest run, read off its env file; "" when there is none.
func (a *app) reviewArtifactsOf(wt string) string {
	run := a.reviewRunDir(wt)
	if run == "" {
		return ""
	}
	b, err := os.ReadFile(run + "/env")
	if err != nil {
		return ""
	}
	if m := regexp.MustCompile(`(?m)^HW_ARTIFACTS='(.*)'$`).FindStringSubmatch(string(b)); m != nil {
		return m[1]
	}
	return ""
}

// reviewWriteFunctions puts functions.txt in the reviewer's $HW_ARTIFACTS, read off its run's env file.
func (a *app) reviewWriteFunctions(p *reviewPlan) {
	rwt := a.workTree(p.lane, p.rtask)
	art := a.reviewArtifactsOf(rwt)
	if art == "" {
		a.warn("functions.txt was NOT written: the reviewer's run directory has no HW_ARTIFACTS (looked in " + rwt + "/.hw). The brief carries the same list.")
		return
	}
	if err := os.MkdirAll(art, 0o777); err != nil {
		a.warn("functions.txt was NOT written: " + err.Error())
		return
	}
	body := strings.Join(p.functions, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(art+"/functions.txt", []byte(body), 0o666); err != nil {
		a.warn("functions.txt was NOT written: " + err.Error())
		return
	}
	a.ok("functions.txt: " + strconv.Itoa(len(p.functions)) + " function(s) in " + art)
}

// ── the reviewer's brief ────────────────────────────────────────────────────

// reviewBriefText fills docs/REVIEW-BRIEF.md: its frontmatter and body carry {{key}} slots, and nothing else is rewritten.
func (a *app) reviewBriefText(p *reviewPlan) (string, error) {
	tpl, err := os.ReadFile(a.c.brain + "/docs/REVIEW-BRIEF.md")
	if err != nil {
		return "", err
	}
	fns := "(hw extracted none: read the diff and list every function you judged)"
	if len(p.functions) > 0 {
		fns = ""
		for _, f := range p.functions {
			n, path, _ := strings.Cut(f, "\t")
			fns += "- `" + n + "` — " + path + "\n"
		}
		fns = strings.TrimRight(fns, "\n")
	}
	skills := ""
	for _, q := range p.pieces {
		skills += "- `" + q + "`\n"
	}
	jd := ""
	if p.jd {
		jd = "This build is declared high risk (its brief lists `judgment-day`: money, mail or live data). After the pieces above, run Judgment Day over the same range with the `judgment-day` skill, once, and fold its confirmed findings into `review.md`."
	}
	asked := "investigate"
	rb := p.requestedBy
	if rb == "" {
		rb = "hw review " + p.lane + " " + p.task
	}
	r := strings.NewReplacer(
		"{{review_list}}", strings.Join(p.list, ", "),
		"{{review_of}}", p.lane+":"+p.task+"@"+p.head,
		"{{lane}}", p.lane, "{{task}}", p.task, "{{head}}", p.head, "{{base}}", p.base,
		"{{tier}}", reviewTierLabel(p.tier, p.jd), "{{code_lines}}", strconv.Itoa(p.code),
		"{{build_brief}}", orDefaultStr(p.buildBrief, "(the build had no brief on disk)"),
		"{{skills}}", strings.TrimRight(skills, "\n"), "{{jd}}", jd, "{{functions}}", fns,
		"{{requested_by}}", strings.ReplaceAll(rb, `"`, `'`), "{{asked}}", asked,
	)
	return r.Replace(string(tpl)), nil
}
