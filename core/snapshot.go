package main

// The two reads `hw receipt` makes of a live pane: the report tokens it
// snapshots into receipt.jsonl (_snapshot_done_report, bin/hw:8576-8641) and the
// context size it prints (_pane_context and _ctx_line_for, bin/hw:10218 and
// 7710). Both go through the herdr CLI, as the bash does.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// bashSub is what `$(...)` does to a command's text: NULs are dropped and
// trailing newlines are stripped.
func bashSub(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimRight(s, "\n") // # MUTATION-ANCHOR: 816-G22
}

func readStream(path string) ([]*jval, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	vals, perr := parseStream(data)
	return vals, perr != nil
}

// lastLine is `tail -1` of newline-terminated outputs.
func lastLine(outs []string) string {
	if len(outs) == 0 {
		return ""
	}
	text := strings.Join(outs, "\n")
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		return text[i+1:]
	}
	return text
}

// paneOf is
//
//	jq -r 'select(.key=="pane") | .value' receipt.jsonl | tail -1
//
// The pane the run recorded, newest line, or "".
func paneOf(receipt string) string { return receiptLast(receipt, "pane") }

// receiptLast is
//
//	jq -r 'select(.key=="<key>") | .value' receipt.jsonl | tail -1
//
// the value the newest line of that key carries, as jq -r prints it, or "".
func receiptLast(receipt, key string) string {
	vals, _ := readStream(receipt)
	var outs []string
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			continue
		}
		if k.kind == vStr && k.s == key {
			val, ok := v.get("value")
			if !ok {
				continue
			}
			outs = append(outs, val.raw())
		}
	}
	return bashSub(lastLine(outs))
}

// hasReportSummary is
//
//	jq -e 'select(.key == "report_summary")' receipt.jsonl
//
// as a condition. jq -e keeps the status of the last input that produced an
// output or an error, and a syntax error ends it as an error.
func hasReportSummary(receipt string) bool {
	vals, perr := readStream(receipt)
	st := 4
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			st = 5
			continue
		}
		if k.kind == vStr && k.s == "report_summary" {
			st = 0
		}
	}
	if perr {
		st = 5
	}
	return st == 0
}

// herdrAgent runs `herdr agent <args>` and returns what it printed, whatever
// its status (the bash is `... 2>/dev/null || true`).
func herdrOut(timeout time.Duration, args ...string) (string, bool) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "herdr", args...)
	cmd.Env = childEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err == nil
}

// jqRaw is `jq -r '<path> // empty'` over a stream: the raw text of each
// output, errors skipped, none for null and false.
func jqRaw(text string, path ...string) []string {
	vals, _ := parseStream([]byte(text))
	var outs []string
	for _, v := range vals {
		cur, ok := v, true
		for _, p := range path {
			if cur, ok = cur.get(p); !ok {
				break
			}
		}
		if !ok || !cur.truthy() {
			continue
		}
		outs = append(outs, cur.raw())
	}
	return outs
}

func jqRawOr(text, def string, path ...string) string {
	outs := jqRaw(text, path...)
	if len(outs) == 0 {
		return def
	}
	return bashSub(strings.Join(outs, "\n"))
}

