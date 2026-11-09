package invokers

import (
	"bytes"
	"encoding/json"
	"strings"

	"hwcore/internal/mux"
)

// cockpitKick is invoker_cockpit_kick: tell the brainer's cockpit something
// changed. Never fails. (798-M01)
func (c *Ctx) cockpitKick() {
	pane := c.get("HW_INVOKER_PANE")
	if pane == "" {
		return
	}
	c.quiet(c.BinDir+"/cockpit-state", "--kick", "--invoker", pane)
}

func (c *Ctx) rpc(args ...string) int { return c.quiet(c.BinDir+"/herdr-rpc", args...) }

// publish is invoker_publish: the tokens (and labels) onto the executor's own
// pane, then proof by read-back. labels is "", "clear" or a JSON object.
// 0 proved; a write failure keeps its status; an unproven read-back is 4.
func (c *Ctx) publish(ttl, tokens, labels string) (int, error) {
	if mux.Backend(c.get) == mux.HC {
		return c.publishHC(tokens)
	}
	pane := c.get("HERDR_PANE_ID")
	params := `{"pane_id":` + jqString(pane) + `,"source":` + jqString("hw:"+c.Prog) + `,"ttl_ms":` + ttl + `,"tokens":` + tokens
	switch labels {
	case "":
	case "clear":
		params += `,"clear_state_labels":true`
	default:
		keys, vals, err := orderedObject(labels)
		if err != nil {
			return 0, die("internal: could not build pane.report_metadata params")
		}
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = jqString(k) + ":" + compactJSON(vals[k])
		}
		params += `,"state_labels":{` + strings.Join(parts, ",") + `}`
	}
	params += "}"
	var out bytes.Buffer
	if rc := c.rpc("call", "pane.report_metadata", params); rc != 0 {
		return rc, nil
	}
	if rc := c.run(runOpts{stdout: &out}, c.BinDir+"/herdr-rpc", "call", "agent.list", "{}"); rc != 0 {
		c.errf("invoker_publish: token publication UNPROVEN: agent.list read-back failed\n")
		return 4, nil
	}
	if !readBackProves(out.String(), pane, tokens) {
		c.errf("invoker_publish: token publication UNPROVEN: requested state is not readable on the exact pane\n")
		return 4, nil
	}
	return 0, nil
}

// selfAddress is this process's own pane: the herdr pane in the TUI, the hc
// terminal in the app. The two never mix: mux.Backend decides once.
func (c *Ctx) selfAddress() string {
	if mux.Backend(c.get) == mux.HC {
		return c.get("HC_TERMINAL_ID")
	}
	return c.get("HERDR_PANE_ID")
}

// publishHC is invoker_publish on herdr-core: `hc label` with the same token
// names (in the hw_ namespace) and values, proved by `hc get --json` read-back.
// A null token is a deletion, which hc spells `key=`. 0 proved; the write's
// status on failure; 4 unproven, the herdr path's exit code.
func (c *Ctx) publishHC(tokens string) (int, error) {
	id := c.get("HC_TERMINAL_ID")
	if id == "" {
		return 3, nil
	}
	keys, vals, err := orderedObject(tokens)
	if err != nil {
		return 0, die("internal: could not build hc label params")
	}
	args := []string{"label", id}
	expect := make(map[string]string, len(keys))
	deleted := make(map[string]bool, len(keys))
	for _, k := range keys {
		name := mux.LabelName(k)
		var s string
		if string(vals[k]) != "null" && json.Unmarshal(vals[k], &s) != nil {
			return 0, die("internal: hc label value for %s is not a string", name)
		}
		// hc has no empty value: `key=` deletes, so "" is a deletion too.
		if s == "" {
			deleted[name] = true
		} else {
			expect[name] = s
		}
		args = append(args, name+"="+s)
	}
	if rc := c.quiet("hc", args...); rc != 0 {
		return rc, nil
	}
	out, rc := c.out("hc", "get", id, "--json")
	if rc != 0 {
		c.errf("invoker_publish: token publication UNPROVEN: hc get read-back failed\n")
		return 4, nil
	}
	if !hcReadBackProves(out, id, expect, deleted) {
		c.errf("invoker_publish: token publication UNPROVEN: requested state is not readable on the exact hc terminal\n")
		return 4, nil
	}
	return 0, nil
}

// hcReadBackProves is readBackProves for hc: the terminal is this one, every
// requested label is readable with its value, and every deletion is absent.
func hcReadBackProves(observed, id string, expect map[string]string, deleted map[string]bool) bool {
	doc, ok := decodeDoc(observed)
	if !ok {
		return false
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return false
	}
	if addr, _ := top["address"].(string); addr != id {
		return false
	}
	labels, ok := top["labels"].(map[string]any)
	if !ok {
		if len(expect) == 0 && len(deleted) == 0 {
			return true
		}
		return false
	}
	for k, want := range expect {
		got, has := labels[k]
		if !has || got != want {
			return false
		}
	}
	for k := range deleted {
		if _, has := labels[k]; has {
			return false
		}
	}
	return true
}

// readBackProves is the jq -e of invoker_publish: exactly one agent on the pane,
// every requested token readable as requested and every deletion absent.
func readBackProves(observed, pane, expectedJSON string) bool {
	doc, ok := decodeDoc(observed)
	if !ok {
		return false
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return false
	}
	if e, has := top["error"]; has && e != nil {
		return false
	}
	agents, ok := top["agents"].([]any)
	if !ok {
		return false
	}
	exp, ok := decodeDoc(expectedJSON)
	expect, isObj := exp.(map[string]any)
	if !ok || !isObj || len(expect) == 0 {
		return false
	}
	var matches []map[string]any
	for _, a := range agents {
		m, isMap := a.(map[string]any)
		if !isMap {
			return false // `.pane_id` on a non-object fails the whole filter
		}
		if p, _ := m["pane_id"].(string); p == pane {
			if _, has := m["pane_id"]; has {
				matches = append(matches, m)
			}
		}
	}
	if len(matches) != 1 {
		return false
	}
	actual, ok := matches[0]["tokens"].(map[string]any)
	if !ok {
		return false
	}
	for k, want := range expect {
		got, has := actual[k]
		if want == nil {
			if has {
				return false
			}
			continue
		}
		if !has || got != want {
			return false
		}
	}
	return true
}

// publicationNote is invoker_publication_note.
func publicationNote(rc int) string {
	if rc == 0 {
		return "Metadata was verified by read-back before the delivery attempt."
	}
	return "Metadata publication could not be established; do not rely on a token record on this pane."
}
