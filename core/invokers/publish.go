package invokers

import (
	"bytes"
	"strings"
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
