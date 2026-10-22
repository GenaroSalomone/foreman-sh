package main

// `hw-core brain <args>` is bin/brain: open (or focus) a lane's long-lived brainer, replace its agent in place
// (--reset, --resume, --self), and relaunch the open claude brainers. The bash it ports is bin/brain with the
// functions of bin/project-spaces.sh it sources; every refusal says whether anything was sent, exactly as there.
//
// The exit codes: 0 opened, focused or reset; 64 a usage error; 3 the brainer is in the WRONG LANE; 75 a resume
// that stopped the agent and did not start one; everything else 1.

import (
	"io"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const brainHelp = `brain — open (or focus) a lane's long-lived brainer.

usage: brain <lane> [--opencode|--claude] [--reset [--self] [--force]]
       brain <lane> --resume <session-id>
       brain relaunch [--all | <lane|pane>...] [--dry-run]
       brain                list the brainers that exist
       brain --version      print the version

    brain <lane>            open the lane's brainer, or focus it if it is open
    brain <lane> --reset    replace the agent, keeping the space, tab and pane
    brain <lane> --no-cockpit   open a claude brainer without the cockpit mod
                            (FOREMAN_COCKPIT=0 does the same)
    brain <lane> --resume ID    the same launch, resuming that conversation (a live
                            brainer's id is herdr's agent_session.value)
    brain relaunch          relaunch the open claude brainers with the cockpit, each
                            resuming its own conversation in its own pane; a working
                            pane is skipped and named. Brainers only: executors are
                            not relaunched (the mod is brainer-only by design).
                            A pane whose agent was stopped and did not come back is
                            FAILED, and the command exits non-zero (--resume: 75).

A lane is a name in projects.json, or one of its aliases. Its executors are
launched with ` + "`hw help`" + `.
`

// brainEntry is `hw-core brain <args>` as main reaches it: the real seams.
func brainEntry(args []string, stdout, stderr io.Writer, getenv func(string) string, stdin io.Reader) int {
	b := &brainRun{out: stdout, errw: stderr, getenv: getenv, stdin: stdin}
	b.env = os.Environ()
	b.exec = realBrainExec
	b.sleep = time.Sleep
	b.now = time.Now
	b.isTTY = func() bool {
		in, ok1 := stdin.(*os.File)
		o, ok2 := stdout.(*os.File)
		e, ok3 := stderr.(*os.File)
		return ok1 && ok2 && ok3 && isTerminal(in) && isTerminal(o) && isTerminal(e)
	}
	b.execve = realExecve
	b.detach = realDetach
	b.spawn = realSpawn
	return b.main(args)
}

func (b *brainRun) main(args []string) (rc int) {
	b.a = &app{term: newTerm(b.out, b.errw, false), getenv: func(k string) string { return b.getenv(k) }, stdin: b.stdin}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(exitNow)
			if !ok {
				panic(r)
			}
			rc = e.code
		}
		// the EXIT trap a resume sets once the pane is touched: any status but 0 is 75 from then on
		if b.trap75 && rc != 0 {
			rc = 75
		}
	}()
	b.home = b.getenv("HOME")
	if b.home == "" && runtime.GOOS == "windows" {
		b.home = b.getenv("USERPROFILE")
	}
	b.brain = brainRoot(b.getenv)
	b.binDir = b.getenv("HW_CORE_BIN_DIR")
	if b.binDir == "" {
		b.binDir = b.brain + "/bin"
	}
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	switch first {
	case "-V", "--version":
		b.outf("brain %s\n", b.cliVersion(b.brain))
		return 0
	case "-h", "--help":
		b.outf("%s", brainHelp)
		return 0
	}
	file := b.getenv("HW_PROJECTS_JSON")
	if file == "" {
		file = b.brain + "/projects.json"
	}
	tab, errs := loadTable(file, b.home)
	if tab == nil {
		if diag, ok := b.laneTableDiag(file); ok {
			b.errf("%s", diag)
			return 1
		}
		for _, l := range errs {
			b.errf("%s\n", l)
		}
		b.errf("brain: the lane table did not load — fix %s/projects.json (the reason is above)\n", b.brain)
		return 1
	}
	b.tab = tab
	b.work = b.getenv("WORK")
	if b.work == "" {
		b.work = tab.work
	}
	b.a.c = &ctx{t: tab, brain: b.brain, work: b.work, home: b.home, getenv: func(k string) string { return b.getenv(k) }}

	if first == "__flush" { // brain's own: the detached half of an in-pane start (see launch)
		if len(args) == 3 {
			b.waitThenFlush(args[1], args[2])
		}
		return 0
	}
	if first == "relaunch" {
		return b.relaunch(args)
	}

	proj := ""
	if first != "" {
		if tab.known(first) {
			proj = first
		} else {
			proj = b.laneAlias(first)
		}
	}
	if proj == "" {
		if first == "" {
			b.outf("brainers:\n")
			ents, _ := os.ReadDir(b.brain)
			var names []string
			for _, e := range ents {
				names = append(names, e.Name())
			}
			sort.Strings(names)
			for _, n := range names {
				d := b.brain + "/" + n
				if !dirExists(d) {
					continue
				}
				if st, err := os.Lstat(d); err == nil && st.Mode()&os.ModeSymlink != 0 {
					continue
				}
				if brFileExists(d + "/CLAUDE.md") {
					b.outf("  %s\n", n)
				}
			}
			b.outf("\nusage: brain <%s> [--opencode|--claude] [--reset [--self] [--force]]\n", strings.Join(tab.names, "|"))
			return 0
		}
		b.errf("unknown brainer: %s  (try: %s)\n", first, strings.Join(tab.names, ", "))
		return 64
	}
	b.proj = proj
	return b.launch(args[1:])
}

