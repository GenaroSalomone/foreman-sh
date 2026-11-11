package main

// The one line of the machine-wide gate (bin/suite-gate sg_status_line): `suite slots: R/N in use (who) — W waiting (who)`.
// `hw status` asks it on every call, so it is read here, from the gate's own directory, and not by running bash: the slots and tickets
// are the ones doneGate (t4_done_verify.go) writes, and the liveness test is the one it takes by. bin/suite-gate stays for the
// harness that sources it (setup/test-hw, setup/release/cut).

import (
	"os"
	"sort"
	"strings"
)

// sgLine is the nth line (1-based) of a file; ok is false when it cannot be read (what `head`/`sed` answered with a failure or nothing).
func sgLine(path string, n int) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	ls := strings.Split(string(b), "\n")
	if n > len(ls) || (n == len(ls) && ls[n-1] == "") {
		return "", true
	}
	return ls[n-1], true
}

func sgOr(v string, ok bool, d string) string {
	if !ok {
		return d
	}
	return v
}

// sgWho is _sg_who: pid, task and what the slot runs.
func sgWho(dir string) string {
	pid, ok := sgLine(dir+"/owner.pid", 1)
	task, _ := sgLine(dir+"/owner.task", 1)
	cmd, okc := sgLine(dir+"/owner.cmd", 1)
	s := "pid " + sgOr(pid, ok, "?")
	if task != "" {
		s += ", task " + task
	}
	return s + ": " + sgOr(cmd, okc, "?")
}

// sgEntries lists the names under dir that start with prefix, in the order a shell glob gives them.
func sgEntries(dir, prefix string) []string {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range es {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// statusLine is sg_status_line for the gate of work (SG_WORK): nothing running when there is no gate directory.
func (g *doneGate) statusLine(work string) string {
	n := g.slots() // a bad HW_SUITE_SLOTS is said once, on stderr, before the line
	dir := ""
	if work != "" {
		dir = work + "/.locks/suite-gate"
	}
	if st, err := os.Stat(dir); dir == "" || err != nil || !st.IsDir() {
		return "suite slots: 0/" + itoa(n) + " in use — none waiting"
	}
	r, w := 0, 0
	var who, wwho []string
	for _, name := range sgEntries(dir, "slot.") {
		k := dir + "/" + name
		pid, _ := sgLine(k+"/owner.pid", 1)
		start, _ := sgLine(k+"/owner.start", 1)
		if !doneSgAlive(pid, start, k) {
			continue
		}
		r++
		who = append(who, sgWho(k))
	}
	for _, name := range sgEntries(dir, "q.") {
		if strings.HasSuffix(name, ".part") {
			continue
		}
		t := dir + "/" + name
		pid, _ := sgLine(t, 1)
		start, _ := sgLine(t, 2)
		if !doneSgAlive(pid, start, t) {
			continue
		}
		w++
		tag := ""
		if strings.HasPrefix(name, "q.0.") {
			tag = " [cut]"
		}
		what, _ := sgLine(t, 4)
		wwho = append(wwho, "pid "+pid+tag+": "+what)
	}
	s := "suite slots: " + itoa(r) + "/" + itoa(n) + " in use"
	if len(who) > 0 {
		s += " (" + strings.Join(who, "; ") + ")"
	}
	s += " — " + itoa(w) + " waiting"
	if len(wwho) > 0 {
		s += " (" + strings.Join(wwho, "; ") + ")"
	}
	return s
}
