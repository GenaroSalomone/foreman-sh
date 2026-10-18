package main

// preflight --json (train T2): what hw's own rules say about a dispatch, without doing it. Port of cmd_preflight
// (bin/hw:586-636). It creates, writes and launches nothing — the verdict is a line of JSON on stdout.

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type preflightRule struct{ id, level, msg string }

func (a *app) cmdPreflight() int {
	usage := "usage: hw preflight --json -- <hw argv>"
	json := false
	args := a.args
	var av []string
loop:
	for len(args) > 0 {
		switch args[0] {
		case "--json":
			json = true
			args = args[1:]
		case "--":
			av = args[1:]
			break loop
		default:
			a.die(usage)
		}
	}
	if !json {
		a.die(usage)
	}
	var rules []preflightRule
	first := ""
	if len(av) > 0 {
		first = av[0]
	}
	lane := a.c.t.canonProject(first)
	if a.c.t.known(lane) && len(av) >= 2 && !strings.HasPrefix(av[1], "-") {
		account := a.c.t.get(lane, "account")
		if account == "" {
			account = "default"
		}
		agent := a.c.t.get(lane, "vendor")
		if agent == "" {
			agent = "claude"
		}
		src := "lane default for " + lane
		def := account
		for i := 2; i < len(av); i++ {
			switch av[i] {
			case "--account":
				if i+1 < len(av) {
					account, src = av[i+1], "chosen"
				}
				i++
			case "--agent":
				if i+1 < len(av) {
					agent = av[i+1]
				}
				i++
			case "--sdd": // read by nothing now: gentle's dep row was the only reader
				i++
			}
		}
		if src == "chosen" && account != def {
			verdict, refusal := a.resolveAccount(account, agent)
			var level, why string
			if verdict == "refused" {
				level = "block"
				why = "--account " + account + " differs from the lane default (" + def + ") and hw would refuse it: " + refusal
			} else {
				level = "warn"
				why = "--account " + account + " differs from the lane default (" + def + ")"
			}
			rules = append(rules, preflightRule{"account-override", level, cutRunes(why, 240)}) // # MUTATION-ANCHOR: 871-G01
		}
		if a.cutHolder() {
			rules = append(rules, preflightRule{"cut-running", "warn", "a suite cut is running; the dispatch will queue behind it"}) // # MUTATION-ANCHOR: 871-G02
		}
		// dep-floor: a tool this dispatch needs is older than lib/hw/deps.conf allows. Only the tools the dispatch
		// uses (`needs`), and only one that IS installed: a missing binary is reported where it is needed, as
		// before. An unreadable version is a warn, never a block.
		rows := a.depsRows()
		for _, n := range rows {
			r, _ := depsRow(rows, n.name)
			switch {
			case r.needs == "always", r.needs == "vendor:"+agent:
			default:
				continue
			}
			state, ver := a.depsStatus(r, "")
			level := ""
			switch state {
			case "below":
				level = "block"
			case "unknown":
				level = "warn"
			default:
				continue
			}
			rules = append(rules, preflightRule{"dep-floor", level, cutRunes(depsMessage(a.depsUpdateFor(r, ""), state, ver), 240)})
		}
	}
	// nothing is written, created or launched above or below, but the version cache
	lvl := "ok"
	for _, r := range rules {
		if r.level == "block" {
			lvl = "block" // # MUTATION-ANCHOR: 871-G03
			break
		}
		if r.level == "warn" {
			lvl = "warn"
		}
	}
	var sb strings.Builder
	sb.WriteString(`{"schema":1,"level":`)
	writeJSONString(&sb, lvl)
	sb.WriteString(`,"rules":[`)
	for i, r := range rules {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":`)
		writeJSONString(&sb, r.id)
		sb.WriteString(`,"level":`)
		writeJSONString(&sb, r.level)
		sb.WriteString(`,"msg":`)
		writeJSONString(&sb, r.msg)
		sb.WriteString("}")
	}
	sb.WriteString("]}\n")
	a.printf("%s", sb.String())
	return 0
}

// cutRunes is ${s:0:n}: n characters.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// resolveAccount is _resolve_account's verdict for an account chosen on the command line: "ok", "refused" (with
// the text of the refusal) or "n/a". The parity check is delegated to the account's verifier, as bash delegates it.
func (a *app) resolveAccount(account, agent string) (verdict, refusal string) {
	var dir, verifier string
	switch account {
	case "default":
	case "personal":
		dir, verifier = a.c.home+"/.claude-personal", "claude-personal"
	default:
		return "refused", "unknown account '" + account + "' (known: default, personal)"
	}
	if dir == "" || agent != "claude" {
		return "n/a", ""
	}
	var out []byte
	var err error
	if _, lerr := exec.LookPath(verifier); lerr != nil {
		out = []byte("account " + account + " is verified by `" + verifier + "`, which is not on PATH — so capability parity cannot be established and the account is refused.\n")
		err = lerr
	} else {
		out, err = exec.Command(verifier, "--check").CombinedOutput()
	}
	if err == nil {
		return "ok", ""
	}
	text := strings.TrimRight(string(out), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return "refused", strings.Join(lines, " ")
}

// cutHolder is sg_cut_holder (bin/suite-gate): a live slot of the machine-wide gate belongs to the release cut.
func (a *app) cutHolder() bool {
	// bin/hw hands the gate `SG_WORK="${WORK:-}"`: the lane table's work directory, whatever the environment says
	w := a.c.work
	if w == "" {
		return false
	}
	d := w + "/.locks/suite-gate"
	if !isDir(d) {
		return false
	}
	ents, err := os.ReadDir(d)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "slot.") {
			continue
		}
		k := d + "/" + e.Name()
		if !sgAlive(k) {
			continue
		}
		if strings.HasPrefix(firstLineOf(k+"/owner.cmd"), "release cut ") {
			return true
		}
	}
	return false
}

// firstLineOf is _sg_first_line: the first line of a file ("" when unreadable).
func firstLineOf(path string) string {
	body, ok := readText(path)
	if !ok {
		return ""
	}
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		return body[:i]
	}
	return body
}

// sgAlive is _sg_alive: the owner's pid is alive and started when the slot says it did; a slot with no owner
// record yet is alive for 60 s after it was made.
func sgAlive(d string) bool {
	pid, start := firstLineOf(d+"/owner.pid"), firstLineOf(d+"/owner.start")
	if pid == "" || start == "" {
		var mt int64
		if st, err := os.Stat(d); err == nil {
			mt = st.ModTime().Unix()
		}
		return int64(wallNow())-mt < 60
	}
	n, err := strconv.Atoi(pid)
	if err != nil || !pidAlive(n) {
		return false
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", pid).Output()
	if err != nil {
		out = nil
	}
	return squeezeSpaces(strings.TrimRight(string(out), "\n")) == start
}

// squeezeSpaces is `tr -s ' '`.
func squeezeSpaces(s string) string {
	var sb strings.Builder
	prev := false
	for _, r := range s {
		if r == ' ' {
			if prev {
				continue
			}
			prev = true
		} else {
			prev = false
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
