package main

// hw executor-turn-end (bin/hw cmd_executor_turn_end): what every executor's
// Stop hook runs at every turn end. The legality of ending a turn is a set of
// disk facts (a report, a delivered ask, a human boundary, a live child) and
// this is the one place that reads them. It also writes the counters and
// markers the rest of hw reads: `turns`, `turns-post-report`,
// `handback-refused`, `ruling-delivered.*`, and the herdr tokens the cockpit
// shows. Exit 8 refuses the turn end; the Stop hook turns it into a block.
//
// The helpers that stay Python (holdfacts, cockpit-state) are run as bash runs
// them; the herdr token publication and the engram lease renewal, which bash
// reaches through herdr-rpc and curl, are native here.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (t *t3) operator() string {
	if v := t.getenv("HW_OPERATOR"); v != "" {
		return v
	}
	if data, err := os.ReadFile(t.c.t.file); err == nil {
		if vals, perr := parseStream(data); perr == nil && len(vals) > 0 {
			if o, ok := vals[0].get("operator"); ok && o.truthy() {
				return o.raw()
			}
		}
	}
	return "the operator"
}

func (t *t3) turnReason() string {
	return "This executor turn has no legal handoff fact on disk — going quiet hands reversible work back. Continue working now, report with `done-invoker`, or report the blocker with `done-invoker --blocked \"<question and what would unblock it>\"`. If the next action crosses a genuine human boundary, record it first with `hw human-boundary \"<the boundary, and why only " + t.operator() + " can decide>\"`."
}

// t3PyJSONStr is json.dumps(s) as Python writes it for a string read from the
// environment: every non-ASCII character as \uXXXX, and a byte that is not
// UTF-8 as the lone surrogate \udcXX that surrogateescape gives it.
func t3PyJSONStr(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && sz == 1 {
			sb.WriteString(`\u` + hex4s(0xdc00+int(s[i])))
			i++
			continue
		}
		i += sz
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
		case r < 0x20 || (r >= 0x7f && r < 0x10000):
			sb.WriteString(`\u` + hex4s(int(r)))
		case r >= 0x10000:
			r -= 0x10000
			sb.WriteString(`\u` + hex4s(0xd800+int(r>>10)) + `\u` + hex4s(0xdc00+int(r&0x3ff)))
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func (t *t3) blockJSON(reason string) {
	t.printf("{\"decision\":\"block\",\"reason\":%s}\n", t3PyJSONStr(reason))
}

// ── live children: _executor_live_children ───────────────────────────────────

// t3LiveChildren is the jq program of _executor_live_children over HW_TURN_END_PAYLOAD:
// "<n> <types>" when a background child has status "running", else "". Only the
// literal "running" counts. A payload jq cannot read yields "" (the bash is `|| true`).
func t3LiveChildren(payload string) string {
	if payload == "" {
		return ""
	}
	vals, _ := parseStream([]byte(payload))
	var outs []string
	for _, v := range vals {
		bt, ok := v.get("background_tasks")
		if !ok {
			break
		}
		if !bt.truthy() {
			continue // `// []`
		}
		var elems []*jval
		switch bt.kind {
		case vArr:
			elems = bt.arr
		case vObj:
			for _, m := range bt.obj {
				elems = append(elems, m.v)
			}
		default:
			return strings.Join(outs, "\n")
		}
		var types []*jval
		n := 0
		for _, e := range elems {
			if e.kind != vObj {
				continue // `.status?` errors are swallowed: "" is not "running"
			}
			st, _ := e.get("status")
			if !(st.kind == vStr && st.s == "running") { // # MUTATION-ANCHOR: 841-C01
				continue
			}
			n++
			ty, _ := e.get("type")
			if !ty.truthy() {
				ty = jstr("task") // # MUTATION-ANCHOR: 841-C02
			}
			types = append(types, ty)
		}
		if n == 0 {
			continue
		}
		joined, ok := t3UniqueJoin(types)
		if !ok {
			return strings.Join(outs, "\n")
		}
		outs = append(outs, strconv.Itoa(n)+" "+joined)
	}
	return bashSub(strings.Join(outs, "\n"))
}

// t3UniqueJoin is `unique | join(",")` over strings, numbers and true (jq's
// order: true < numbers < strings); an array or an object makes join fail.
func t3UniqueJoin(vs []*jval) (string, bool) {
	type item struct {
		rank int
		num  float64
		s    string
	}
	var items []item
	for _, v := range vs {
		switch v.kind {
		case vBool:
			items = append(items, item{1, 0, "true"})
		case vNum:
			f, _ := strconv.ParseFloat(v.s, 64)
			items = append(items, item{2, f, v.raw()})
		case vStr:
			items = append(items, item{3, 0, v.s})
		default:
			return "", false
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.rank == 2 {
			return a.num < b.num
		}
		return a.s < b.s
	})
	var out []string
	for i, it := range items {
		if i > 0 && it.rank == items[i-1].rank && it.s == items[i-1].s {
			continue
		}
		out = append(out, it.s)
	}
	return strings.Join(out, ","), true
}

// ── a hold: _executor_pending_reply ──────────────────────────────────────────

// pendingReply is the first valid `pending-reply-N` hold of the task dir whose
// intent is answer or ruling: "<intent>\t<file>", or "".
func (t *t3) pendingReply(dir string) string {
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "pending-reply-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		file := dir + "/" + n
		if !isFile(file) {
			continue
		}
		if !t.replyFactsValid(file) { // # MUTATION-ANCHOR: 841-T07
			continue
		}
		body, _ := os.ReadFile(file)
		intent := ""
		for _, line := range strings.Split(string(body), "\n") {
			k, v, found := strings.Cut(line, "=")
			if k == "intent" {
				if found {
					intent = v
				} else {
					intent = line
				}
				break
			}
		}
		// holdfacts check (above) already refuses any other intent; this repeats bash's own case, and no input reaches it
		if intent == "answer" || intent == "ruling" {
			return intent + "\t" + file
		}
	}
	return ""
}

