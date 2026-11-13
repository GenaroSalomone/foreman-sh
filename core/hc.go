package main

// THE herdr-core READER OF THE INVOKER RESOLVER (herdr-app-brain, step 2).
//
// The TUI resolver reads `herdr pane list` and matches on pane_id, label and cwd.
// An app terminal carries the same three facts under other names — its address,
// the `label` key of its hc labels, and its cwd — so this reader NORMALISES
// `hc ls`/`hc get` into that shape and the resolver stays one program: the same
// arms, the same refusals, the same words. `hc ls` answers with no labels, so the
// label of a NAMED terminal costs one `hc get`; the brainers come from a label
// query, which needs none.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

// hcOut is herdrOut's hc twin: what `hc <args>` printed, and whether it succeeded.
func hcOut(timeout time.Duration, args ...string) (string, bool) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "hc", args...)
	// a child that outlives the deadline still holds the stdout pipe (herdrOut's reason)
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Env = childEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err == nil
}

// hcPaneListJSON is this call's terminal list in herdr's shape,
// {"result":{"panes":[{"pane_id","label","cwd","agent"}]}}, or "" when `hc ls`
// could not answer — the same "" the herdr path reads as unreachable.
//
// named are the terminals this call names (HW_INVOKER_PANE, HC_TERMINAL_ID): they
// are read with `hc get` so their own labels are visible, which is what lets the
// resolver refuse an executor or accept a brainer it would otherwise not know.
func hcPaneListJSON(named ...string) string {
	raw, ok := hcOut(0, "ls", "--json", "--label", "label=brain")
	if !ok {
		return ""
	}
	v, parsed := bjParse(brTrimNL(raw))
	list, isList := bjGet(v, "terminals").([]any)
	if !parsed || !isList { // a reply hw cannot read is not "no brainer": see hcTerminals
		return ""
	}
	panes := []any{}
	seen := map[string]bool{}
	add := func(id, label, cwd string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		panes = append(panes, jmap{"pane_id": id, "label": label, "cwd": cwd, "agent": "hc"})
	}
	for _, e := range list {
		if m, isMap := e.(jmap); isMap {
			add(hcStr(m, "address"), "brain", hcStr(m, "cwd"))
		}
	}
	fetched := map[string]bool{}
	for _, id := range named {
		if id == "" || fetched[id] {
			continue
		}
		fetched[id] = true
		one, got := hcOut(0, "get", id, "--json")
		if !got {
			continue // not a live terminal: the resolver's own liveness refusal says so
		}
		v, parsed := bjParse(brTrimNL(one))
		if !parsed {
			continue
		}
		m, isMap := v.(jmap)
		if !isMap || hcStr(m, "state") != "live" { // a finished terminal is not a pane to report to
			continue
		}
		label := ""
		if labels, isMap := m["labels"].(jmap); isMap {
			label = hcStr(labels, "label")
		}
		add(hcStr(m, "address"), label, hcStr(m, "cwd"))
	}
	out, err := json.Marshal(map[string]any{"result": map[string]any{"panes": panes}})
	if err != nil {
		return ""
	}
	return string(out)
}

// hcAgentGetJSON is `herdr agent get <id>` for a live hc terminal, in herdr's shape
// ({"result":{"agent":{agent_status,cwd,agent,tokens}}}), so the witness helpers
// channel-send and hw ruling already have read it unchanged. The tokens are the
// terminal's labels as published (hw_turn_state), plus each one without its hw_
// prefix (turn_state) where that name is free: the herdr tokens carry no prefix.
// agent_status is the program's own OSC 7501 word, "unknown" while it has said none.
// ok is false for a terminal that is gone, finished, or unreadable.
func hcAgentGetJSON(id string) (string, bool) {
	raw, ok := hcOut(20*time.Second, "get", id, "--json")
	if !ok {
		return "", false
	}
	v, parsed := bjParse(brTrimNL(raw))
	m, isMap := v.(jmap)
	if !parsed || !isMap || hcStr(m, "state") != "live" {
		return "", false
	}
	tokens := jmap{}
	labels, _ := m["labels"].(jmap)
	for k, val := range labels {
		tokens[k], _ = val.(string)
	}
	for k, val := range labels {
		if bare := strings.TrimPrefix(k, "hw_"); bare != k {
			if _, taken := tokens[bare]; !taken {
				tokens[bare], _ = val.(string)
			}
		}
	}
	status, app := "unknown", ""
	if st, isSt := m["status"].(jmap); isSt {
		app = hcStr(st, "app")
		if s := hcStr(st, "state"); s != "" {
			status = s
		}
	}
	out, err := json.Marshal(map[string]any{"result": map[string]any{"agent": jmap{
		"agent_status": status, "cwd": hcStr(m, "cwd"), "agent": app, "turn": m["turn"], "tokens": tokens}}})
	if err != nil {
		return "", false
	}
	return string(out), true
}

