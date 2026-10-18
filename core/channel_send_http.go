package main

// channel-send's two OpenCode routes, the two `node --input-type=module` scripts of bin/channel-send as native
// HTTP: `opencode` (the causal route: prompt_async into a known session, with the dead-receiver preflight, the
// idempotent delivery id, the persistence proof and the processed receipt) and `opencode-tui` (the first-delivery
// route: the TUI's own prompt over HTTP, then proof that the exact bytes landed). The words a failed fetch carries
// are node's (undici's): "fetch failed" for a refused connection, "The operation was aborted due to timeout".

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// jsEncodeURIComponent is encodeURIComponent: everything but A-Z a-z 0-9 - _ . ! ~ * ' ( ) is %XX per UTF-8 byte.
func jsEncodeURIComponent(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.IndexByte("-_.!~*'()", b) >= 0 {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

// positiveInt is the node scripts' env reader: digits only, greater than zero, else the fallback.
func positiveInt(v string, fallback int64) int64 {
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// fetchErr is what a rejected fetch() says.
type fetchErr struct{ timeout bool }

func (e *fetchErr) Error() string {
	if e.timeout {
		return "The operation was aborted due to timeout"
	}
	return "fetch failed"
}

type csResp struct {
	status int
	body   []byte
}

func (r *csResp) ok() bool { return r.status >= 200 && r.status < 300 }

func (r *csResp) text() string { return string(r.body) }

// fetch is one request with a deadline over the whole exchange, as AbortSignal.timeout makes it.
func csFetch(method, rawurl string, headers map[string]string, body []byte, timeout time.Duration) (*csResp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawurl, rd)
	if err != nil {
		return nil, &fetchErr{}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Transport: &http.Transport{DisableKeepAlives: true, Proxy: nil}}).Do(req)
	if err != nil {
		var ne net.Error
		to := errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
		return nil, &fetchErr{timeout: to}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil && ctx.Err() != nil {
		return nil, &fetchErr{timeout: true}
	}
	return &csResp{status: resp.StatusCode, body: b}, nil
}

// jsonParse is response.json(): the value, or node's own wording for why not.
func jsonParse(b []byte) (any, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("Unexpected end of JSON input")
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		s := string(b)
		snippet := s
		if len(snippet) > 10 {
			snippet = snippet[:10] + "..."
		}
		return nil, fmt.Errorf("Unexpected token '%c', \"%s\" is not valid JSON", s[0], snippet)
	}
	return v, nil
}

func jobj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func jstring(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// ── the causal route ────────────────────────────────────────────────────────

type ocRoute struct {
	c                                  *chanSend
	endpoint, session, deliveryID      string
	deliveryTO, persistTO, preflightTO time.Duration
	required                           string
	isRuling, holdProven               bool
}

func (o *ocRoute) fail(layer, detail string, code int) {
	o.c.printf("opencode delivery failed at %s: %s\n", layer, detail)
	o.c.exit(code)
}

func (o *ocRoute) failBody(layer string, r *csResp) {
	body := r.text()
	d := "HTTP " + strconv.Itoa(r.status)
	if body != "" {
		d += ": " + body
	}
	o.fail(layer, d, 1)
}

func (o *ocRoute) request(method, path string, headers map[string]string, body []byte, to time.Duration, layer string) *csResp {
	r, err := csFetch(method, o.endpoint+path, headers, body, to)
	if err != nil {
		o.fail(layer, err.Error(), 1)
	}
	return r
}

func (o *ocRoute) json(path, layer string) any {
	r := o.request("GET", path, nil, nil, o.deliveryTO, layer)
	if !r.ok() {
		o.failBody(layer, r)
	}
	v, err := jsonParse(r.body)
	if err != nil {
		o.fail(layer, "invalid JSON response", 1)
	}
	return v
}

func (o *ocRoute) sleepMS(n int) { o.c.sleep(float64(n) / 1000) }

func (c *chanSend) routeOpencode() {
	if c.tgt == "" {
		c.die("opencode target session is empty")
	}
	if !strings.HasPrefix(c.endpoint, "http://") && !strings.HasPrefix(c.endpoint, "https://") {
		c.die("invalid opencode endpoint: " + c.endpoint)
	}
	o := &ocRoute{
		c: c, endpoint: strings.TrimSuffix(c.endpoint, "/"), session: jsEncodeURIComponent(c.tgt),
		deliveryTO:  time.Duration(positiveInt(c.getenv("CHANNEL_DELIVERY_TIMEOUT_MS"), 90000)) * time.Millisecond,
		persistTO:   time.Duration(positiveInt(c.getenv("CHANNEL_PERSISTENCE_TIMEOUT_MS"), 5000)) * time.Millisecond,
		preflightTO: time.Duration(positiveInt(c.getenv("CHANNEL_PREFLIGHT_TIMEOUT_MS"), 2000)) * time.Millisecond,
		required:    c.required, isRuling: c.isRuling, holdProven: c.holdProven,
	}
	if c.deliveryKey != "" {
		h := sha256.Sum256([]byte(c.tgt + "\x00" + c.deliveryKey))
		o.deliveryID = "msg_channel_" + hex.EncodeToString(h[:])[:32]
	} else {
		var b [16]byte
		rand.Read(b[:])
		o.deliveryID = "msg_channel_" + hex.EncodeToString(b[:])
	}
	o.run()
}

// preflightDefaultAgent is the dead-receiver preflight: exit 6 or silence.
func (o *ocRoute) preflightDefaultAgent() {
	var cfgR, agR *csResp
	var cfgE, agE error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); cfgR, cfgE = csFetch("GET", o.endpoint+"/config", nil, nil, o.preflightTO) }()
	go func() { defer wg.Done(); agR, agE = csFetch("GET", o.endpoint+"/agent", nil, nil, o.preflightTO) }()
	wg.Wait()
	if cfgE != nil || agE != nil {
		return
	}
	if !cfgR.ok() || !agR.ok() { // # MUTATION-ANCHOR: 26-G08
		return
	}
	config, err1 := jsonParse(cfgR.body)
	agents, err2 := jsonParse(agR.body)
	if err1 != nil || err2 != nil { // # MUTATION-ANCHOR: 26-G07
		return
	}
	cfg := jobj(config)
	defaultAgent := ""
	if s, ok := jstring(cfg["default_agent"]); ok {
		defaultAgent = strings.TrimSpace(s)
	}
	if defaultAgent == "" { // # MUTATION-ANCHOR: 26-G09
		return
	}
	list, ok := agents.([]any)
	if !ok {
		return
	}
	var resolved []string
	for _, a := range list {
		if s, ok := jstring(jobj(a)["name"]); ok {
			resolved = append(resolved, s)
		}
	}
	if len(resolved) == 0 { // # MUTATION-ANCHOR: 26-G04
		return
	}
	for _, r := range resolved {
		if r == defaultAgent { // # MUTATION-ANCHOR: 26-G03
			return
		}
	}
	var declared any
	if am, ok := cfg["agent"].(map[string]any); ok {
		declared = am[defaultAgent]
	}
	var why string
	if dm := jobj(declared); dm != nil && dm["disable"] == true {
		why = `that scope also sets "disable": true on it`
	} else if jsTruthy(declared) {
		why = "it is declared but did not survive config resolution"
	} else {
		why = "no merged config scope declares it at all"
	}
	scopes := ""
	if pr, err := csFetch("GET", o.endpoint+"/path", nil, nil, o.preflightTO); err == nil && pr.ok() {
		if pv, err := jsonParse(pr.body); err == nil {
			pm := jobj(pv)
			global, project := "", ""
			if s, ok := jstring(pm["config"]); ok {
				global = s + "/opencode.json"
			}
			if s, ok := jstring(pm["directory"]); ok {
				project = s + "/opencode.json"
			}
			if global != "" || project != "" {
				scopes = "\n  the config scopes that have to agree:"
				if global != "" {
					scopes += "\n    global : " + global
				}
				if project != "" {
					scopes += "\n    project: " + project
				}
			}
		}
	}
	o.fail("receiver preflight", fmt.Sprintf("the receiver at %s cannot accept ANY prompt. Its merged config selects default agent \"%s\", which is not among the %d agents the endpoint actually resolves (GET /agent), because %s. Every prompt_async into this session dies with 'default agent \"%s\" not found' before the message is persisted, so NO POST was sent and nothing landed — this is a proven non-delivery and retrying it, with or without --id, cannot succeed.%s\n  fix: make default_agent name a resolvable agent (or stop disabling this one), then RESTART the receiver — the merged config is read at startup.",
		o.endpoint, defaultAgent, len(resolved), why, defaultAgent, scopes),
		6) // # MUTATION-ANCHOR: 26-G02
}