// replyFactsValid is _executor_reply_facts_valid: bin/holdfacts check, exit 0 valid.
func (t *t3) replyFactsValid(file string) bool {
	cmd := exec.Command(t.binDir()+"/holdfacts", "check", file, "--run", t.getenv("HW_RUN"), "--pane", t.getenv("HERDR_PANE_ID"), "--vendor", t.getenv("HW_EXECUTOR_VENDOR"))
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	rc := exitStatus(cmd.Run())
	if rc > 1 {
		t.warnRaw("hw: holdfacts did not run for " + file + " (exit " + strconv.Itoa(rc) + "): " + bashSub(buf.String()))
	}
	return rc == 0
}

// warnRaw is `printf '…\n' >&2`: hw's own plain stderr line.
func (t *t3) warnRaw(s string) { io.WriteString(t.err, s+"\n") }

// ── publication: _executor_turn_publish ──────────────────────────────────────

func (t *t3) ctxJSON() string {
	vendor := t.getenv("HW_EXECUTOR_VENDOR")
	if vendor == "" {
		vendor = "claude"
	}
	payload := t.getenv("HW_TURN_END_PAYLOAD")
	if vendor != "claude" || payload == "" {
		return ""
	}
	tp := jqRawOr(payload, "", "transcript_path")
	bin := t.binDir() + "/cockpit-state"
	if tp == "" || !t3IsExec(bin) {
		return ""
	}
	out, _ := exec.Command(bin, "ctx", tp).Output()
	return bashSub(string(out))
}

func t3IsExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