// hcWaitBoundary waits for a terminal to reach one of the statuses `at`: 0 reached, 2 budget spent, 3 terminal gone,
// 6 blocked on a person. `wait` runs `hc wait` for that many seconds and returns its exit code.
//
// A failed `hc wait` returns at once, so it must not become a hot loop until the budget ends: it backs off
// a second and the next status read decides. shortcut: what each non-zero code of `hc wait` means (timeout vs error) is
// unmeasured, so none is read as an answer; the status read is the only authority.
func hcWaitBoundary(state func() (string, bool), wait func(secs int64) int, pause func(), totalMS int64, at ...string) int {
	deadline := nextEpoch() + (totalMS+999)/1000
	for {
		st, ok := state()
		if !ok {
			return 3
		}
		for _, s := range at {
			if st == s {
				return 0
			}
		}
		if st == "blocked" {
			return 6
		}
		remaining := deadline - nextEpoch()
		if remaining <= 0 {
			return 2
		}
		if remaining > 20 {
			remaining = 20
		}
		if wait(remaining) != 0 {
			pause()
		}
	}
}

// hcRunIdentity is the environment an executor carries to say WHICH run it is. An app started from an executor's shell hands it to
// every terminal it opens (QA5 G2: HW_INVOKER_PANE, HW_RUN, HW_TASK in a plain app terminal), so a brainer started there
// would read itself as that executor. A brainer starts without it.
var hcRunIdentity = []string{"HW_RUN", "HW_TASK", "HW_PROJECT", "HW_WORKDIR", "HW_ARTIFACTS", "HW_EXECUTOR_VENDOR",
	"HW_PORT_WEB", "HW_PORT_API", "HW_AGENT_ENDPOINT", "HW_AGENT_PORT", "HW_INVOKER_SESSION", "HW_INVOKER_VENDOR", "HW_INVOKER_ENDPOINT"}

// hcPathWord is the `PATH=` word that puts brain/bin first in an hc terminal. The core runs a program with ITS
// environment, which lacks hw and brain (MEASURED 2026-11-09, herdr-app live-cycle QA: a brainer and an executor
// in the app could dispatch only by absolute path, and a plain shell was refused as an invoker); the caller's PATH
// is the nearest thing to the one its own shell resolves claude, node and gh with.
func hcPathWord(binDir, path string) string {
	if path == "" {
		return "PATH=" + binDir
	}
	return "PATH=" + binDir + ":" + path
}

// hcSandboxPathWord puts the sandbox shim's directory first in a `PATH=` word; with no shim the word is returned as it is.
func hcSandboxPathWord(shimdir, word string) string {
	if shimdir == "" {
		return word
	}
	return "PATH=" + shimdir + ":" + strings.TrimPrefix(word, "PATH=") // MUTATION-ANCHOR: hc-sbx-path
}

// hcBrokerAlive waits (10 s, as agentStartSandboxed) for the sandbox broker's heartbeat in the run's state dir: the proof
// that the agent an hc terminal started went through the shim, and so runs inside the profile.
func hcBrokerAlive(state string, sleep func(time.Duration)) bool {
	for n := 0; n < 20; n++ {
		if fi, err := os.Stat(state + "/broker.alive"); err == nil && fi.Mode().IsRegular() {
			return true
		}
		sleep(500 * time.Millisecond)
	}
	return false
}

// hcChildPids is `pgrep -P <pid>`: the pids of a process's children; a variable so a test names a tree without spawning one.
var hcChildPids = func(pid string) []string {
	out, _ := exec.Command("pgrep", "-P", pid).Output()
	return strings.Fields(string(out))
}

// claudeSessionSandboxed is claudeSessionOf for a terminal started through the sandbox shim: the pid `hc get` names is the
// launch wrapper (`hw-sandbox run`, which holds the broker) and claude is its child under sandbox-exec, so the session file
// is the child's. Measured 2026-11-12: with the terminal's own pid a sandboxed dispatch recorded "NO SESSION ID".
func claudeSessionSandboxed(pid, cwd string, dirs ...string) string {
	if !allDigits(pid) {
		return ""
	}
	for _, child := range hcChildPids(pid) {
		if sid := claudeSessionOf(child, cwd, dirs...); sid != "" { // MUTATION-ANCHOR: hc-sbx-session
			return sid
		}
	}
	return ""
}

// hcCoreID is the id of the herdr-core this process talks to ("" when unreadable): `<state>/herdr-core/default/core.json`, with the
// state root the core itself uses ($XDG_STATE_HOME, else ~/.local/state). An address (t2) is only unique inside one core, so
// the files and runs hw keys on one carry this beside it. shortcut: only the `default` core is read, when a second named core exists.
func hcCoreID(getenv func(string) string) string {
	root := getenv("XDG_STATE_HOME")
	if root == "" {
		root = getenv("HOME") + "/.local/state"
	}
	raw, err := os.ReadFile(root + "/herdr-core/default/core.json")
	if err != nil {
		return ""
	}
	var c struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return ""
	}
	return c.ID
}
