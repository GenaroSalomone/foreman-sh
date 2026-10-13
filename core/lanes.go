package main

// The lane table: brain/projects.json, read the way bin/project-spaces.sh's
// lane_config_load reads it (project-spaces.sh:56-196), with the same checks in
// the same order, so a table that does not load fails the same way. The jq
// program's own diagnostic line ("jq: error (at FILE:N): ...") is the one text
// here that is only approximated; see setup/contracts/hw.md.

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type lane struct {
	name string
	f    map[string]string // the _LC_<lane>__<field> values
	hwAl []string
	brAl []string
}

type table struct {
	file   string
	names  []string // HW_LANES, in table order
	lanes  map[string]*lane
	aliasH map[string]string // hw alias -> lane
	work   string            // _LC_WORK_DEFAULT
	// T2: operator and retention (the _LC_OPERATOR_DEFAULT and _LC_RETENTION_* values, as the strings jq tostring prints)
	operator                         string
	retArtifact, retBackup, retBuild string
	survey                           []string // HW_LANES_SURVEY: survey_order first, then the other lanes in table order
}

var (
	reWord  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	reIdent = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)?$`)
	rePin   = regexp.MustCompile(`^claude-[a-z0-9.-]+$`)
	reSub   = regexp.MustCompile(`^(inherit|haiku|sonnet|opus|fable|claude-[a-z0-9.-]+)$`)
	rePort  = regexp.MustCompile(`^[a-z]+$`)
)

var laneFields = []string{"product_repo", "hw_aliases", "brain_aliases", "space", "engram", "vendor", "model", "account", "artifacts", "repoless",
	"checkout", "checkout_var", "base", "base_ref_prefix", "branch", "worktree_root", "worktree", "ports",
	"hint_aliases", "build", "agents_from", "deps", "db", "devserver", "reap", "sweep", "brain_guard", "brief_note", "qa_target", "sdd_modes",
	"model_floor", "model_pins", "subagent_model", "requested_by", "opencode_config_dir", "suite_lock"}

var laneSub = map[string][]string{
	"model_floor": {"tier", "accepts"},
	"deps":        {"line", "contention"},
	"db":          {"line", "provisioned", "no_worktree", "no_db_inert"},
	"devserver":   {"start", "line", "off"},
	"reap":        {"copies", "copies_from", "max_worktrees", "max_disk_pct", "max_build_gb"},
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(o *jval) []string {
	ks := make([]string, 0, len(o.obj))
	for _, m := range o.obj {
		ks = append(ks, m.k)
	}
	sort.Strings(ks)
	return ks
}

// orDefault is jq's `a // b` for a value that may be absent.
func orDefault(v *jval, def string) string {
	if v == nil || !v.truthy() {
		return def
	}
	return v.tostring()
}

// expandHome is `sub("^~(?=/|$)"; $home)`.
func expandHome(s, home string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		return home + s[1:]
	}
	return s
}

func field(o *jval, k string) *jval {
	if o == nil || o.kind != vObj {
		return nil
	}
	v, _ := o.get(k)
	if v.kind == vNull {
		return nil
	}
	return v
}

func joinStrs(v *jval, sep string) string {
	if v == nil || v.kind != vArr {
		return ""
	}
	parts := make([]string, len(v.arr))
	for i, e := range v.arr {
		parts[i] = e.tostring()
	}
	return strings.Join(parts, sep)
}

func strList(v *jval) []string {
	if v == nil || v.kind != vArr {
		return nil
	}
	out := make([]string, len(v.arr))
	for i, e := range v.arr {
		out[i] = e.tostring()
	}
	return out
}