func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	}
	return true
}

func (o *ocRoute) blockers() []string {
	qv := o.json("/question", "question-state")
	pv := o.json("/permission", "permission-state")
	var out []string
	add := func(v any, kind string) {
		list, ok := v.([]any)
		if !ok {
			o.c.printf("TypeError: %s.filter is not a function\n", map[string]string{"question": "questions", "permission": "permissions"}[kind])
			o.c.exit(1)
		}
		for _, it := range list {
			m := jobj(it)
			if s, _ := jstring(m["sessionID"]); s == o.c.tgt {
				id := "<unknown>"
				if iv, ok := m["id"]; ok && iv != nil {
					id = jsString(iv)
				}
				out = append(out, kind+" "+id)
			}
		}
	}
	add(qv, "question")
	add(pv, "permission")
	return out
}

func jsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (o *ocRoute) exactUserMessage(m any) bool {
	mm := jobj(m)
	info := jobj(mm["info"])
	if id, _ := jstring(info["id"]); id != o.deliveryID {
		return false
	}
	if role, _ := jstring(info["role"]); role != "user" {
		return false
	}
	parts, ok := mm["parts"].([]any)
	if !ok || len(parts) != 1 {
		return false
	}
	p := jobj(parts[0])
	if t, _ := jstring(p["type"]); t != "text" {
		return false
	}
	txt, ok := jstring(p["text"])
	return ok && txt == o.c.message
}

