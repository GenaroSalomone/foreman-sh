package main

// The stall watcher: an executor whose turn ended LEGALLY (waiting on a background child, a reply to its ask, a human
// boundary) was never bounded: a product executor sat 17 h, `working`, behind a lock nobody would release. Each such
// turn end starts bin/stall-watch, detached, which alerts the brainer once if no new turn comes in HW_STALL_MIN minutes
// (default 20; 0 turns it off). The timer is that one short-lived process, in the shape of setup/heavy's frozen-run
// watchdog and started by the hook that already runs at every turn end: no LaunchAgent, no daemon, and a new turn (the
// `turns` count moves), the report or a gone pane ends it.

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// t3RunningWhat names what the running background children are: "shell `sleep 3600`", "subagent explore".
func t3RunningWhat(payload string) string {
	vals, _ := parseStream([]byte(payload))
	var parts []string
	for _, v := range vals {
		bt, ok := v.get("background_tasks")
		if !ok || bt.kind != vArr {
			break
		}
		for _, e := range bt.arr {
			if e.kind != vObj {
				continue
			}
			st, _ := e.get("status")
			if !(st.kind == vStr && st.s == "running") {
				continue
			}
			ty, _ := e.get("type")
			name := "task"
			if ty.kind == vStr && ty.s != "" {
				name = ty.s
			}
			what := ""
			for _, k := range []string{"command", "description"} {
				if w, ok := e.get(k); ok && w.kind == vStr && w.s != "" {
					what = w.s
					break
				}
			}
			if what != "" {
				name += " `" + what + "`"
			}
			parts = append(parts, name)
		}
	}
	out := strings.Join(parts, ", ")
	if len(out) > 300 {
		out = out[:300] + "…"
	}
	return out
}

// stallWatchStart starts bin/stall-watch for this turn end; why is what the executor ended its turn waiting on. It does
// nothing when the watcher is off (HW_STALL_MIN=0), the run is a shadow one, or the tool is not there.
func (t *t3) stallWatchStart(stateDir string, turns int64, why string) {
	if t.fs.readonly || t.getenv("HW_STALL_MIN") == "0" {
		return
	}
	bin := t.binDir() + "/stall-watch"
	if !t3IsExec(bin) {
		return
	}
	cmd := exec.Command(bin, stateDir, strconv.FormatInt(turns, 10), why)
	cmd.Env = os.Environ()
	t3Detach(cmd)
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}