// loadTable returns the table, or the error text for stderr: the lines
// lane_config_load prints before bin/hw's own `die`.
func loadTable(file, home string) (*table, []string) {
	data, err := os.ReadFile(file)
	if err != nil {
		if _, serr := os.Stat(file); serr != nil || !readable(file) {
			return nil, []string{fmt.Sprintf("lane table: cannot read %s", file)}
		}
		return nil, []string{fmt.Sprintf("lane table: cannot read %s", file)}
	}
	bad := func(msg string) (*table, []string) {
		return nil, []string{fmt.Sprintf("jq: error (at %s:%d): %s", file, strings.Count(string(data), "\n"), msg),
			fmt.Sprintf("lane table: %s is not a valid lane table (nothing was loaded)", file)}
	}
	vals, perr := parseStream(data)
	if perr != nil {
		return nil, []string{"jq: error: " + perr.Error() + " (while parsing '" + file + "')",
			fmt.Sprintf("lane table: %s is not a valid lane table (nothing was loaded)", file)}
	}
	if len(vals) == 0 {
		return nil, []string{fmt.Sprintf("lane table: %s is not a valid lane table (nothing was loaded)", file)}
	}
	root := vals[0]
	if root.kind != vObj {
		return bad("Cannot index " + kindName(root) + ` with "lanes"`)
	}
	lanesV := field(root, "lanes")
	if lanesV == nil {
		return bad("no `lanes` object")
	}
	if lanesV.kind != vObj {
		return bad("Cannot iterate over " + kindName(lanesV))
	}
	var names []string
	for _, m := range lanesV.obj {
		names = append(names, m.k)
	}
	if len(names) == 0 {
		return bad("`lanes` is empty")
	}
	var notWord []string
	for _, n := range names {
		if !reWord.MatchString(n) {
			notWord = append(notWord, n)
		}
	}
	if len(notWord) > 0 {
		arr := &jval{kind: vArr}
		for _, n := range notWord {
			arr.arr = append(arr.arr, jstr(n))
		}
		return bad("a lane name is not a lowercase word: " + arr.toJSON())
	}
	// unknown sub-fields, then unknown fields
	var badsub []string
	for _, m := range lanesV.obj {
		if m.v.kind != vObj {
			continue
		}
		for _, f := range m.v.obj {
			allowed, isSub := laneSub[f.k]
			if !isSub || f.v.kind == vNull {
				continue
			}
			if f.v.kind == vObj {
				for _, k := range sortedKeys(f.v) {
					if !has(allowed, k) {
						badsub = append(badsub, m.k+"."+f.k+"."+k)
					}
				}
			} else {
				badsub = append(badsub, m.k+"."+f.k+" (not an object)")
			}
		}
	}
	if len(badsub) > 0 {
		return bad("unknown lane field(s): " + strings.Join(badsub, ", "))
	}
	var unknown []string
	for _, m := range lanesV.obj {
		if m.v.kind != vObj {
			continue
		}
		for _, k := range sortedKeys(m.v) {
			if !has(laneFields, k) {
				unknown = append(unknown, m.k+"."+k)
			}
		}
	}
	if len(unknown) > 0 {
		return bad("unknown lane field(s): " + strings.Join(unknown, ", "))
	}
	collect := func(key string, okf func(*jval) bool) []string {
		var out []string
		for _, m := range lanesV.obj {
			if v := field(m.v, key); v != nil && !okf(v) {
				out = append(out, m.k+"."+key)
			}
		}
		return out
	}
	inList := func(list ...string) func(*jval) bool {
		return func(v *jval) bool { return v.kind == vStr && has(list, v.s) }
	}
	badmodes := collect("sdd_modes", func(v *jval) bool {
		if v.kind != vArr {
			return false
		}
		for _, e := range v.arr {
			if !(e.kind == vStr && e.s == "gentle") {
				return false
			}
		}
		return true
	})
	badreq := collect("requested_by", inList("required", "warn"))
	if len(badreq) > 0 {
		return bad("requested_by is required or warn: " + strings.Join(badreq, ", "))
	}
	badlock := collect("suite_lock", inList("lane", "none"))
	if len(badlock) > 0 {
		return bad("suite_lock is lane or none: " + strings.Join(badlock, ", "))
	}
	badart := collect("artifacts", inList("deny", "default", "personal"))
	if len(badart) > 0 {
		return bad("artifacts is deny or an account (default, personal): " + strings.Join(badart, ", "))
	}
	badfloor := collect("model_floor", func(v *jval) bool {
		tier := field(v, "tier")
		if tier == nil || tier.kind != vStr || !has([]string{"haiku", "sonnet", "opus"}, tier.s) {
			return false
		}
		acc := field(v, "accepts")
		if acc == nil {
			return true
		}
		if acc.kind != vArr {
			return false
		}
		for _, e := range acc.arr {
			if e.kind != vStr {
				return false
			}
		}
		return true
	})
	badpins := collect("model_pins", func(v *jval) bool {
		if v.kind != vObj {
			return false
		}
		for _, m := range v.obj {
			if !has([]string{"haiku", "sonnet", "opus", "fable"}, m.k) || m.v.kind != vStr || !rePin.MatchString(m.v.s) {
				return false
			}
		}
		return true
	})
	badsubm := collect("subagent_model", func(v *jval) bool { return v.kind == vStr && reSub.MatchString(v.s) })
	if len(badsubm) > 0 {
		return bad("subagent_model is inherit, a Claude alias (haiku, sonnet, opus, fable) or a claude-* id: " + strings.Join(badsubm, ", "))
	}
	if len(badpins) > 0 {
		return bad("model_pins maps an alias (haiku, sonnet, opus, fable) to a full claude-* id: " + strings.Join(badpins, ", "))
	}
	if len(badfloor) > 0 {
		return bad("model_floor.tier is haiku, sonnet or opus, and accepts is a list of model ids: " + strings.Join(badfloor, ", "))
	}
	if len(badmodes) > 0 {
		return bad("sdd_modes lists the modes a lane adds to speckit and none, and the only one is gentle: " + strings.Join(badmodes, ", "))
	}
	var top []string
	for _, k := range sortedKeys(root) {
		if !has([]string{"comment", "work", "operator", "metrics_direct", "survey_order", "retention", "lanes"}, k) {
			top = append(top, k)
		}
	}
	if len(top) > 0 {
		return bad("unknown top-level key(s): " + strings.Join(top, ", "))
	}
	retErr := "retention is {artifact_retention_days, backup_retention_days, build_output_days}, each a whole number of days >= 0 (0 disables that window)"
	if r := field(root, "retention"); r != nil {
		if r.kind != vObj {
			return bad(retErr)
		}
		for _, m := range r.obj {
			if !has([]string{"artifact_retention_days", "backup_retention_days", "build_output_days"}, m.k) {
				return bad(retErr)
			}
		}
		for _, k := range []string{"artifact_retention_days", "backup_retention_days", "build_output_days"} {
			v := field(r, k)
			if v == nil {
				continue
			}
			if v.kind != vNum || !wholeNonNegative(v.s) {
				return bad(retErr)
			}
		}
	}
	if so := field(root, "survey_order"); so != nil {
		if so.kind != vArr {
			return bad("Cannot iterate over " + kindName(so))
		}
		for _, e := range so.arr {
			if !has(names, e.tostring()) {
				return bad("survey_order names a lane that `lanes` does not have")
			}
		}
	}
	type al struct{ kind, alias, lane string }
	var aliases []al
	for _, m := range lanesV.obj {
		for _, kind := range []string{"hw", "brain"} {
			if v := field(m.v, kind+"_aliases"); v != nil {
				if v.kind != vArr {
					return bad("Cannot iterate over " + kindName(v))
				}
				for _, e := range v.arr {
					if e.kind != vStr || !reWord.MatchString(e.s) {
						return bad("an alias is not a lowercase word")
					}
					aliases = append(aliases, al{kind, e.s, m.k})
				}
			}
		}
	}
	for _, a := range aliases {
		if has(names, a.alias) {
			return bad("an alias shadows a lane name")
		}
	}
	seen := map[string]bool{}
	for _, a := range aliases {
		if seen[a.kind+"\x00"+a.alias] {
			return bad("an alias is claimed twice")
		}
		seen[a.kind+"\x00"+a.alias] = true
	}
	workV := field(root, "work")
	if workV == nil {
		return bad("no top-level `work`")
	}
	var survey []string
	if so := field(root, "survey_order"); so != nil {
		for _, e := range so.arr {
			survey = append(survey, e.tostring())
		}
	}
	for _, n := range names {
		if !has(survey, n) {
			survey = append(survey, n)
		}
	}
	t := &table{file: file, names: names, survey: survey, lanes: map[string]*lane{}, aliasH: map[string]string{}, work: expandHome(workV.tostring(), home)}
	for _, a := range aliases {
		if a.kind == "hw" {
			t.aliasH[a.alias] = a.lane
		}
	}
	t.operator = orDefault(field(root, "operator"), "the operator")
	t.retArtifact, t.retBackup, t.retBuild = "30", "60", "7"
	if r := field(root, "retention"); r != nil {
		t.retArtifact = orDefault(field(r, "artifact_retention_days"), "30")
		t.retBackup = orDefault(field(r, "backup_retention_days"), "60")
		t.retBuild = orDefault(field(r, "build_output_days"), "7")
	}
	for _, m := range lanesV.obj {
		v := m.v
		if cv := orDefault(field(v, "checkout_var"), ""); !reIdent.MatchString(cv) {
			return bad("checkout_var of " + m.k + " is not an identifier")
		}
		if field(v, "space") == nil {
			return bad("lane " + m.k + " has no space label")
		}
		if p := field(v, "ports"); p != nil && p.kind == vObj {
			for _, pm := range p.obj {
				if !rePort.MatchString(pm.k) || pm.v.kind != vNum {
					return bad("a port of " + m.k + " is not name: number")
				}
			}
		}
		l := &lane{name: m.k, f: map[string]string{}}
		if p := field(v, "ports"); p != nil && p.kind == vObj { // lane_get <lane> port_<name>
			for _, pm := range p.obj {
				l.f["port_"+pm.k] = orDefault(pm.v, "")
			}
		}
		str := func(k, def string) string { return orDefault(field(v, k), def) }
		sub := func(o, k string) *jval { return field(field(v, o), k) }
		l.f["vendor"] = str("vendor", "claude")
		l.f["model"] = str("model", "")
		l.f["account"] = str("account", "default")
		l.f["checkout"] = expandHome(str("checkout", ""), home)
		l.f["checkout_var"] = str("checkout_var", "")
		l.f["engram"] = str("engram", "brain")
		if p := field(v, "product_repo"); p != nil && p.kind == vBool && p.b {
			l.f["product_repo"] = "1"
		}
		l.f["worktree_root"] = expandHome(str("worktree_root", ""), home)
		l.f["base"] = str("base", "")
		l.f["branch"] = str("branch", "task/{task}")
		l.f["worktree"] = expandHome(str("worktree", ""), home)
		l.f["hint_aliases"] = joinStrs(field(v, "hint_aliases"), "|")
		if p := sub("db", "provisioned"); p != nil && p.kind == vBool && p.b {
			l.f["db_provisioned"] = "1"
		}
		l.f["space"] = str("space", "")
		l.f["repoless"] = ""
		if p := field(v, "repoless"); p != nil && p.kind == vBool && p.b {
			l.f["repoless"] = "1"
		}
		l.f["base_ref_prefix"] = str("base_ref_prefix", "origin/")
		l.f["sweep"] = "1"
		if p := field(v, "sweep"); p != nil && p.kind == vBool && !p.b {
			l.f["sweep"] = "0"
		}
		l.f["reap_copies"] = joinStrs(sub("reap", "copies"), "\n")
		l.f["reap_copies_from"] = orDefault(sub("reap", "copies_from"), "")
		l.f["devserver"] = orDefault(sub("devserver", "start"), "")
		l.f["devserver_line"] = orDefault(sub("devserver", "line"), "")
		t.lanes[m.k] = l
	}
	return t, nil
}