func (o *ocRoute) waitForProcessing() {
	deadline := time.Now().Add(o.deliveryTO)
	for time.Now().Before(deadline) {
		v := o.json("/session/"+o.session+"/message?limit=100", "message processing")
		list, ok := v.([]any)
		if !ok {
			o.c.printf("TypeError: messages.some is not a function\n")
			o.c.exit(1)
		}
		for _, m := range list {
			mm := jobj(m)
			info := jobj(mm["info"])
			if role, _ := jstring(info["role"]); role != "assistant" {
				continue
			}
			if p, _ := jstring(info["parentID"]); p != o.deliveryID {
				continue
			}
			if jsTruthy(info["error"]) { // # MUTATION-ANCHOR: 05-G02
				continue
			}
			if _, isNum := jobj(info["time"])["completed"].(float64); isNum {
				return
			}
		}
		o.sleepMS(100)
	}
	o.fail("message processing", fmt.Sprintf("%s IS in the receiver's session — admitted and persisted — but no completed assistant response was causally parented to it within %dms. The delivery succeeded; the turn did not.", o.deliveryID, o.deliveryTO.Milliseconds()), 4) // # MUTATION-ANCHOR: 05-G01
}

func (o *ocRoute) run() {
	o.preflightDefaultAgent() // # MUTATION-ANCHOR: 26-G01
	o.json("/session/"+o.session, "session-identity")
	if o.c.deliveryKey != "" {
		r, err := csFetch("GET", o.endpoint+"/session/"+o.session+"/message/"+jsEncodeURIComponent(o.deliveryID), nil, nil, o.persistTO)
		if err != nil {
			o.fail("idempotency lookup", err.Error(), 1)
		}
		if r.ok() {
			v, _ := jsonParse(r.body)
			if !o.exactUserMessage(v) {
				o.fail("idempotency lookup", o.deliveryID+" already exists with different message bytes", 1)
			}
			if o.required == "processed" {
				o.waitForProcessing()
			}
			return
		}
		if r.status != 404 {
			o.failBody("idempotency lookup", r)
		}
	}
	statuses := o.json("/session/status", "session-status")
	pending := o.blockers()
	var status map[string]any
	var statusRaw any
	if sm := jobj(statuses); sm != nil {
		statusRaw = sm[o.c.tgt]
		status = jobj(statusRaw)
	}
	stType, _ := jstring(status["type"])
	busy := stType == "busy" || stType == "retry"
	if o.isRuling && !o.holdProven && (len(pending) > 0 || (status != nil && busy)) { // # MUTATION-ANCHOR: 41-G03
		state := stType
		if len(pending) > 0 {
			state = "blocked by " + strings.Join(pending, ", ")
		}
		pane := o.c.getenv("CLAUDE_PANE")
		if pane == "" {
			pane = o.c.getenv("HERDR_PANE_ID")
		}
		if pane == "" {
			pane = "<pane>"
		}
		o.fail("ruling safety", fmt.Sprintf("--ruling REFUSED before delivery: receiver is %s, not at a clean idle boundary. A ruling changes the premise of work, so injecting it now could arrive only after the work it was meant to correct. NOTHING was sent.\n\nTHREE EXITS, in the order to consider them:\n  1. `hw ruling %s \"<the correction>\"` queues it, and the executor receives it at its NEXT TURN END. Nothing is stopped, nothing is killed, and you do not wait. This is almost always the right one.\n  2. Let it finish and report, then hand the correction to its next task with hw next.\n  3. Only if the correction cannot wait for a turn end: stop the work deliberately.\n\nDO NOT run hw done to reach exit 3. It closes the executor and its whole session context goes with it, which is a permanent cost for a temporary correction. Do not substitute --report either: reports do not steer work, rulings do.", state, pane), 1)
	}
	if len(pending) > 0 {
		o.fail("process-capable activation", fmt.Sprintf("receiver has pending %s; no text was injected", strings.Join(pending, ", ")), 1)
	}
	if status != nil && !busy {
		o.fail("session-status", "unsupported receiver status "+jsStringify(statusRaw), 1)
	}

	// ONE ROUTE, NOT TWO: prompt_async starts a real turn on a settled session too.
	payload, _ := json.Marshal(map[string]any{"messageID": o.deliveryID, "parts": []any{map[string]any{"type": "text", "text": o.c.message}}})
	r := o.request("POST", "/session/"+o.session+"/prompt_async", map[string]string{"content-type": "application/json"}, payload, o.deliveryTO, "HTTP admission")
	if r.status != 204 {
		o.failBody("HTTP admission", r)
	}
	// A POLL THAT FAILS IS NOT A NON-DELIVERY: the POST was accepted.
	deadline := time.Now().Add(o.persistTO) // # MUTATION-ANCHOR: 26-G06
	lastPollError := ""
	for time.Now().Before(deadline) {
		left := time.Until(deadline)
		if left < time.Millisecond {
			left = time.Millisecond
		}
		p, err := csFetch("GET", o.endpoint+"/session/"+o.session+"/message/"+jsEncodeURIComponent(o.deliveryID), nil, nil, left)
		if err != nil {
			lastPollError = err.Error()
			o.sleepMS(50)
			continue
		}
		if p.ok() {
			v, _ := jsonParse(p.body)
			if o.exactUserMessage(v) {
				if b := o.blockers(); len(b) > 0 {
					o.fail("process-capable activation", "receiver became blocked by "+strings.Join(b, ", "), 1)
				}
				if o.required == "processed" {
					o.waitForProcessing()
				}
				return
			}
			o.fail("message persistence", "receiver returned the wrong message identity", 1)
		}
		if p.status != 404 {
			o.failBody("message persistence", p)
		}
		o.sleepMS(50)
	}
	last := ""
	if lastPollError != "" {
		last = " (last poll: " + lastPollError + ")"
	}
	o.fail("message persistence", fmt.Sprintf("HTTP 204 was ACCEPTED by the endpoint, but polling never confirmed %s landed within %dms%s. The message may already be in the receiver's session — this is not a proven non-delivery. Retrying without --id may duplicate it.", o.deliveryID, o.persistTO.Milliseconds(), last), 5)
}