// receiptInto is _receipt_into (bin/hw:7172): append one fact with its source.
func (a *app) receiptInto(dir, key, val, src string) {
	if dir == "" || a.getenv("HW_CORE_READONLY") != "" { // the shim's shadow mode: bash already wrote this fact
		return
	}
	if src == "" {
		a.warn("internal: receipt '" + key + "' has no source; not recorded")
		return
	}
	if os.MkdirAll(dir, 0o777) != nil {
		return
	}
	obj := &jval{kind: vObj, obj: []member{
		{"key", jstr(key)}, {"value", jstr(strings.ToValidUTF8(val, "�"))}, {"source", jstr(src)},
		{"at", jstr(time.Now().UTC().Format("2006-01-02T15:04:05Z"))},
	}}
	f, err := os.OpenFile(filepath.Join(dir, "receipt.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(obj.toJSON() + "\n")
}

func digitsOnly(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// snapshotDoneReport persists the completion pointer a pane still carries.
func (a *app) snapshotDoneReport(rundir string) {
	if !isDir(rundir) {
		return
	}
	rc := rundir + "/receipt.jsonl"
	if readableFile(rc) && hasReportSummary(rc) {
		return
	}
	pane := paneOf(rc)
	if pane == "" {
		return
	}
	info, _ := herdrOut(0, "agent", "get", pane)
	info = bashSub(info)
	tokens := bashSub(strings.Join(jqTokens(info), "\n"))
	if tokens == "" {
		return
	}
	if jqRawOr(tokens, "", "done_status") == "" { // # MUTATION-ANCHOR: 816-G21
		return
	}
	tokenRun := jqRawOr(tokens, "", "hw_run")
	base := filepath.Base(rundir)
	if tokenRun != base { // # MUTATION-ANCHOR: 816-G19
		u := tokenRun
		if u == "" {
			u = "<unknown>"
		}
		a.warn("report tokens on " + pane + " belong to run " + u + ", not " + base + "; receipt unchanged")
		return
	}
	marker := rundir + "/done"
	seq := "1"
	if tb, err := os.ReadFile(rundir + "/task"); err == nil {
		seq = digitsOnly(string(tb))
		if seq == "" {
			seq = "1"
		}
		if seq != "1" {
			marker = rundir + "/t" + seq + "/done" // # MUTATION-ANCHOR: 816-G20
		}
	}
	if seq != "1" && !isFile(marker) {
		dat := doneAtEpoch(tokens)
		if st, err := os.Stat(rundir + "/task"); err == nil && dat != nil && *dat < st.ModTime().Unix() { // # MUTATION-ANCHOR: 816-G02
			a.receiptInto(rundir, "completion_marker",
				"absent — task "+seq+" has not reported; the pane's report tokens (done_at "+jqRawOrNull(tokens, "done_at")+") predate its dispatch, so they are an earlier task's and were not copied",
				"test ! -f "+marker+"; done_at token < mtime of "+rundir+"/task")
			return
		}
	}
	summary := summaryOf(tokens)
	src := func(what string) string { return "herdr agent get " + pane + " " + what }
	a.receiptInto(rundir, "report_project", jqRawOr(tokens, "unknown", "hw_project"), src("token hw_project"))
	a.receiptInto(rundir, "report_task", jqRawOr(tokens, "unknown", "hw_task"), src("token hw_task"))
	a.receiptInto(rundir, "report_run", tokenRun, "herdr agent get "+pane+" token hw_run matched receipt run directory")
	a.receiptInto(rundir, "report_summary", summary, "herdr agent get "+pane+" tokens sum/sum2/... before pane close")
	a.receiptInto(rundir, "report_status", jqRawOr(tokens, "", "done_status"), src("token done_status"))
	a.receiptInto(rundir, "report_state", jqRawOr(tokens, "unknown", "done_state"), src("token done_state"))
	a.receiptInto(rundir, "report_artifacts", jqRawOr(tokens, "unknown", "artifacts"), src("token artifacts"))
	a.receiptInto(rundir, "report_engram", jqRawOr(tokens, "unknown", "engram"), src("token engram"))
	if isFile(marker) {
		a.receiptInto(rundir, "completion_marker",
			"present — zero-byte flag means the receiver admitted the report (admission, not proof it was read); report content is in report_summary/status/artifacts/engram above",
			"test -f "+marker+" after done-invoker delivery accounting")
	} else {
		a.receiptInto(rundir, "completion_marker",
			"absent — report recovered from pane tokens, but receiver admission is not established",
			"test ! -f "+marker+" while snapshotting completion tokens")
	}
}

// jqTokens is  jq -c '.result.agent.tokens // {}'  over what herdr printed.
func jqTokens(info string) []string {
	vals, _ := parseStream([]byte(info))
	var outs []string
	for _, v := range vals {
		cur, ok := v, true
		for _, p := range []string{"result", "agent", "tokens"} {
			if cur, ok = cur.get(p); !ok {
				break
			}
		}
		if !ok {
			continue
		}
		if !cur.truthy() {
			outs = append(outs, "{}")
			continue
		}
		outs = append(outs, cur.toJSON())
	}
	return outs
}

// jqRawOrNull is `jq -r '.k'`: null reads as the word null.
func jqRawOrNull(text, k string) string {
	vals, _ := parseStream([]byte(text))
	var outs []string
	for _, v := range vals {
		if c, ok := v.get(k); ok {
			outs = append(outs, c.raw())
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// doneAtEpoch is  jq -r '.done_at // empty | fromdateiso8601? // empty'.
func doneAtEpoch(tokens string) *int64 {
	s := jqRawOr(tokens, "", "done_at")
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		return nil
	}
	e := t.Unix()
	return &e
}

// summaryOf is  jq -r '[.sum,.sum2,...,.sum7] | map(select(. != null)) | join("")'.
func summaryOf(tokens string) string {
	vals, _ := parseStream([]byte(tokens))
	var outs []string
	for _, v := range vals {
		var sb strings.Builder
		bad := false
		for _, k := range []string{"sum", "sum2", "sum3", "sum4", "sum5", "sum6", "sum7"} {
			c, ok := v.get(k)
			if !ok {
				bad = true
				break
			}
			switch c.kind {
			case vNull:
			case vStr, vNum, vBool:
				sb.WriteString(c.tostring())
			default:
				bad = true
			}
		}
		if !bad {
			outs = append(outs, sb.String())
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// ── context ─────────────────────────────────────────────────────────────────

var (
	reCtxLine = regexp.MustCompile(`Ctx: *[0-9]+(\.[0-9]+)?[kKmM]? *\|`)
	reCtxTok  = regexp.MustCompilePOSIX(`Ctx: *[0-9.]+[kKmM]?`)
	reCtxUsed = regexp.MustCompilePOSIX(`Ctx Used: *[0-9.]+`)
	reCtxLead = regexp.MustCompilePOSIX(`^Ctx: *`)
	reUsedLd  = regexp.MustCompilePOSIX(`^Ctx Used: *`)
)

// atof is awk's string-to-number: the longest numeric prefix, else 0.
func atof(s string) float64 {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	return f
}

// paneContext is _pane_context: "measured", "no-statusline" or "unreadable",
// never a guess. The read is bounded by HW_CTX_READ_TIMEOUT (default 3 s).
func (a *app) paneContext(pane string) (state, tok, pct string) {
	secs := 3
	if v, ok := a.lookupEnv("HW_CTX_READ_TIMEOUT"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(digitsPrefix(v)))
		if err != nil {
			n = 0
		}
		secs = n
	}
	out, ok := herdrOut(time.Duration(secs)*time.Second, "agent", "read", pane, "--source", "visible")
	out = bashSub(out)
	if !ok || out == "" { // # MUTATION-ANCHOR: 816-G31
		return "unreadable", "", ""
	}
	out = strings.ReplaceAll(out, "\u00a0", " ") // # MUTATION-ANCHOR: 816-G03
	line := ""
	for _, l := range strings.Split(out, "\n") {
		if reCtxLine.MatchString(l) { // # MUTATION-ANCHOR: 816-G01
			line = l
		}
	}
	if line == "" {
		return "no-statusline", "", ""
	}
	t := reCtxTok.FindString(line)
	p := reCtxUsed.FindString(line)
	if t != "" {
		t = reCtxLead.ReplaceAllString(t, "")
	}
	if p != "" {
		p = reUsedLd.ReplaceAllString(p, "")
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(t, "k") || strings.HasSuffix(t, "K"):
		mult = 1000
	case strings.HasSuffix(t, "m") || strings.HasSuffix(t, "M"):
		mult = 1000000
	}
	t = strings.TrimRight(t, "kKmM")
	if t == "" {
		return "no-statusline", "", ""
	}
	return "measured", strconv.FormatInt(int64(atof(t)*mult+0), 10), p
}

func digitsPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func (a *app) lookupEnv(k string) (string, bool) {
	v := a.getenv(k)
	return v, v != ""
}

// ctxLineFor is _ctx_line_for: one line, no colour.
func (a *app) ctxLineFor(pane string) string {
	state, tok, pct := a.paneContext(pane)
	if state != "measured" {
		return "context: not measured (" + state + ")"
	}
	s := "context: " + tok + " tokens used"
	if pct != "" {
		s += " (" + pct + "% of the window)"
	}
	// `[ "$tok" -le "$HW_CTX_PREFER_FRESH_TOKENS" ] || printf ...`: a threshold
	// that is not an integer makes the test an error, and the arrow prints.
	thr := a.getenv("HW_CTX_PREFER_FRESH_TOKENS")
	if thr == "" {
		thr = "150000"
	}
	tn, err1 := strconv.ParseInt(tok, 10, 64)
	th, err2 := strconv.ParseInt(strings.TrimSpace(thr), 10, 64)
	if err1 != nil || err2 != nil || tn > th { // # MUTATION-ANCHOR: 816-G25
		s += " → prefer a fresh executor over hw next or a ruling"
	}
	return s
}

// receiptContextLine is _receipt_context_line: silent without a recorded pane.
func (a *app) receiptContextLine(receipt string) {
	pane := paneOf(receipt)
	if pane == "" {
		return
	}
	a.printf("    %s%s  (pane %s, read now)%s\n\n", a.cDim, a.ctxLineFor(pane), pane, a.c0)
}