func readable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func wholeNonNegative(lit string) bool {
	f, err := strconv.ParseFloat(lit, 64)
	return err == nil && f >= 0 && f == math.Floor(f)
}

func kindName(v *jval) string {
	switch v.kind {
	case vNull:
		return "null"
	case vBool:
		return "boolean"
	case vNum:
		return "number"
	case vStr:
		return "string"
	case vArr:
		return "array"
	}
	return "object"
}

// ── lookups, as project-spaces.sh answers them ──────────────────────────────

func (t *table) known(name string) bool {
	if name == "" || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return false
	}
	_, ok := t.lanes[name]
	return ok
}

// get is lane_get: the raw value, or "" for an unknown lane or field.
func (t *table) get(name, fieldName string) string {
	if !t.known(name) {
		return ""
	}
	return t.lanes[name].f[fieldName]
}

// canonProject is _canon_project: an hw alias names its lane.
func (t *table) canonProject(a string) string {
	if a == "" || strings.Trim(a, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return a
	}
	if l, ok := t.aliasH[a]; ok && l != "" {
		return l // # MUTATION-ANCHOR: 816-G14
	}
	return a
}

func (t *table) namesBar() string { return strings.Join(t.names, " | ") }

type ctx struct {
	t      *table
	brain  string
	work   string
	home   string
	getenv func(string) string
}

// checkout is lane_checkout: the lane's checkout_var when it names one (so an
// override is honoured), else the table's, with {brain} resolved.
func (c *ctx) checkout(name string) string {
	if !c.t.known(name) {
		return ""
	}
	if v := c.t.get(name, "checkout_var"); v != "" {
		if e := c.getenv(v); e != "" {
			return e
		}
		return c.expandBrain(c.t.get(name, "checkout"))
	}
	return c.expandBrain(c.t.get(name, "checkout"))
}

func (c *ctx) expandBrain(s string) string { return strings.ReplaceAll(s, "{brain}", c.brain) }

// lanePath is lane_path: a path template with every token resolved, in
// project-spaces.sh's order.
func (c *ctx) lanePath(name, fieldName, task string) string {
	if !c.t.known(name) {
		return ""
	}
	s := c.t.get(name, fieldName)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "{checkout}", c.checkout(name))
	s = strings.ReplaceAll(s, "{work}", c.work)
	s = strings.ReplaceAll(s, "{lane}", name)
	s = strings.ReplaceAll(s, "{task}", task)
	return c.expandBrain(s)
}
