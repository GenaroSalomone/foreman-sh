package main

// hw outbox flush (bin/hw cmd_outbox; the verb of hw-core is `outbox-flush`, the shim hands it what follows `flush`): redeliver the reports that never reached a
// brainer. `outbox list` is a read verb and is not ported here. A report whose
// delivery is UNCERTAIN (channel-send exit 5) is never redelivered: it may already
// be in the brainer's session, and a second copy is the duplicate the exit exists to prevent.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var t3ReOutboxMeta = regexp.MustCompile(`/\.hw/.*/outbox/`)

// mkOutboxRoots is _outbox_roots: the directories a run's outbox can sit under.
func (t *t3) mkOutboxRoots(proj string) []string {
	var out []string
	if proj != "" {
		if isDir(t.c.work + "/" + proj) {
			out = append(out, t.c.work+"/"+proj)
		}
	} else {
		out = append(out, t.c.work)
	}
	for _, p := range t.c.t.names {
		if t.c.t.get(p, "product_repo") != "1" {
			continue
		}
		root := t.c.lanePath(p, "worktree_root", "")
		if root == "" || (proj != "" && p != proj) {
			continue
		}
		if isDir(root) {
			out = append(out, root)
		}
	}
	return out
}

// mkOutboxMetas is _outbox_metas: every `<run>/outbox/*.meta`, oldest first. The walk prunes
// what cannot lead to one (a task's source tree), as the bash does.
func (t *t3) mkOutboxMetas(proj string) []string {
	seen := map[string]bool{}
	var all []string
	for _, r := range t.mkOutboxRoots(proj) {
		var walk func(dir string, depth int, inHW bool)
		walk = func(dir string, depth int, inHW bool) {
			if depth > 7 { // find -maxdepth 8: a file in this directory is at depth+1
				return
			}
			ents, err := os.ReadDir(dir)
			if err != nil {
				return
			}
			for _, e := range ents {
				name := e.Name()
				p := dir + "/" + name
				if e.IsDir() {
					if name == "node_modules" || name == ".git" {
						continue
					}
					d := depth + 1
					nowHW := inHW || name == ".hw"
					if d >= 3 && name != ".hw" && !inHW {
						continue
					}
					walk(p, d, nowHW)
					continue
				}
				if e.Type().IsRegular() && strings.HasSuffix(name, ".meta") && t3ReOutboxMeta.MatchString(p) {
					if !seen[p] {
						seen[p] = true
						all = append(all, p)
					}
				}
			}
		}
		walk(r, 0, false)
	}
	sort.Strings(all)
	type mm struct {
		m  string
		mt int64
	}
	var ms []mm
	for _, p := range all {
		var mt int64
		if st, err := os.Stat(p); err == nil {
			mt = st.ModTime().Unix()
		}
		ms = append(ms, mm{p, mt})
	}
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].mt != ms[j].mt {
			return ms[i].mt < ms[j].mt
		}
		return ms[i].m < ms[j].m
	})
	out := make([]string, len(ms))
	for i, x := range ms {
		out[i] = x.m
	}
	return out
}

// t3OutboxField is _outbox_field: the first `key=` line's value.
func t3OutboxField(meta, key string) string {
	b, err := os.ReadFile(meta)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

func (t *t3) outboxFlush() int {
	args := t.args // `hw outbox flush <args>`: the shim drops the two words
	proj, to := "", ""
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--to":
			if len(args) < 2 {
				t.die("--to needs a value")
			}
			to, args = args[1], args[2:]
		case strings.HasPrefix(a, "-"):
			t.die("unknown option: " + a + " (usage: hw outbox [list|flush] [<project>] [--to <brainer-pane>])")
		default:
			proj = t.c.t.canonProject(a)
			if !t.c.t.known(proj) {
				t.die("unknown project: " + a + " (expected: " + t.laneNamesHint() + ")")
			}
			args = args[1:]
		}
	}
	if to == "" {
		t.die("hw outbox flush needs --to <brainer-pane>: a report is redelivered to a pane you name, never to a guessed one (brain <lane> passes its own)")
	}
	fails := 0
	for _, m := range t.mkOutboxMetas(proj) {
		key := strings.TrimSuffix(m, ".meta")
		t.fs.watch(filepath.Dir(m))
		t.fs.watch(filepath.Dir(filepath.Dir(key)))
		if !isFile(key + ".msg") {
			continue
		}
		st, p, task := t3OutboxField(m, "state"), t3OutboxField(m, "project"), t3OutboxField(m, "task")
		if st == "uncertain" { // # MUTATION-ANCHOR: 841-O01
			t.printf("  %s:%s  %s  uncertain — NOT redelivered: it may already be in the brainer's session (channel-send exit 5); read that pane, then remove %s\n",
				p, task, t3OutboxField(m, "status"), key+".msg")
			continue
		}
		if pid := t3OutboxField(m, "pid"); pid != "" {
			if n, err := strconv.Atoi(pid); err == nil && n > 0 && (t3PidAlive(n)) { // # MUTATION-ANCHOR: 841-O02
				t.printf("  %s:%s  still being sent by done-invoker (pid %s) — left alone\n", p, task, pid)
				continue
			}
		}
		msg := "[redelivered from the outbox: the pane this report was first sent to was gone or unreachable]\n" + mkReadSub(key+".msg")
		cmd := exec.Command(t.binDir()+"/channel-send", "--report", "herdr", to, "-", msg)
		rc := exitStatus(cmd.Run())
		switch rc {
		case 0:
			t.fs.remove(key + ".msg") // # MUTATION-ANCHOR: 841-O05
			t.fs.remove(m)            // # MUTATION-ANCHOR: 841-O06
			// THE REPORT REACHED THE BRAINER: say so where `hw done` and a re-run of done-invoker look.
			state := filepath.Dir(filepath.Dir(key))
			t.fs.write(state+"/done", nil, 0o666) // # MUTATION-ANCHOR: 841-O03
			t.fs.remove(state + "/reopened")      // # MUTATION-ANCHOR: 841-O07
			t.printf("  redelivered %s:%s to %s\n", p, task, to)
		case 5:
			t.fs.write(m, t3Lines(
				"envelope="+t3OutboxField(m, "envelope"), // # MUTATION-ANCHOR: 841-O10
				"project="+p,                         // # MUTATION-ANCHOR: 841-O11
				"task="+task,                         // # MUTATION-ANCHOR: 841-O12
				"run="+t3OutboxField(m, "run"),       // # MUTATION-ANCHOR: 841-O13
				"status="+t3OutboxField(m, "status"), // # MUTATION-ANCHOR: 841-O14
				"state=uncertain",                    // # MUTATION-ANCHOR: 841-O15
				"at="+t3IsoNow(),                     // # MUTATION-ANCHOR: 841-O16
			), 0o666)
			t.warn("redelivering " + p + ":" + task + " to " + to + " is UNCERTAIN (channel-send exit 5): it may already be there; held, not retried")
			fails++
		default:
			t.warn("could not redeliver " + p + ":" + task + " to " + to + " (channel-send exit " + strconv.Itoa(rc) + "); it stays in the outbox")
			fails++
		}
	}
	if fails != 0 {
		return 1
	}
	return 0
}