type rpcReq struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// t3HerdrCall is `herdr-rpc call <method> <params>`, its answer ignored: one
// newline-delimited JSON request on the socket, and the wait for its response.
func t3HerdrCall(sock, method string, params any, timeout time.Duration) {
	if sock == "" {
		return
	}
	conn, err := dialHerdr(sock, timeout)
	if err != nil {
		return
	}
	defer conn.Close()
	line, _ := json.Marshal(rpcReq{ID: "1", Method: method, Params: params})
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(conn)
		for {
			l, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(l)) > 0 {
				var o struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(l, &o) == nil && o.ID == "1" {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// t3JSONStr is jq's string encoding: raw UTF-8, only the quote, the backslash and the control characters escaped.
func t3JSONStr(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// t3JSONObj is an object of already-encoded values, in the order given.
func t3JSONObj(kv [][2]string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range kv {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(t3JSONStr(p[0]) + ":" + p[1])
	}
	b.WriteByte('}')
	return b.String()
}

func (t *t3) publish(state string, turns int64, extraName, extraValue string) {
	if t.getenv("HERDR_ENV") != "1" || t.getenv("HERDR_PANE_ID") == "" || t.fs.readonly {
		return
	}
	ctx := t.ctxJSON()
	proj := t.getenv("HW_PROJECT")
	if proj == "" {
		proj = "unknown"
	}
	task := t.getenv("HW_TASK")
	if task == "" {
		task = "unknown"
	}
	run := t.getenv("HW_RUN")
	if run == "" {
		run = "unknown"
	}
	// the wire order of bash's jq program: pane_id, source, ttl_ms, tokens{hw_project, hw_task, hw_run, turn_state,
	// turn_ended_at, turns, the extra, ctx_pct, ctx_tokens} — a herdr reads it either way, the bytes are the contract
	tokens := [][2]string{
		{"hw_project", t3JSONStr(proj)},                    // # MUTATION-ANCHOR: 841-U01
		{"hw_task", t3JSONStr(task)},                       // # MUTATION-ANCHOR: 841-U02
		{"hw_run", t3JSONStr(run)},                         // # MUTATION-ANCHOR: 841-U03
		{"turn_state", t3JSONStr(state)},                   // # MUTATION-ANCHOR: 841-U04
		{"turn_ended_at", t3JSONStr(t3IsoNow())},           // # MUTATION-ANCHOR: 841-U05
		{"turns", t3JSONStr(strconv.FormatInt(turns, 10))}, // # MUTATION-ANCHOR: 841-U06
	}
	if extraName != "" {
		tokens = append(tokens, [2]string{extraName, t3JSONStr(extraValue)}) // # MUTATION-ANCHOR: 841-U07
	}
	if ctx != "" {
		if vals, err := parseStream([]byte(ctx)); err == nil && len(vals) == 1 && vals[0].truthy() {
			c := vals[0]
			pct, ok1 := c.get("pct")
			tok, ok2 := c.get("tokens")
			if !ok1 || !ok2 { // jq errors on `.pct` of a non-object: the whole params are lost
				return
			}
			tokens = append(tokens, [2]string{"ctx_pct", t3JSONStr(pct.raw())})    // # MUTATION-ANCHOR: 841-U08
			tokens = append(tokens, [2]string{"ctx_tokens", t3JSONStr(tok.raw())}) // # MUTATION-ANCHOR: 841-U09
		}
	}
	params := json.RawMessage(t3JSONObj([][2]string{
		{"pane_id", t3JSONStr(t.getenv("HERDR_PANE_ID"))}, // # MUTATION-ANCHOR: 841-U10
		{"source", t3JSONStr("hw:executor-turn-end")},     // # MUTATION-ANCHOR: 841-U11
		{"ttl_ms", "86400000"},                            // # MUTATION-ANCHOR: 841-U12
		{"tokens", t3JSONObj(tokens)},
	}))
	t3HerdrCall(t.getenv("HERDR_SOCKET_PATH"), "pane.report_metadata", params, 10*time.Second)
	// The cockpit shows this turn end: one debounced write (bin/cockpit-state --kick).
	t.executorCockpitKick() // # MUTATION-ANCHOR: 841-T08
}

func (t *t3) executorCockpitKick() {
	bin := t.binDir() + "/cockpit-state"
	if !t3IsExec(bin) {
		return
	}
	if p := t.getenv("HW_INVOKER_PANE"); p != "" {
		exec.Command(bin, "--kick", "--invoker", p).Run()
	} else if d, ok := t.executorStateDir(); ok {
		exec.Command(bin, "--kick", "--rundir", d).Run()
	}
}

// ── the engram lease: _engram_renew_session ─────────────────────────────────

// receiptValues is `jq -r 'select(.key=="<key>") | .value' receipt.jsonl`: the values of that key, in order, up to the
// first record jq cannot index.
func receiptValues(stateDir, key string) []string {
	vals, _ := readStream(stateDir + "/receipt.jsonl")
	var outs []string
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			break
		}
		if k.kind == vStr && k.s == key {
			val, _ := v.get("value")
			outs = append(outs, val.raw())
		}
	}
	return outs
}

func (t *t3) engramRenew(stateDir string) {
	if t.fs.readonly {
		return
	}
	id := lastLine(receiptValues(stateDir, "engram_session"))
	if !strings.HasPrefix(id, "hw-") {
		return
	}
	project, dir := t.getenv("ENGRAM_PROJECT"), t.getenv("HW_WORKDIR")
	if project == "" || dir == "" {
		return
	}
	t.engramLeasePost(id, project, dir)
}

// engramLeasePost is engram_serve_verdict then engram_session_post: renew the session's 30-minute lease, on a serve
// proven to be this machine's store, and never say anything.
func (t *t3) engramLeasePost(id, project, dir string) {
	if t.fs.readonly {
		return
	}
	if st, err := os.Stat(t.binDir() + "/engram-serve.sh"); err != nil || st.IsDir() {
		return
	}
	url := t.getenv("HW_ENGRAM_URL")
	if url == "" {
		port := t.getenv("ENGRAM_PORT")
		if port == "" {
			port = "7437"
		}
		url = "http://127.0.0.1:" + port
	}
	if !t.engramServeIsOurs(url) {
		return
	}
	body, _ := json.Marshal(map[string]string{"id": id, "project": project, "directory": dir})
	cl := &http.Client{Timeout: 5 * time.Second}
	if resp, err := cl.Post(url+"/sessions", "application/json", bytes.NewReader(body)); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// engramServeIsOurs is engram_serve_verdict returning 0: the serve on the port is
// this machine's store (its /health instance_id is the .instance-id of the data dir).
func (t *t3) engramServeIsOurs(url string) bool {
	dir := t.getenv("ENGRAM_DATA_DIR")
	if strings.TrimSpace(dir) == "" {
		dir = t.getenv("HOME") + "/.engram"
	}
	want := ""
	if b, err := os.ReadFile(dir + "/.instance-id"); err == nil {
		want = strings.Join(strings.Fields(string(b)), "")
	}
	cl := &http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	b, _ := io.ReadAll(resp.Body)
	got := jqRawOr(string(b), "", "instance_id")
	return want != "" && got != "" && got == want
}

// ── the verb ─────────────────────────────────────────────────────────────────

func t3Atoi(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimLeft(s, "0"), 10, 64)
	return v
}

// piTurnEndForeign is _pi_turn_end_foreign: whether this turn end names ANOTHER session than the one hw registered.
// pi's adapter names its session in the payload (the session file herdr publishes as agent_session and hw records as
// `session_id`); a pi-subagents child is a session of the same process with the same environment, so it must not be judged
// as the executor. ONLY A REGISTERED SESSION NARROWS IT: a receipt with no session id keeps the old behaviour.
func (t *t3) piTurnEndForeign(stateDir string) bool {
	if t.getenv("HW_EXECUTOR_VENDOR") != "pi" || t.getenv("HW_TURN_END_PAYLOAD") == "" { // # MUTATION-ANCHOR: 986-T02
		return false
	}
	payload := jqRawOr(t.getenv("HW_TURN_END_PAYLOAD"), "", "session_id")
	if payload == "" { // # MUTATION-ANCHOR: 986-T06
		return false
	}
	registered, _ := reviveReceiptLast(stateDir+"/receipt.jsonl", "session_id")
	if registered == "" { // # MUTATION-ANCHOR: 986-T03
		return false
	}
	if strings.HasPrefix(registered, "none") { // # MUTATION-ANCHOR: 986-T05
		return false
	}
	if payload == registered {
		return false
	}
	if payload[strings.LastIndexByte(payload, '/')+1:] == registered[strings.LastIndexByte(registered, '/')+1:] { // # MUTATION-ANCHOR: 986-T04
		return false
	}
	return true
}

func (t *t3) executorTurnEnd() int {
	hookJSON := false
	args := t.args
	if len(args) > 0 && args[0] == "--hook-json" {
		hookJSON, args = true, args[1:]
	}
	if len(args) != 0 {
		t.die("usage: hw executor-turn-end [--hook-json]")
	}
	stateDir, ok := t.executorStateDir()
	if t.getenv("HW_TASK") == "" || !ok {
		return 0
	}
	if t.fs.mkdirAll(stateDir) != nil {
		return 0
	}
	// A child session's turn end is not the executor's: no count, no handback, no ruling.
	if t.piTurnEndForeign(stateDir) { // # MUTATION-ANCHOR: 986-T01
		return 0
	}
	t.engramRenew(stateDir)

	taskDir := mkTaskDir(stateDir)
	t.fs.watch(taskDir)
	var turns int64
	readTurns := func() {
		turns = 0
		if b, err := os.ReadFile(stateDir + "/turns"); err == nil {
			turns = t3Atoi(t3Digits(string(b)))
		}
	}

	// `done` beside `reopened` is a run `hw revive` brought back: its old report stands, and this
	// session owes a new one, so the turn is judged like any unreported run's.
	if isFile(taskDir+"/done") && !isFile(taskDir+"/reopened") { // # MUTATION-ANCHOR: 841-T03
		t.fs.remove(stateDir + "/handback-refused") // # MUTATION-ANCHOR: 841-B05
		curSeq := mkTaskSeq(stateDir)
		postSeq, post := "", int64(0)
		if b, err := os.ReadFile(stateDir + "/turns-post-report"); err == nil {
			rec := bashSub(string(b))
			postSeq = rec
			if i := strings.IndexByte(rec, ' '); i >= 0 {
				postSeq = rec[:i]
			}
			last := rec
			if i := strings.LastIndexByte(rec, ' '); i >= 0 {
				last = rec[i+1:]
			}
			if last == "" || strings.Trim(last, "0123456789") != "" {
				post = 0
			} else {
				post = t3Atoi(last)
			}
		}
		if postSeq != curSeq { // # MUTATION-ANCHOR: 841-T11
			post = 0
		}
		blocked := taskDir + "/blocked-waiting"
		switch {
		case t.getenv("HW_TURN_HAD_INPUT") == "0": // # MUTATION-ANCHOR: 841-T12
		case isFile(blocked) && !t3HasLinePrefix(blocked, "report_turn_ended="):
			t.fs.appendTo(blocked, []byte("report_turn_ended="+strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o666) // # MUTATION-ANCHOR: 841-T05
		default:
			post++ // # MUTATION-ANCHOR: 841-T04
		}
		rec := curSeq                            // # MUTATION-ANCHOR: 841-P01
		rec += " " + strconv.FormatInt(post, 10) // # MUTATION-ANCHOR: 841-P02
		t.fs.write(stateDir+"/turns-post-report", []byte(rec+"\n"), 0o666)
		readTurns()
		t.publish("ended_after_report", turns, "turns_post_report", curSeq+" "+strconv.FormatInt(post, 10))
		return 0
	}

	readTurns()
	turns++
	t.fs.write(stateDir+"/turns", []byte(strconv.FormatInt(turns, 10)+"\n"), 0o666) // # MUTATION-ANCHOR: 841-T01

	// A QUEUED RULING LANDS HERE: the only clean boundary there is. Delivered exactly once.
	if ruling, ok := t.queueClaimAll(stateDir, "turn-end"); ok { // # MUTATION-ANCHOR: 841-T02
		t.publish("ended_with_ruling", turns, "ruling", t.substr(ruling, 80))
		if hookJSON {
			t.blockJSON("RULING FROM YOUR BRAINER, delivered at this turn end because it changes the premise of your work. Read it before doing anything else, and if it contradicts what you have already done, say so rather than quietly redoing it.\n\n" + ruling)
		}
		return 8
	}

	// ask-invoker writes its hold into the CURRENT task's dir.
	if hold := t.pendingReply(taskDir); hold != "" {
		t.fs.remove(stateDir + "/handback-refused") // # MUTATION-ANCHOR: 841-B03
		t.publish("ended_holding", turns, "hold_kind", hold[:strings.IndexByte(hold, '\t')])
		return 0
	}

	if b, err := os.ReadFile(stateDir + "/human-boundary"); err == nil { // # MUTATION-ANCHOR: 841-T06
		boundary := bashSub(strings.ReplaceAll(string(b), "\n", " "))
		t.fs.remove(stateDir + "/human-boundary")   // # MUTATION-ANCHOR: 841-B01
		t.fs.remove(stateDir + "/handback-refused") // # MUTATION-ANCHOR: 841-B02
		t.publish("ended_human_boundary", turns, "human_boundary", t.substr(boundary, 80))
		return 0
	}

	if children := t3LiveChildren(t.getenv("HW_TURN_END_PAYLOAD")); children != "" {
		t.fs.remove(stateDir + "/handback-refused") // # MUTATION-ANCHOR: 841-B04
		t.publish("ended_awaiting_child", turns, "children_running", children)
		return 0
	}

	reason := t.turnReason()
	t.fs.write(stateDir+"/handback-refused", t3Lines(
		"at="+t3IsoNow(),                      // # MUTATION-ANCHOR: 841-H01
		"turns="+strconv.FormatInt(turns, 10), // # MUTATION-ANCHOR: 841-H02
		"reason="+reason,                      // # MUTATION-ANCHOR: 841-H03
	), 0o666)
	t.publish("handback_refused", turns, "", "")
	if hookJSON {
		t.blockJSON(reason)
	}
	return 8 // # MUTATION-ANCHOR: 841-T09
}

// t3HasLinePrefix is `grep -q '^<prefix>' file`.
func t3HasLinePrefix(path, prefix string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}
