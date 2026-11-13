package invokers

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// The lane table, read the way the invokers need it: three facts, straight from projects.json. They were three calls into
// bin/project-spaces.sh (lane_config_load, then lane_get <lane> engram, HW_OPERATOR and all_lane_worktree_roots), run in a
// bash of their own. project-spaces.sh stays for install.sh and scripts/init.sh, which read the table before Go exists;
// these callers do not need it. lane_config_load validated the whole table before answering and this reads three keys, so a
// table it would refuse but that still parses answers here: the invokers are the last thing to run in a broken install, and
// every other command already stopped at the loader.

var laneNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// laneTableDoc is the table's JSON, or nil when the file cannot be read, does not parse, or lacks the `lanes` object or a
// work root — `work`, else `root`/work (tableWork) — every table has (what lane_config_load refuses first). It prints nothing: rootsFromTable says why when the
// fallback runs.
func (c *Ctx) laneTableDoc() map[string]any {
	b, err := os.ReadFile(c.or("HW_PROJECTS_JSON", c.brainRoot()+"/projects.json"))
	if err != nil {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil {
		return nil
	}
	if lanes, _ := doc["lanes"].(map[string]any); len(lanes) == 0 {
		return nil
	}
	if tableWork(doc) == "" {
		return nil
	}
	return doc
}

// tableWork is the table's work root as written: `work` when it is a string, else `root` + "/work" (docs/layout.md: the
// agents root is written once, as `root`), else "". project-spaces.sh's _LC_WORK_DEFAULT and core/lanes.go say the same.
func tableWork(doc map[string]any) string {
	if w, ok := doc["work"].(string); ok {
		return w
	}
	if r, ok := doc["root"].(string); ok {
		return r + "/work"
	}
	return ""
}

// jqStringOr is `tostring` of a value `// default` leaves alone: null and false are absent, a string is itself, anything else
// is its JSON.
func jqStringOr(v any, def string) string {
	switch x := v.(type) {
	case nil:
		return def
	case bool:
		if !x {
			return def
		}
		return "true"
	case string:
		return x
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// tableOperator is `.operator // "the operator" | tostring`: the HW_OPERATOR the table gives.
func tableOperator(doc map[string]any) string { return jqStringOr(doc["operator"], "the operator") }

// tableEngram is `lane_get <lane> engram`: the lane's `engram` label ("brain" when it names none), "" for a lane the table
// does not have.
func tableEngram(doc map[string]any, lane string) string {
	lanes, _ := doc["lanes"].(map[string]any)
	v, ok := lanes[lane].(map[string]any)
	if !ok || !laneNameRe.MatchString(lane) {
		return ""
	}
	return jqStringOr(v["engram"], "brain")
}

// derivedRoots is `$WORK` plus all_lane_worktree_roots: every lane's lane_work_bases outside $WORK, one per line. A root is
// only ever asked "is this path under you", so the order and the repeats bash had do not matter and are not kept.
func (c *Ctx) derivedRoots(doc map[string]any) string {
	home := c.get("HOME")
	work := c.get("WORK")
	if work == "" {
		work = expandHome(home, tableWork(doc))
	}
	roots := []string{work}
	lanes, _ := doc["lanes"].(map[string]any)
	for lane, raw := range lanes {
		v, _ := raw.(map[string]any)
		wr, _ := v["worktree_root"].(string)
		wr = expandHome(home, wr)
		if strings.Contains(wr, "{checkout}") {
			wr = strings.ReplaceAll(wr, "{checkout}", c.laneCheckout(v))
		}
		wr = strings.ReplaceAll(wr, "{work}", work)
		wr = strings.ReplaceAll(wr, "{lane}", lane)
		wr = strings.ReplaceAll(wr, "{task}", "")
		wr = strings.ReplaceAll(wr, "{brain}", c.get("BRAIN"))
		if wr == "" || wr == work+"/"+lane || strings.HasPrefix(wr, work+"/") {
			continue
		}
		roots = append(roots, wr)
	}
	return strings.Join(roots, "\n")
}

// laneCheckout is lane_checkout: the lane's checkout_var as the caller set it, else the table's checkout.
func (c *Ctx) laneCheckout(v map[string]any) string {
	if name, _ := v["checkout_var"].(string); name != "" {
		if val := c.get(name); val != "" {
			return val
		}
	}
	co, _ := v["checkout"].(string)
	return strings.ReplaceAll(expandHome(c.get("HOME"), co), "{brain}", c.get("BRAIN"))
}
