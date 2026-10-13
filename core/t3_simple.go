package main

// human-boundary, output and stage (bin/hw cmd_human_boundary, cmd_output,
// cmd_stage): the three small writers an executor runs from its own shell.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ── hw human-boundary "<reason>" ─────────────────────────────────────────────

func (t *t3) humanBoundary() int {
	reason := strings.Join(t.args, " ")
	op := t.operator()
	dir, ok := t.executorStateDir()
	if t.getenv("HW_TASK") == "" || !ok {
		t.die("hw human-boundary is executor-only: HW_TASK, HW_RUN and HW_WORKDIR must identify the run")
	}
	if reason == "" {
		t.die("usage: hw human-boundary \"<specific boundary and why only " + op + " can decide>\"")
	}
	if n := t.charLen(reason); n > 600 {
		t.die("human boundary is " + itoa(n) + " characters; limit 600. Name the boundary and why it needs " + op + ", not the transcript.")
	}
	if strings.Contains(reason, "\n") {
		t.die("human boundary must be one line")
	}
	if err := t.fs.mkdirAll(dir); err != nil { // bash: `mkdir -p` under set -e ends the verb, silently
		return 1
	}
	file := dir + "/human-boundary"
	if err := t.fs.write(file, []byte(reason+"\n"), 0o600); err != nil { // umask 077 // # MUTATION-ANCHOR: 842-N01
		t.die("could not record the human boundary at " + file)
	}
	t.ok("one-shot human boundary recorded for the next turn end: " + reason)
	return 0
}

// ── hw output tree|none|outside ──────────────────────────────────────────────

// receiptAppend is _receipt_into for these verbs: one JSON line, appended, through the plan.
func (t *t3) receiptAppend(dir, key, val, src string) {
	if dir == "" {
		return
	}
	if src == "" {
		t.warn("internal: receipt '" + key + "' has no source; not recorded")
		return
	}
	if t.fs.mkdirAll(dir) != nil {
		return
	}
	obj := &jval{kind: vObj, obj: []member{
		{"key", jstr(key)}, // # MUTATION-ANCHOR: 841-K01
		{"value", jstr(strings.ToValidUTF8(val, "�"))}, // # MUTATION-ANCHOR: 841-K02
		{"source", jstr(src)},                          // # MUTATION-ANCHOR: 841-K03
		{"at", jstr(t3IsoNow())},                       // # MUTATION-ANCHOR: 841-K04
	}}
	t.fs.appendTo(dir+"/receipt.jsonl", []byte(obj.toJSON()+"\n"), 0o666)
}

// t3GitTop is `git rev-parse --show-toplevel 2>/dev/null || true`, then `cd -P … && pwd`.
func t3GitTop() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	top := bashSub(string(out))
	if top == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(top); err == nil {
		return r
	}
	return top
}

func (t *t3) outputRunDir() string {
	wd, run, proj, task := t.getenv("HW_WORKDIR"), t.getenv("HW_RUN"), t.getenv("HW_PROJECT"), t.getenv("HW_TASK")
	if wd == "" || run == "" || proj == "" || task == "" {
		t.die("hw output needs HW_WORKDIR, HW_RUN, HW_PROJECT and HW_TASK from an hw executor; it will not guess a run")
	}
	if !isDir(wd + "/.hw/" + run) {
		t.die("hw output found no run directory at " + wd + "/.hw/" + run + "; it will not invent an attribution record")
	}
	return wd + "/.hw/" + run
}

func t3BadOutputPath(p string) bool {
	switch {
	case p == "", strings.HasPrefix(p, "/"), strings.HasPrefix(p, "../"), strings.Contains(p, "/../"),
		p == ".", p == "..", p == ".hw", strings.HasPrefix(p, ".hw/"), strings.Contains(p, "\n"):
		return true
	}
	return false
}

func (t *t3) output() int {
	kind := ""
	args := t.args
	if len(args) > 0 {
		kind, args = args[0], args[1:]
	}
	rundir := t.outputRunDir()
	writer := t.getenv("HW_PROJECT") + ":" + t.getenv("HW_TASK") + " run " + t.getenv("HW_RUN")
	const decl = "executor declaration via hw output"
	switch kind {
	case "tree":
		if len(args) == 0 {
			t.die("usage: hw output tree <relative-path>... | hw output none | hw output outside <locator>")
		}
		top := t3GitTop()
		if top == "" {
			t.die("hw output tree must run from the repository it edited; use 'cd <tree> && hw output tree <relative-path>...'")
		}
		t.receiptAppend(rundir, "output_writer", writer, decl)                                                    // # MUTATION-ANCHOR: 841-Y01
		t.receiptAppend(rundir, "output_tree_root", top, "git rev-parse --show-toplevel at executor declaration") // # MUTATION-ANCHOR: 841-Y02
		for _, p := range args {
			if t3BadOutputPath(p) {
				t.die("hw output tree path must be a non-.hw relative path without '..' or newlines: " + p)
			}
			t.receiptAppend(rundir, "output_tree_path", p, "executor declaration via hw output (not tree-derived)") // # MUTATION-ANCHOR: 841-Y03
		}
		t.ok("recorded executor-declared tree output for " + writer + "; a brainer checks it with hw stage")
	case "none":
		if len(args) != 0 {
			t.die("usage: hw output none")
		}
		t.receiptAppend(rundir, "output_writer", writer, decl)                                             // # MUTATION-ANCHOR: 841-Y04
		t.receiptAppend(rundir, "output_tree", "none — executor declared no repository-tree output", decl) // # MUTATION-ANCHOR: 841-Y05
		t.ok("recorded no repository-tree output for " + writer)
	case "outside":
		if len(args) == 0 {
			t.die("usage: hw output outside <locator>")
		}
		t.receiptAppend(rundir, "output_writer", writer, decl)                                      // # MUTATION-ANCHOR: 841-Y06
		t.receiptAppend(rundir, "output_external", "outside-tree — "+strings.Join(args, " "), decl) // # MUTATION-ANCHOR: 841-Y07
		t.ok("recorded outside-tree output for " + writer + "; it cannot attribute staged files")
	default:
		t.die("usage: hw output tree <relative-path>... | hw output none | hw output outside <locator>")
	}
	return 0
}

