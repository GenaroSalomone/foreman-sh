package main

// outbox list (train T2): the reports that never reached a brainer, read from the run directories' outbox
// folders. Port of cmd_outbox's `list` half (bin/hw:11773-11900). `outbox flush` writes (it redelivers through
// channel-send and removes what it delivered), so it stays bash: the shim sends only the list here.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// reOutboxPath is find's `-path '*/.hw/*/outbox/*'`, where `*` also matches a slash.
var reOutboxPath = regexp.MustCompile(`/\.hw/.*/outbox/.*`)

func (a *app) cmdOutbox() int {
	proj, args := "", a.args
	if len(args) > 0 && (args[0] == "list" || args[0] == "flush") {
		args = args[1:]
	}
	for len(args) > 0 {
		switch {
		case args[0] == "--to":
			if len(args) < 2 {
				a.die("--to needs a value")
			}
			args = args[2:]
			continue
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown option: " + args[0] + " (usage: hw outbox [list|flush] [<project>] [--to <brainer-pane>])")
		default:
			proj = a.c.t.canonProject(args[0])
			if !a.c.t.known(proj) {
				a.die("unknown project: " + args[0] + " (expected: " + a.laneNamesHint() + ")")
			}
		}
		args = args[1:]
	}
	n := 0
	for _, m := range a.outboxMetas(proj) {
		key := strings.TrimSuffix(m, ".meta")
		if !isFile(key + ".msg") {
			continue
		}
		st, p, task := outboxField(m, "state"), outboxField(m, "project"), outboxField(m, "task")
		if st == "uncertain" { // # MUTATION-ANCHOR: 871-G50
			a.printf("  %s:%s  %s  uncertain — NOT redelivered: it may already be in the brainer's session (channel-send exit 5); read that pane, then remove %s\n",
				p, task, outboxField(m, "status"), key+".msg")
			continue
		}
		a.printf("  %s:%s  %s  pending  envelope %s  (%s)\n", p, task, outboxField(m, "status"), outboxField(m, "envelope"), key+".msg")
		n++
	}
	if n == 0 {
		a.info("no pending reports in any outbox" + map[bool]string{true: " for " + proj, false: ""}[proj != ""])
	}
	return 0
}

// outboxField is _outbox_field: the first `key=value` line's value, "" for none.
func outboxField(file, key string) string {
	body, ok := readText(file)
	if !ok {
		return ""
	}
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

// outboxRoots is _outbox_roots: the directories a run's `.hw` can sit under, one per line.
func (a *app) outboxRoots(proj string) []string {
	var roots []string
	if proj != "" {
		if isDir(a.c.work + "/" + proj) {
			roots = append(roots, a.c.work+"/"+proj)
		}
	} else {
		roots = append(roots, a.c.work)
	}
	for _, line := range strings.Split((&statusRun{app: a}).worktreeRoots(), "\n") {
		if line == "" {
			continue
		}
		i := strings.LastIndex(line, ":")
		root, lane := line[:i], line[i+1:]
		if proj != "" && lane != proj {
			continue
		}
		if isDir(root) {
			roots = append(roots, root)
		}
	}
	return roots
}

// outboxMetas is _outbox_metas: every `*.meta` under `.hw/<run>/outbox/`, oldest first. The walk is `find
// <root> -maxdepth 8 ( -name node_modules -o -name .git ) -prune -o ( -type d -path '<root>/*/*/*' ! -name .hw
// ! -path '*/.hw/*' -prune ) -o -type f -name '*.meta' -path '*/.hw/*/outbox/*' -print`: below the third level
// only a `.hw` leads to an outbox, so nothing else is entered.
func (a *app) outboxMetas(proj string) []string {
	set := map[string]bool{}
	for _, root := range a.outboxRoots(proj) {
		rootDepth := strings.Count(strings.TrimRight(root, "/"), "/")
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			depth := strings.Count(strings.TrimRight(path, "/"), "/") - rootDepth
			if path == root {
				depth = 0
			}
			name := d.Name()
			if d.IsDir() {
				if depth > 0 && (name == "node_modules" || name == ".git") {
					return filepath.SkipDir
				}
				if depth >= 3 && name != ".hw" && !strings.Contains(path, "/.hw/") { // # MUTATION-ANCHOR: 826-G29
					return filepath.SkipDir
				}
				if depth >= 8 {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() && strings.HasSuffix(name, ".meta") && reOutboxPath.MatchString(path) {
				set[path] = true
			}
			return nil
		})
	}
	// sort -u, then `<mtime> <path>` lines through `sort -n` and `cut`: ties are broken by the line, as the shell does
	paths := make([]string, 0, len(set))
	for m := range set {
		paths = append(paths, m)
	}
	paths = sortLocale(paths, true)
	lines := make([]string, len(paths))
	for i, m := range paths {
		var mt int64
		if st, err := os.Stat(m); err == nil {
			mt = st.ModTime().Unix()
		}
		lines[i] = strconv.FormatInt(mt, 10) + " " + m
	}
	var out []string
	for _, l := range sortLinesNumeric(lines) {
		if i := strings.IndexByte(l, ' '); i >= 0 {
			out = append(out, l[i+1:])
		}
	}
	return out
}
