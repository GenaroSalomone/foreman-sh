package main

// The lane fields the dispatch reads and the T0/T1 loader (lanes.go) did not need: lane_config_load emits them all
// (bin/project-spaces.sh), and `table.get` answers "" for one the loader left out — which is not what bash answers
// (`subagent_model` is `inherit` unless the lane says otherwise). Added here, in
// T5's own file, so the loader stays as T0/T1 hold it: only what is MISSING from a lane is filled, from the same
// projects.json, with lane_config_load's defaults.

import "os"

func t5FillLaneFields(t *table, home string) {
	data, err := os.ReadFile(t.file)
	if err != nil {
		return
	}
	vals, err := parseStream(data)
	if err != nil || len(vals) == 0 {
		return
	}
	lanes := field(vals[0], "lanes")
	if lanes == nil || lanes.kind != vObj {
		return
	}
	for _, m := range lanes.obj {
		l := t.lanes[m.k]
		v := m.v
		if l == nil || v.kind != vObj {
			continue
		}
		set := func(k, val string) {
			if _, ok := l.f[k]; !ok {
				l.f[k] = val
			}
		}
		str := func(k, def string) string { return orDefault(field(v, k), def) }
		flag := func(o *jval, k string) string { // `if $v.k == true then "1" else "" end`
			if p := field(o, k); p != nil && p.kind == vBool && p.b {
				return "1"
			}
			return ""
		}
		sub := func(o, k string) *jval { return field(field(v, o), k) }
		set("engram", str("engram", "brain"))
		set("vendor", str("vendor", "claude"))
		set("model", str("model", ""))
		set("model_floor", orDefault(sub("model_floor", "tier"), ""))
		set("model_floor_accepts", joinStrs(sub("model_floor", "accepts"), " "))
		pins := ""
		if mp := field(v, "model_pins"); mp != nil && mp.kind == vObj {
			for i, e := range mp.obj {
				if i > 0 {
					pins += " "
				}
				pins += e.k + "=" + e.v.tostring()
			}
		}
		set("model_pins", pins)
		set("requested_by", str("requested_by", ""))
		set("suite_lock", str("suite_lock", ""))
		set("account", str("account", "default"))
		set("subagent_model", str("subagent_model", "inherit"))
		set("artifacts", orDefault(field(v, "artifacts"), orDefault(field(v, "account"), "default")))
		set("repoless", flag(v, "repoless"))
		set("product_repo", flag(v, "product_repo"))
		set("checkout", expandHome(str("checkout", ""), home))
		set("checkout_var", str("checkout_var", ""))
		set("base", str("base", ""))
		set("branch", str("branch", "task/{task}"))
		set("worktree_root", expandHome(str("worktree_root", ""), home))
		set("worktree", expandHome(str("worktree", ""), home))
		set("hint_aliases", joinStrs(field(v, "hint_aliases"), "|"))
		set("build", str("build", ""))
		if p := field(v, "sweep"); p != nil && p.kind == vBool && !p.b {
			set("sweep", "0")
		} else {
			set("sweep", "1")
		}
		if p := field(v, "brain_guard"); p != nil && p.kind == vBool && !p.b {
			set("brain_guard", "0")
		} else {
			set("brain_guard", "1")
		}
		set("agents_from", str("agents_from", ""))
		set("brief_note", str("brief_note", ""))
		set("qa_target", flag(v, "qa_target"))
		set("opencode_config_dir", expandHome(str("opencode_config_dir", ""), home))
		set("deps_line", orDefault(sub("deps", "line"), ""))
		set("deps_contention", flag(field(v, "deps"), "contention"))
		set("db_line", orDefault(sub("db", "line"), ""))
		set("db_provisioned", flag(field(v, "db"), "provisioned"))
		set("db_no_worktree", orDefault(sub("db", "no_worktree"), ""))
		set("db_no_db_inert", flag(field(v, "db"), "no_db_inert"))
		set("devserver", orDefault(sub("devserver", "start"), ""))
		set("devserver_line", orDefault(sub("devserver", "line"), ""))
		set("devserver_off", orDefault(sub("devserver", "off"), ""))
	}
}
