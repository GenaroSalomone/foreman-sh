package invokers

import (
	"fmt"
	"regexp"
	"strings"
)

// THE JUDGMENT DAY PROTOCOL, ENFORCED FROM THE TRANSCRIPT (pi-adopt-gentle, 2026-11-05; reference: gentle-pi's
// review-policy-judgment-day.ts, a frozen ledger, a re-judgment that cannot add findings, a budget of rounds and judge runs).
// JudgmentDayGate proves the judges were CALLED; this proves they were called within the protocol the skill states:
//
//   - budget: at most jdMaxJudgeRounds two-judge calls (the first judgment and the skill's two scoped re-judgments) and
//     jdMaxJudgeRuns judge runs in all (those six plus a coverage relaunch's judges). A third re-judgment is a loop the skill forbids
//     ("never reset or extend an exhausted round budget").
//   - frozen ledger: the FIRST round's findings are the ledger. A later two-judge round (a scoped re-judgment) may report a finding
//     only at a file the ledger names, or its own prompt names (the prompt carries the ledger and the fix delta: a fix that moved
//     code reports it at its new file), or an earlier re-judgment recorded as caused by the fix; anything else is a NEW finding,
//     which a re-judgment may not add unless the judge marks it `"causal_disposition":"fix-caused"`. A round that launched ONE
//     judge is a coverage relaunch of the first judgment (its missing paths): it extends the ledger and is not a round.
//
// A finding is read off the judges' own results: the `"location"` of a findings object, or the path in a markdown ledger row
// (`| JD-001 | core/a.go:10 | CRITICAL | open | … |`, the shape ~/.claude/agents/jd-judge-*.md asks for and judges answer in). The judges number their rows from JD-001
// each and the orchestrator renumbers when it merges them, so an id is NOT compared (it would refuse a legitimate re-judgment).
// Results with no location are not judged by this gate (it says so in its note): it refuses what it can read, never what it cannot.
//
// BOTH breaches are a refusal unless the report says ESCALATED: a re-judgment that already added a finding, or a budget already
// spent, cannot be undone in an append-only transcript, and the skill's own exit for that is an ESCALATED verdict naming what is open.
const (
	jdMaxJudgeRuns   = 8
	jdMaxJudgeRounds = 3
)

var (
	jdLocationRe  = regexp.MustCompile(`"location"\s*:\s*"([^"]+)"`)
	jdMdRowRe     = regexp.MustCompile(`(?m)^[ \t]*\|.*\bJD-\d+\b.*$`) // a row of the markdown ledger the judge contract asks for: | JD-001 | path:line | SEVERITY | status | evidence |
	jdIDCellRe    = regexp.MustCompile("^[ \t*`]*JD-\\d+[ \t*`]*$")
	jdFixCausedRe = regexp.MustCompile("(?i)\\|[ \t*`]*fix-caused[ \t*`]*(?:\\||$)|causal_disposition[\"'`*]*[ \t]*[:=][ \t]*[\"'`*]*fix-caused")
	jdEscalatedRe = regexp.MustCompile(`\bESCALATED\b`)
	jdApprovedRe  = regexp.MustCompile(`\bAPPROVED\b`)
	jdNegatedRe   = regexp.MustCompile(`(?i)\b(?:not|no|none|nothing|never|without)\s+ESCALATED\b`)
	jdPathRe      = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(?:go|ts|mjs|js|py|sh|md|json|txt|tsv|yml|yaml)\b`)
)

// jdFindingKey is the file a location names, lowercase, without line numbers or a leading ./ .
func jdFindingKey(loc string) string {
	loc = strings.TrimPrefix(strings.TrimSpace(loc), "./")
	if i := strings.Index(loc, ":"); i > 0 {
		loc = loc[:i]
	}
	return strings.ToLower(loc)
}

// jdKeys are the finding keys of one text: every path-like token in it, by file (a prompt's ledger and fix delta).
func jdKeys(text string) []string {
	var out []string
	for _, m := range jdPathRe.FindAllString(text, -1) {
		out = append(out, jdFindingKey(m))
	}
	return out
}

type jdFinding struct {
	key       string
	fixCaused bool
}

// jdFindingsOf is what one round's text reports: each findings object's file, and whether the object marks it fix-caused
// (anywhere in the object: the fields come in any order and `evidence` can be long).
func jdFindingsOf(text string) []jdFinding {
	out := jdMarkdownFindingsOf(text)
	locs := jdLocationRe.FindAllStringSubmatchIndex(text, -1)
	for i, m := range locs {
		start, end := strings.LastIndex(text[:m[0]], "{"), len(text)
		if start < 0 || (i > 0 && start < locs[i-1][1]) { // not inside its own object: from the end of the previous row
			start = m[0]
			if i > 0 {
				start = locs[i-1][1]
			}
		}
		if j := strings.Index(text[m[1]:], "}"); j >= 0 {
			end = m[1] + j
		}
		if i+1 < len(locs) && locs[i+1][0] < end { // the next row opens before this one closes: stop at it
			end = locs[i+1][0]
		}
		out = append(out, jdFinding{key: jdFindingKey(text[m[2]:m[3]]), fixCaused: strings.Contains(text[start:end], "fix-caused")})
	}
	return out
}

// jdMarkdownFindingsOf reads the ledger rows a judge answers in markdown: a table row holding a JD-NNN id, whose location is the
// CELL right after the id, keyed like a JSON location (the whole token, so an extensionless `bin/hw:20` is read). A cell with no
// `/`, `.` or `:` in it is not a location (a column in another order is never guessed at: the row is skipped). A row is one line;
// it is fix-caused when a cell IS `fix-caused` or says `causal_disposition: fix-caused` (the word inside free prose, "not
// fix-caused", marks nothing).
func jdMarkdownFindingsOf(text string) []jdFinding {
	var out []jdFinding
	for _, row := range jdMdRowRe.FindAllString(text, -1) { // # MUTATION-ANCHOR: 1250-G03
		cells := strings.Split(row, "|")
		for i, c := range cells {
			if !jdIDCellRe.MatchString(c) {
				continue
			}
			if i+1 < len(cells) {
				if tok := strings.Fields(strings.Trim(cells[i+1], " \t*`")); len(tok) > 0 && strings.ContainsAny(tok[0], "/.:") {
					out = append(out, jdFinding{key: jdFindingKey(strings.Trim(tok[0], "*`")), fixCaused: jdFixCausedRe.MatchString(row)})
				}
			}
			break
		}
	}
	return out
}

