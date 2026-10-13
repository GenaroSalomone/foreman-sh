package main

// `hw worktrees` (cmd_worktrees, bin/hw:9740, and _wt_occupants, bin/hw:8961):
// the audit of where each repo lane's worktrees live. It reads herdr to learn
// who is working in a stray worktree, and moves a free one only under --migrate.

import (
	"bytes"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// occupants is _wt_occupants: the three-valued answer. observed is false when
// herdr could not say, and why names what failed; empty-and-observed means nobody.
type occupancy struct {
	list     string
	observed bool
	why      string
}

type occupantsReader struct {
	h   herdrClient
	win bool // compare paths the way bash does under msys/cygwin: one spelling, through realpath
}

func trimTR(s string, n int) string { // tr '\n' ' ' | cut -c1-N
	s = strings.ReplaceAll(s, "\n", " ")
	if utf8.RuneCountInString(s) > n {
		r := []rune(s)
		s = string(r[:n])
	}
	return s
}

// windowsPaths says whether paths are compared by the rules bash applies under msys/cygwin
// (_wt_occupants, bin/hw): on a Windows build, or where HW_CORE_PATH_RULES=windows asks for them
// (the contract cases run on every system, and Windows spells a path in more ways than one).
func windowsPaths(getenv func(string) string) bool {
	switch getenv("HW_CORE_PATH_RULES") {
	case "windows":
		return true
	case "posix":
		return false
	}
	return runtime.GOOS == "windows"
}

// onePath is the spelling _wt_occupants puts both sides in under msys/cygwin, where herdr reports a
// backslash drive path and git a mixed one: a string compare of the two reads a worktree with a live
// agent in it as free, which is the dangerous answer (three executors died of it on 2026-08-21).
// Separators become '/', the path is cleaned, symlinks are resolved when the path exists (the
// realpath of bash), and case is dropped because the file system does not tell them apart. Anywhere
// else paths are compared as they are written, which is what bash does.
func onePath(p string, win bool) string {
	if !win || p == "" {
		return p
	}
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	// realpath resolves what exists and keeps the rest as written: a cwd under a directory that is
	// itself reached through a link ("/var" is "/private/var") must still land in the same place
	rest := ""
	for head := p; ; {
		if r, err := filepath.EvalSymlinks(filepath.FromSlash(head)); err == nil {
			p = path.Clean(strings.ReplaceAll(r, "\\", "/") + rest)
			break
		}
		parent := path.Dir(head)
		if parent == head || parent == "." || (len(parent) == 2 && parent[1] == ':') { // "C:" alone is the drive's current directory, not its root
			break
		}
		rest = "/" + path.Base(head) + rest
		head = parent
	}
	return strings.ToLower(p)
}

// agentList is `herdr agent list`, read: its reply, or why it could not be.
func (o *occupantsReader) agentList() (jmap, string) {
	cmd := exec.Command("herdr", "agent", "list")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, "herdr agent list exited " + itoa(exitStatus(err)) + ": " + trimTR(errb.String(), 200)
	}
	raw := bashSub(out.String())
	v, ok := pyLoads(raw)
	m, isMap := v.(jmap)
	if !ok || !isMap || !eqStr(pyDict(m["result"])["type"], "agent_list") {
		return nil, "herdr agent list exited 0 but its reply is not an agent_list: " + trimTR(raw, 200)
	}
	return m, ""
}

// occupantsIn lists the agents of a reply whose cwd is dir or under it.
func (o *occupantsReader) occupantsIn(reply jmap, dir string) string {
	d := onePath(dir, o.win)
	var lines []string
	for _, a := range pyDicts(pyDict(reply["result"])["agents"]) {
		cwd := ""
		if c, ok := a["cwd"].(string); ok {
			cwd = onePath(c, o.win)
		}
		if cwd == d || strings.HasPrefix(cwd, d+"/") {
			lines = append(lines, jqText(a["pane_id"])+" ("+jqOr(a["agent"], "?")+", "+jqOr(a["agent_status"], "?")+")")
		}
	}
	return strings.Join(lines, "\n")
}

// of asks herdr who is in dir. It asks EVERY time, as bash does for each stray just before
// moving it: an agent that arrived since the previous stray was read must hold this one.
func (o *occupantsReader) of(dir string) occupancy {
	reply, why := o.agentList()
	if reply == nil {
		return occupancy{why: why}
	}
	return occupancy{list: o.occupantsIn(reply, dir), observed: true}
}

// jqText is "\(x)": a string as itself, anything else as JSON.
func jqText(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	}
	return pyJSONCompact(v)
}

func jqOr(v any, def string) string { // (v // def) in an interpolation
	if v == nil {
		return def
	}
	if b, ok := v.(bool); ok && !b {
		return def
	}
	return jqText(v)
}

func pyJSONCompact(v any) string {
	if n, ok := v.(interface{ String() string }); ok {
		return n.String()
	}
	return pyStr(v)
}

func gitOut(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err == nil
}