// ── hw stage ─────────────────────────────────────────────────────────────────

// t3StageClaims is the pair of jq programs hw stage runs over one receipt:
//
//	jq -esr --arg root R --arg path P '(any(.[]; .key=="output_tree_root" and .value==$root))
//	                                    and (any(.[]; .key=="output_tree_path" and .value==$path))'
//	jq -sr '[.[] | select(.key == "output_writer")][-1].value // "unknown executor"'
//
// jq's `.key` on anything but an object or null is an error, and `any` stops at
// the first true: the loop below keeps both. matched is the exit status of the
// first program; writer is the output of the second, or "" when it errored.
func t3StageClaims(vals []*jval, root, path string) (matched bool, writer string) {
	anyOf := func(key, want string) (bool, bool) { // found, ok (false = a jq error)
		for _, v := range vals {
			k, ok := v.get("key")
			if !ok {
				return false, false
			}
			if !(k.kind == vStr && k.s == key) { // `and` short-circuits: .value is not read
				continue
			}
			val, ok := v.get("value")
			if !ok {
				return false, false
			}
			if val.kind == vStr && val.s == want {
				return true, true
			}
		}
		return false, true
	}
	r, ok := anyOf("output_tree_root", root)
	if !ok || !r {
		return false, ""
	}
	p, ok := anyOf("output_tree_path", path)
	if !ok || !p {
		return false, ""
	}
	// the writer: the last output_writer's .value, jq's `//` turning null/false into the default
	var last *jval
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			return true, ""
		}
		if k.kind == vStr && k.s == "output_writer" {
			last = v // # MUTATION-ANCHOR: 841-S05
		}
	}
	if last == nil {
		return true, "unknown executor"
	}
	val, _ := last.get("value")
	if !val.truthy() {
		return true, "unknown executor"
	}
	return true, val.raw()
}

// jqSlurpOK reads a receipt the way `jq -s` does: every value, or failure when
// any of the stream is not JSON.
func t3JqSlurp(path string) ([]*jval, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	vals, perr := parseStream(data)
	if perr != nil {
		return nil, false
	}
	return vals, true
}

func (t *t3) stage() int {
	top := t3GitTop()
	if top == "" {
		t.die("hw stage must run inside the repository whose index is being reviewed")
	}
	var receipts []string
	for _, pat := range []string{t.c.work + "/*/*/.hw/*/receipt.jsonl", top + "/.hw/*/receipt.jsonl"} {
		m, _ := filepath.Glob(pat)
		sort.Strings(m)
		receipts = append(receipts, m...)
	}
	t.printf("staged work in %s (executor declarations, not tree-derived authorship)\n", top)
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "-z")
	raw, _ := cmd.Output()
	result, any := 0, false
	for _, path := range strings.Split(string(raw), "\x00") {
		if path == "" {
			continue
		}
		any = true
		matches := ""
		for _, rc := range receipts {
			if !readableFile(rc) {
				continue
			}
			vals, ok := t3JqSlurp(rc)
			if !ok {
				continue
			}
			m, w := t3StageClaims(vals, top, path)
			if !m {
				continue
			}
			if !strings.Contains("\n"+matches+"\n", "\n"+w+"\n") { // # MUTATION-ANCHOR: 841-S03
				if matches != "" {
					matches += "\n"
				}
				matches += w
			}
		}
		switch {
		case matches == "": // # MUTATION-ANCHOR: 841-S01
			t.printf("  UNATTRIBUTED  %s\n", path)
			result = 1
		case strings.Contains(matches, "\n"): // # MUTATION-ANCHOR: 841-S02
			lines := strings.Split(matches, "\n")
			for i := range lines {
				lines[i] = "    claimed by: " + lines[i]
			}
			t.printf("  AMBIGUOUS    %s\n%s\n", path, strings.TrimRight(strings.Join(lines, "\n"), "\n"))
			result = 1
		default:
			t.printf("  claimed by %s  %s\n", matches, path)
		}
	}
	if !any {
		t.printf("  no staged paths\n")
	}
	if result != 0 {
		t.warn("do not commit attribution guesses: declare each executor's tree paths with hw output, or separate the overlapping work")
		return 1
	}
	return 0
}