// reportsEscalated is a summary whose verdict is ESCALATED: the word is there, not negated ("nothing ESCALATED"), and the same
// report does not also say APPROVED (judges A and B, round 2: any mention of the word used to unlock a breach).
func reportsEscalated(summary string) bool {
	if jdApprovedRe.MatchString(summary) {
		return false
	}
	return jdEscalatedRe.MatchString(jdNegatedRe.ReplaceAllString(summary, ""))
}

// JudgmentDayProtocolGate returns a refusal when the judges' calls broke the protocol (unless the summary reports ESCALATED),
// and a note (possibly empty) otherwise.
func JudgmentDayProtocolGate(proof *TranscriptProof, summary string) (note string, err error) {
	if proof == nil || len(proof.JudgeRounds) == 0 {
		return "", nil
	}
	runs, rounds := 0, 0
	for _, r := range proof.JudgeRounds {
		runs += r.Runs
		if r.Runs >= 2 {
			rounds++
		}
	}
	escalated := reportsEscalated(summary)
	breach := func(msg string) (string, error) {
		if escalated {
			return "\njudgment day protocol: " + msg + " — accepted because the report says ESCALATED", nil
		}
		return "", die("%s", msg+" Report what is left as ESCALATED (the verdict, naming the findings still open) or --blocked. NOTHING was sent or marked.")
	}
	if runs > jdMaxJudgeRuns || rounds > jdMaxJudgeRounds { // # MUTATION-ANCHOR: 1250-G01
		return breach(fmt.Sprintf("this task's Judgment Day ran %d judge runs in %d two-judge rounds, over the protocol's budget (%d runs, %d rounds: the first judgment and two scoped re-judgments); a round budget is never reset or extended.", runs, rounds, jdMaxJudgeRuns, jdMaxJudgeRounds))
	}
	ledger := map[string]bool{}
	read := false
	for i, r := range proof.JudgeRounds {
		found := jdFindingsOf(r.Text)
		if len(found) > 0 {
			read = true
		}
		if i == 0 || r.Runs < 2 { // the first judgment, or a coverage relaunch of it: they WRITE the ledger
			for _, f := range found {
				ledger[f.key] = true
			}
			continue
		}
		named := map[string]bool{} // the files this re-judgment was handed: the ledger and the fix delta
		for _, k := range jdKeys(r.Prompt) {
			named[k] = true
		}
		var added []string
		for _, f := range found {
			if f.fixCaused {
				ledger[f.key] = true // a later re-judgment verifies it
				continue
			}
			if !ledger[f.key] && !named[f.key] { // # MUTATION-ANCHOR: 1250-G02
				added = append(added, f.key)
			}
		}
		if len(added) > 0 {
			return breach(fmt.Sprintf("Judgment Day re-judgment (call %d) added finding(s) that are not in the frozen ledger of the first judgment and not in what it was handed: %s. A scoped re-judgment verifies each ledger entry and may record a defect the FIX caused (mark it \"causal_disposition\":\"fix-caused\"), and nothing else.", i+1, strings.Join(added, ", ")))
		}
	}
	if !read {
		return "\njudgment day protocol: not checked — the judges' results carry no readable findings (no \"location\" fields, no markdown ledger rows); only the call budget was held", nil
	}
	return "", nil
}