func (a *app) cmdWorktrees() int {
	migrate, quiet, filter := false, false, ""
	for _, arg := range a.args {
		switch {
		case arg == "--migrate":
			migrate = true
		case arg == "--quiet":
			quiet = true
		case strings.HasPrefix(arg, "-"):
			a.die("unknown option: " + arg + " — usage: hw worktrees [<project>] [--migrate]")
		default:
			filter = a.c.t.canonProject(arg)
		}
	}
	if a.getenv("HW_CORE_READONLY") != "" { // the shim's shadow mode: a verb that moves nothing, never moves anything
		migrate = false
	}
	strays, moved := 0, 0
	occ := &occupantsReader{h: herdrClient{getenv: a.getenv}, win: windowsPaths(a.getenv)}
	for _, proj := range a.c.t.survey {
		main := a.c.checkout(proj)
		if main == "" { // _repo_lanes: a lane with a checkout
			continue
		}
		if filter != "" && filter != proj {
			continue
		}
		wtDir := a.c.lanePath(proj, "worktree", "__probe__")
		if wtDir == "" {
			continue
		}
		root := filepath.Dir(wtDir)
		if !isDir(main) {
			a.warn(proj + ": no checkout at " + main)
			continue
		}
		if !quiet {
			a.printf("\n  %s%s%s  canonical: %s\n", a.cB, proj, a.c0, root)
		}
		listing, _ := gitOut(main, "worktree", "list", "--porcelain")
		for _, line := range strings.Split(listing, "\n") {
			if !strings.HasPrefix(line, "worktree ") {
				continue
			}
			path := line[len("worktree "):]
			if path == main {
				continue
			}
			if strings.HasPrefix(path, root+"/") { // # MUTATION-ANCHOR: 826-G12
				if !quiet {
					a.info(filepath.Base(path) + " — in place")
				}
				continue
			}
			if strings.HasPrefix(path, main+"/.git/") { // # MUTATION-ANCHOR: 826-G26
				if !quiet {
					a.info(filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path) + " — internal (.git), left alone")
				}
				continue
			}
			strays++
			o := occ.of(path)
			dirty := ""
			if st, ok := gitOutErr(path, "status", "--short"); ok {
				n := 0
				for _, l := range strings.Split(strings.TrimRight(st, "\n"), "\n") {
					if l != "" {
						n++
					}
				}
				dirty = itoa(n)
			} else {
				dirty = "unreadable (" + trimTR(st, 80) + ")" // # MUTATION-ANCHOR: 826-G25
			}
			leaf := filepath.Base(path)
			branch, _ := gitOut(path, "rev-parse", "--abbrev-ref", "HEAD")
			branch = bashSub(branch)
			if leaf == filepath.Base(main) && branch != "" && branch != "HEAD" {
				leaf = branch[strings.LastIndexByte(branch, '/')+1:]
			}
			target := root + "/" + leaf
			if quiet {
				a.printf("\n  %sworktrees%s\n", a.cB, a.c0)
			}
			a.warn("OUTSIDE " + proj + "'s canonical root: " + path)
			plural := "ies"
			if dirty == "1" {
				plural = "y"
			}
			branchShown := func() string {
				b, ok := gitOut(path, "rev-parse", "--abbrev-ref", "HEAD")
				if !ok {
					b += "?\n"
				}
				return bashSub(b)
			}
			if strings.HasPrefix(dirty, "unreadable") {
				a.printf("      %sbranch %s · uncommitted entries %s%s\n", a.cDim, branchShown(), dirty, a.c0)
			} else {
				a.printf("      %sbranch %s · %s uncommitted entr%s%s\n", a.cDim, branchShown(), dirty, plural, a.c0)
			}
			if !o.observed { // # MUTATION-ANCHOR: 826-G13
				a.printf("      %sUNDETERMINED — could not read who is working in it: %s%s\n", a.cWarn, o.why, a.c0)
				a.printf("      %sNOT moving it. This is not a verdict that somebody is in it;%s\n", a.cDim, a.c0)
				a.printf("      %sit is the absence of one. Fix what could not be read, then re-run.%s\n", a.cDim, a.c0)
				continue
			}
			if o.list != "" { // # MUTATION-ANCHOR: 826-G14
				a.printf("      %sheld by %s — NOT moving it%s\n", a.cDim, strings.ReplaceAll(o.list, "\n", " "), a.c0)
				a.printf("      %smove it once that pane is free:%s\n", a.cDim, a.c0)
				a.printf("      %sgit -C %s worktree move %s %s%s\n", a.cDim, main, path, target, a.c0)
				continue
			}
			if _, err := os.Stat(target); err == nil { // # MUTATION-ANCHOR: 826-G24
				a.printf("      %s%s already exists — resolve by hand%s\n", a.cDim, target, a.c0)
				continue
			}
			if migrate { // # MUTATION-ANCHOR: 826-G15
				mv := exec.Command("git", "-C", main, "worktree", "move", path, target)
				if mv.Run() == nil {
					a.ok("moved into " + target)
					moved++
				} else {
					a.warn("git worktree move failed — left where it is")
				}
			} else {
				a.printf("      %swould move to %s (add --migrate)%s\n", a.cDim, target, a.c0)
			}
		}
	}
	if quiet && strays == 0 {
		return 0
	}
	a.printf("\n")
	switch {
	case strays == 0:
		a.ok("every worktree is under its repo's canonical root")
	case migrate:
		a.ok("moved " + itoa(moved) + " of " + itoa(strays))
	default:
		a.info(itoa(strays) + " outside the canonical root. Add --migrate to move the free ones.")
	}
	return 0
}

// gitOutErr is `git -C dir <args> 2>file`: stdout on success, the diagnostic on failure.
func gitOutErr(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return errb.String(), false
	}
	return out.String(), true
}
