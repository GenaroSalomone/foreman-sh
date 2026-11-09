package main

// THE EXECUTORS THAT RUN IN THE HERDR APP, AS THE READ-SIDE TOOLS SEE THEM (herdr-app-read-tools, step 5).
//
// `hw status` reads herdr's `agent list` and `pane list`. An executor launched as an `hc run` terminal is in
// neither, so it is invisible there. hcRunEntries reads `hc ls --label hw_task` + `hc get` and answers in herdr's
// own shapes — the labels are the tokens, hw_turn_state is also turn_state — so the status program stays one
// program. With no hc, or none running, it adds nothing and the TUI path is untouched.
// shortcut: one `hc get` per terminal, serially; upgrade when a lane runs dozens.

import (
	"encoding/json"
	"strings"
	"time"
)

// hcReadTimeout bounds each read: a stuck core must not hang `hw status` (the cockpit writer waits 5 s too).
const hcReadTimeout = 5 * time.Second

// hcRunEntries is the live hc executors as herdr agent entries and pane entries; answered is false when
// `hc ls` could not answer (absent or down), which is "unknown", never "none".
func hcRunEntries() (agents, panes []any, answered bool) {
	raw, ok := hcOut(hcReadTimeout, "ls", "--json", "--label", "hw_task")
	if !ok {
		return nil, nil, false
	}
	v, parsed := bjParse(brTrimNL(raw))
	list, isList := bjGet(v, "terminals").([]any)
	if !parsed || !isList {
		return nil, nil, false
	}
	for _, e := range list {
		m, isMap := e.(jmap)
		if !isMap || hcStr(m, "address") == "" {
			continue
		}
		one, got := hcOut(hcReadTimeout, "get", hcStr(m, "address"), "--json")
		g, isGet := bjParseMap(one, got)
		if !isGet || hcStr(g, "state") != "live" {
			continue // a finished terminal is not a running executor
		}
		tokens := jmap{}
		if labels, isLabels := g["labels"].(jmap); isLabels {
			for k, val := range labels {
				tokens[k] = val
			}
			for k, val := range labels {
				if _, has := tokens[strings.TrimPrefix(k, "hw_")]; strings.HasPrefix(k, "hw_") && !has {
					tokens[strings.TrimPrefix(k, "hw_")] = val
				}
			}
		}
		status := "unknown"
		switch bjRaw(bjGet(g, "status", "state")) {
		case "working":
			status = "working"
		case "idle", "done":
			status = "idle"
		}
		id := hcStr(g, "address")
		agents = append(agents, jmap{"pane_id": id, "agent": "hc", "agent_status": status, "tokens": tokens, "cwd": hcStr(g, "cwd")})
		panes = append(panes, jmap{"pane_id": id, "cwd": hcStr(g, "cwd"), "agent": "hc"})
	}
	return agents, panes, true
}

func bjParseMap(s string, ok bool) (jmap, bool) {
	if !ok {
		return nil, false
	}
	v, parsed := bjParse(brTrimNL(s))
	m, isMap := v.(jmap)
	return m, parsed && isMap
}

// withHCRuns appends the hc entries to a herdr reply of the shape {"result":{<key>:[…]}}. A reply it cannot
// read is returned as it came.
func withHCRuns(reply, key string, extra []any) string {
	if len(extra) == 0 {
		return reply
	}
	var doc map[string]any
	if json.Unmarshal([]byte(reply), &doc) != nil {
		doc = map[string]any{}
	}
	res, _ := doc["result"].(map[string]any)
	if res == nil {
		res = map[string]any{}
	}
	cur, _ := res[key].([]any)
	res[key] = append(cur, extra...)
	doc["result"] = res
	out, err := json.Marshal(doc)
	if err != nil {
		return reply
	}
	return string(out)
}