// jsStringify is JSON.stringify of a decoded value: compact, members in the order the endpoint sent.
func jsStringify(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			sb.Write(kb)
			sb.WriteByte(':')
			sb.WriteString(jsStringify(x[k]))
		}
		sb.WriteByte('}')
		return sb.String()
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = jsStringify(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ── the first-delivery route ────────────────────────────────────────────────

type tuiRoute struct {
	c                                 *chanSend
	endpoint, directory               string
	submitTO, landTO, turnTO, probeTO time.Duration
}

func (t *tuiRoute) fail(layer, detail string, code int) {
	t.c.printf("opencode-tui delivery failed at %s: %s\n", layer, detail)
	t.c.exit(code)
}

func (t *tuiRoute) request(method, path string, headers map[string]string, body []byte, layer string) *csResp {
	r, err := csFetch(method, t.endpoint+path, headers, body, t.submitTO)
	if err != nil {
		t.fail(layer, fmt.Sprintf("%s%s did not answer: %s", t.endpoint, path, err.Error()), 1)
	}
	return r
}

func (t *tuiRoute) json(path, layer string, to time.Duration) any {
	r, err := csFetch("GET", t.endpoint+path, nil, nil, to)
	if err != nil {
		t.fail(layer, fmt.Sprintf("%s%s did not answer: %s", t.endpoint, path, err.Error()), 1)
	}
	if !r.ok() {
		t.fail(layer, fmt.Sprintf("GET %s returned HTTP %d", path, r.status), 1)
	}
	v, err := jsonParse(r.body)
	if err != nil {
		t.fail(layer, fmt.Sprintf("GET %s returned unparseable JSON: %s", path, err.Error()), 1)
	}
	return v
}

// softJSON reads an advisory endpoint: an unreadable answer is "nothing known", never "nothing pending".
func (t *tuiRoute) softJSON(path string) any {
	r, err := csFetch("GET", t.endpoint+path, nil, nil, t.submitTO)
	if err != nil || !r.ok() {
		return nil
	}
	v, err := jsonParse(r.body)
	if err != nil {
		return nil
	}
	return v
}

func (t *tuiRoute) sessionsHere() []map[string]any {
	all := t.json("/session?directory="+jsEncodeURIComponent(t.directory), "session listing", t.submitTO)
	list, ok := all.([]any)
	if !ok {
		t.fail("session listing", "the endpoint did not return a session array", 1)
	}
	var out []map[string]any
	for _, s := range list {
		m := jobj(s)
		if d, _ := jstring(m["directory"]); d == t.directory {
			out = append(out, m)
		}
	}
	return out
}

func msgText(m any) string {
	parts, _ := jobj(m)["parts"].([]any)
	var sb strings.Builder
	for _, p := range parts {
		pm := jobj(p)
		if ty, _ := jstring(pm["type"]); ty == "text" {
			if s, ok := jstring(pm["text"]); ok {
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

func (c *chanSend) routeOpencodeTUI() {
	if c.tgt == "" {
		c.die("opencode-tui target directory is empty")
	}
	if !strings.HasPrefix(c.tgt, "/") {
		c.die("opencode-tui target must be the receiver's absolute working directory, not a session id (got: " + c.tgt + ")")
	}
	if !strings.HasPrefix(c.endpoint, "http://") && !strings.HasPrefix(c.endpoint, "https://") {
		c.die("invalid opencode endpoint: " + c.endpoint)
	}
	if c.isRuling {
		c.die("--ruling REFUSED: opencode-tui is the first-delivery route and cannot establish a receiver boundary — there is no session to read a state from. NOTHING was sent. A ruling belongs on the causal opencode route, which exists from the first turn on.")
	}
	if strings.HasPrefix(c.message, "/") {
		c.die("opencode-tui REFUSED: the message begins with '/', which the TUI prompt reads as a slash command, not as text. NOTHING was sent.")
	}
	ms := func(k string, d int64) time.Duration {
		return time.Duration(positiveInt(c.getenv(k), d)) * time.Millisecond
	}
	t := &tuiRoute{c: c, endpoint: strings.TrimSuffix(c.endpoint, "/"), directory: c.tgt,
		submitTO: ms("CHANNEL_DELIVERY_TIMEOUT_MS", 20000), landTO: ms("CHANNEL_TUI_LAND_TIMEOUT_MS", 30000),
		turnTO: ms("CHANNEL_TUI_TURN_TIMEOUT_MS", 900000), probeTO: ms("CHANNEL_TUI_PROBE_TIMEOUT_MS", 8000)}
	t.run()
}

func (t *tuiRoute) run() {
	// IS ANYONE HOME? The liveness probe comes first and gets its own short budget.
	probe, err := csFetch("GET", t.endpoint+"/agent", nil, nil, t.probeTO)
	if err != nil {
		if fe, ok := err.(*fetchErr); ok && fe.timeout {
			t.fail("endpoint liveness", fmt.Sprintf("%s accepted the connection and answered NOTHING within %dms. The receiver is not slow, it is hung: its port is open and its HTTP server is not serving. NOTHING was typed. Do not fall back to the terminal buffer — the same process serves both, so a buffered prompt produces agent_prompt_stalled and an executor holding no brief. Restart the receiver, then deliver again.", t.endpoint, t.probeTO.Milliseconds()), 6)
		}
		t.fail("endpoint liveness", fmt.Sprintf("%s refused the connection (%s). Nothing is listening yet, which is what a receiver still starting up looks like. NOTHING was typed.", t.endpoint, err.Error()), 1)
	}
	if !probe.ok() {
		t.fail("endpoint liveness", fmt.Sprintf("GET /agent returned HTTP %d; this receiver cannot be used", probe.status), 6)
	}

	// THE REFUSALS, all before anything is typed.
	before := t.sessionsHere()
	statuses := t.json("/session/status", "receiver status", t.submitTO)
	questions := t.softJSON("/question")
	permissions := t.softJSON("/permission")
	mine := map[string]bool{}
	for _, s := range before {
		if id, ok := jstring(s["id"]); ok {
			mine[id] = true
		}
	}
	var busy []string
	if sm := jobj(statuses); sm != nil {
		ids := make([]string, 0, len(sm))
		for id := range sm {
			ids = append(ids, id)
		}
		sort.Strings(ids) // the endpoint's own key order is not kept by a decoded map; a stable one is the next best
		for _, id := range ids {
			if ty, _ := jstring(jobj(sm[id])["type"]); mine[id] && (ty == "busy" || ty == "retry") {
				busy = append(busy, id)
			}
		}
	}
	if len(busy) > 0 {
		t.fail("receiver readiness", fmt.Sprintf("the receiver is mid-turn (%s). A TUI prompt is one shared buffer, so appending now would land wherever that turn happens to be, or be lost. NOTHING was typed. This route is for the FIRST delivery; a receiver that already has a turn has a session, and the causal opencode route is stronger.", strings.Join(busy, ", ")), 1)
	}
	var pending []string
	addPending := func(v any, kind string) {
		if l, ok := v.([]any); ok {
			for _, q := range l {
				if id, _ := jstring(jobj(q)["sessionID"]); mine[id] {
					pending = append(pending, kind)
				}
			}
		}
	}
	addPending(questions, "question")
	addPending(permissions, "permission")
	if len(pending) > 0 {
		var uniq []string
		seen := map[string]bool{}
		for _, p := range pending {
			if !seen[p] {
				seen[p] = true
				uniq = append(uniq, p)
			}
		}
		t.fail("receiver readiness", fmt.Sprintf("the receiver has a pending %s modal; text typed now answers the modal instead of opening a turn. NOTHING was typed.", strings.Join(uniq, " and ")), 1)
	}

	// TYPE IT, THEN SUBMIT IT. Clear first: whatever a stalled delivery left in the buffer would change the bytes.
	appendBody, _ := json.Marshal(map[string]string{"text": t.c.message})
	for _, st := range []struct {
		path  string
		body  []byte
		layer string
	}{{"/tui/clear-prompt", nil, "prompt clear"}, {"/tui/append-prompt", appendBody, "prompt append"}, {"/tui/submit-prompt", nil, "prompt submit"}} {
		var h map[string]string
		if st.body != nil {
			h = map[string]string{"content-type": "application/json"}
		}
		r := t.request("POST", st.path, h, st.body, st.layer)
		if !r.ok() {
			d := r.text()
			detail := fmt.Sprintf("POST %s returned HTTP %d", st.path, r.status)
			if d != "" {
				detail += ": " + d
			}
			t.fail(st.layer, detail, 1)
		}
		v, err := jsonParse(r.body)
		if err != nil {
			v = nil
		}
		if v != true {
			t.fail(st.layer, fmt.Sprintf("POST %s answered %s instead of true — the TUI did not accept it", st.path, jsStringifyTop(v)), 1)
		}
	}

	// THEN PROVE IT: our exact bytes as a user message in this directory, which is also how the session is found.
	var landedSession string
	var landedCreated float64
	landDeadline := time.Now().Add(t.landTO)
	for time.Now().Before(landDeadline) && landedSession == "" {
		for _, s := range t.sessionsHere() {
			sid, _ := jstring(s["id"])
			mv := t.json("/session/"+sid+"/message?limit=20", "message landing", t.submitTO)
			list, _ := mv.([]any)
			for _, m := range list {
				info := jobj(jobj(m)["info"])
				if role, _ := jstring(info["role"]); role == "user" && msgText(m) == t.c.message {
					landedSession = sid
					landedCreated, _ = jobj(info["time"])["created"].(float64)
					break
				}
			}
			if landedSession != "" {
				break
			}
		}
		if landedSession != "" {
			break
		}
		t.c.sleep(0.4)
	}
	if landedSession == "" {
		t.fail("message landing", fmt.Sprintf("the TUI accepted the submission, but no user message with these exact bytes appeared under %s within %dms. The message may be in the receiver's session — this is NOT a proven non-delivery, so do not blind-retry: read the pane first.", t.directory, t.landTO.Milliseconds()), 5)
	}
	t.c.printf("CHANNEL_SESSION_ID=%s\n", landedSession)
	if t.c.required != "processed" {
		return
	}
	// A completed assistant response created after our message: the same causal bar the opencode route holds.
	turnDeadline := time.Now().Add(t.turnTO)
	for time.Now().Before(turnDeadline) {
		mv := t.json("/session/"+landedSession+"/message?limit=40", "turn processing", t.submitTO)
		list, _ := mv.([]any)
		for _, m := range list {
			info := jobj(jobj(m)["info"])
			if role, _ := jstring(info["role"]); role != "assistant" {
				continue
			}
			created, _ := jobj(info["time"])["created"].(float64)
			if created < landedCreated {
				continue
			}
			if jsTruthy(info["error"]) {
				t.fail("turn processing", fmt.Sprintf("the brief IS in session %s and its turn failed: %s. Do not resend — the receiver has the message.", landedSession, jsStringify(info["error"])), 4)
			}
			if c, ok := jobj(info["time"])["completed"]; ok && jsTruthy(c) {
				return
			}
		}
		t.c.sleep(0.5)
	}
	t.fail("turn processing", fmt.Sprintf("the brief IS in session %s, and no assistant response completed within %dms. Do not resend — the receiver has the message.", landedSession, t.turnTO.Milliseconds()), 4)
}

func jsStringifyTop(v any) string {
	if v == nil {
		return "null"
	}
	return jsStringify(v)
}

var _ = url.QueryEscape
var _ = os.Getenv