// laneAlias is `lane_alias brain <word>`: the lane a brain alias names, "" when none.
func (b *brainRun) laneAlias(word string) string {
	if word == "" || strings.Trim(word, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return ""
	}
	return b.tab.aliasB[word]
}

func (b *brainRun) laneKnown(name string) bool { return b.tab.known(name) }

// relaunch is brain_relaunch: each open claude brainer relaunched in place, resuming its own conversation.
func (b *brainRun) relaunch(args []string) int {
	all, dry := false, false
	var targets []string
	for _, a := range args[1:] {
		switch {
		case a == "--all":
			all = true
		case a == "--dry-run":
			dry = true
		case a == "-h" || a == "--help":
			b.outf("usage: brain relaunch [--all | <lane|pane>...] [--dry-run]   (brainers only; executors are not relaunched)\n")
			return 0
		case strings.HasPrefix(a, "-"):
			b.errf("brain relaunch: unknown flag: %s  (try: --all, --dry-run)\n", a)
			return 64
		case matchPanePattern(a):
			targets = append(targets, "pane:"+a)
		default:
			if b.laneKnown(a) {
				targets = append(targets, "lane:"+a)
			} else if al := b.laneAlias(a); al != "" {
				targets = append(targets, "lane:"+al)
			} else {
				b.errf("brain relaunch: neither a lane nor a pane id: %s\n", a)
				return 64
			}
		}
	}
	if !all && len(targets) == 0 {
		b.errf("brain relaunch: name lanes or panes, or pass --all\n")
		return 64
	}
	if all && len(targets) > 0 {
		b.errf("brain relaunch: --all and named targets do not combine\n")
		return 64
	}
	// A FAILED LIST IS NOT AN EMPTY LIST: nothing is relaunched on a herdr that did not answer.
	r := b.herdr(brainCmd{}, "pane", "list")
	listed := r.Out
	v, ok := bjParse(listed)
	if _, isArr := bjGet(v, "result", "panes").([]any); r.RC != 0 || !ok || !isArr {
		b.errf("brain relaunch: herdr did not answer `pane list`, so there is nothing to relaunch from.\n")
		return 1
	}
	list, _ := bjGet(v, "result", "panes").([]any)
	n, fails, rc := 0, 0, 0
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap {
			continue
		}
		if l, _ := p["label"].(string); l != "brain" {
			continue
		}
		if ag, _ := p["agent"].(string); ag != "claude" {
			continue
		}
		pane := bjRaw(p["pane_id"])
		cwd := bjRaw(bjAlt(p["cwd"], ""))
		sid := bjRaw(bjAlt(bjGet(p["agent_session"], "value"), ""))
		st := bjRaw(bjAlt(p["agent_status"], ""))
		if pane == "" {
			continue
		}
		lane := b.laneOfCwd(cwd)
		if !all {
			hit := false
			for _, t := range targets {
				if t == "pane:"+pane || (lane != "" && t == "lane:"+lane) {
					hit = true
				}
			}
			if !hit {
				continue
			}
		}
		n++
		if lane == "" || !b.laneKnown(lane) {
			c := cwd
			if c == "" {
				c = "?"
			}
			b.outf("skipped %s: its cwd (%s) is not a lane directory under %s\n", pane, c, b.brain)
			continue
		}
		if sid == "" {
			b.outf("skipped %s (%s): herdr reports no session id for it, so there is no conversation to resume\n", pane, lane)
			continue
		}
		switch st {
		case "idle", "done":
		default:
			s := st
			if s == "" {
				s = "no status"
			}
			b.outf("skipped %s (%s): %s — a turn may be in flight, and a relaunch would cut it\n", pane, lane, s)
			continue
		}
		_, conv := b.sessionTranscript(sid)
		if dry {
			if conv {
				b.outf("would relaunch %s (%s) resuming %s\n", pane, lane, sid)
			} else {
				b.outf("would relaunch %s (%s) as a NEW conversation: %s has no transcript, so there is nothing to resume\n", pane, lane, sid)
			}
			continue
		}
		res := b.run(brainCmd{Name: b.binDir + "/brain", Args: []string{lane, "--resume", sid, "--in-pane", pane}, Merge: true})
		out := brTrimNL(res.Out)
		rc = res.RC
		if rc == 75 {
			b.outf("FAILED %s (%s): the agent was stopped and did not come back, rc=%d: %s\n", pane, lane, rc, tail2(out))
			rc = 0
			fails++
			continue
		}
		if rc != 0 {
			b.outf("skipped %s (%s): brain refused, rc=%d: %s\n", pane, lane, rc, tail2(out))
			rc = 0
			continue
		}
		// THE PANE ID IS THE RETURN ADDRESS (HW_INVOKER_PANE): proved, not assumed.
		up := false
		for i := 0; i < 10; i++ {
			if b.paneIsClaudeBrainer(pane) {
				up = true
				break
			}
			b.sleep(1 * time.Second)
		}
		if !up {
			how := "a new conversation"
			if conv {
				how = "resuming " + sid
			}
			b.outf("FAILED %s (%s): claude did not come up in the pane (%s) — the pane is a bare shell; `brain %s` starts a new brainer in it\n", pane, lane, how, lane)
			fails++
			continue
		}
		bg := ""
		if m := reBgKept.FindString(out); m != "" {
			bg = " — background tasks moved to the background (processes alive: " + strings.TrimPrefix(m, "background-kept ") + ")"
		}
		mod := ""
		if m := reCockpitFloor.FindString(out); m != "" {
			mod = " — without the mod: " + m
		}
		if conv {
			b.outf("relaunched %s (%s) resuming %s%s%s\n", pane, lane, sid, bg, mod)
		} else {
			b.outf("relaunched %s (%s) as a NEW conversation — %s has no transcript (it never had a turn), so there was nothing to resume%s%s\n", pane, lane, sid, bg, mod)
		}
	}
	if n == 0 {
		b.outf("brain relaunch: no open claude brainer matched.\n")
	}
	if fails != 0 {
		return 1
	}
	return 0
}

func (b *brainRun) paneIsClaudeBrainer(pane string) bool {
	r := b.herdr(brainCmd{}, "pane", "list")
	v, ok := bjParse(r.Out)
	if !ok {
		return false
	}
	list, _ := bjGet(v, "result", "panes").([]any)
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap {
			continue
		}
		if id, _ := p["pane_id"].(string); id == pane {
			l, _ := p["label"].(string)
			a, _ := p["agent"].(string)
			if l == "brain" && a == "claude" {
				return true
			}
		}
	}
	return false
}

// tail2 is `tail -2 | tr '\n' ' '` of a text with no trailing newline.
func tail2(s string) string {
	if s == "" {
		return ""
	}
	l := strings.Split(s, "\n")
	if len(l) > 2 {
		l = l[len(l)-2:]
	}
	return strings.Join(l, " ")
}

// matchPanePattern is the case pattern `*:p*`.
func matchPanePattern(s string) bool { return strings.Contains(s, ":p") }

var reBgKept = regexp.MustCompile(`background-kept [0-9]+/[0-9]+|background-kept unverified`)
var reCockpitFloor = regexp.MustCompile(`cockpit needs claude >= [0-9.]+ \(found [^)]*\)`)
