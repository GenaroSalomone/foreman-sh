package invokers

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// HerdrLeak is one herdr process of an isolated instance that is still alive.
type HerdrLeak struct {
	Pid    int
	Comm   string
	Socket string // the unix socket (or, when only the cwd matched, the cwd) that tied it to the run
}

// leakScope is what ties a process to THIS run: the directories the run owns.
type leakScope struct {
	Tmpdir     string   // the run's TMPDIR
	Workdir    string   // the run's work directory (HW_WORKDIR)
	Home       string   // the person's HOME: its herdr dirs are never reported
	Registered []string // instance dirs `herdr-isolated start` recorded in $HW_ARTIFACTS/tmp/herdr-instances
}

// resolved is a path and, when it differs, its symlink-resolved form (/tmp is /private/tmp on macOS).
func resolved(p string) []string {
	p = filepath.Clean(p)
	out := []string{p}
	if r, err := filepath.EvalSymlinks(p); err == nil && r != p {
		out = append(out, r)
	}
	return out
}

func under(p string, dirs []string) bool {
	for _, d := range dirs {
		if d != "" && d != "/" && (p == d || strings.HasPrefix(p, d+"/")) {
			return true
		}
	}
	return false
}

func herdrComm(comm string) bool {
	switch comm {
	case "herdr-core", "hc", "Herdr", "herdr-app", "herdr":
		return true
	}
	return false
}

// parseLsofF reads `lsof -F pcn` output into pid → (command, names), in pid order.
func parseLsofF(out string) (pids []int, comm map[int]string, names map[int][]string) {
	comm, names = map[int]string{}, map[int][]string{}
	cur := 0
	for _, l := range strings.Split(out, "\n") {
		if len(l) < 2 {
			continue
		}
		switch l[0] {
		case 'p':
			n, err := strconv.Atoi(l[1:])
			if err != nil {
				cur = 0
				continue
			}
			cur = n
			pids = append(pids, n)
		case 'c':
			if cur != 0 {
				comm[cur] = l[1:]
			}
		case 'n':
			if cur != 0 && strings.HasPrefix(l[1:], "/") { // a path; "->0x…" is the far end of a socketpair
				names[cur] = append(names[cur], l[1:])
			}
		}
	}
	return
}

// HerdrLeaks names the herdr processes of an isolated instance this run left alive. sockets is `lsof -nP -U -F pcn` and cwds is
// `lsof -nP -d cwd -F pcn`. A process counts when it holds a unix socket under the run's TMPDIR or under an instance dir the run
// registered, or when it is an herdr binary whose cwd is under the run's work directory or TMPDIR (a hand-built instance). The
// person's own core (HOME/.local/state/herdr-core, HOME/.config/herdr-app) never does, and neither does a process that holds
// only a socket somewhere else: another executor's instance is not this run's.
func HerdrLeaks(sockets, cwds string, s leakScope) []HerdrLeak {
	var own, mine []string
	if s.Home != "" {
		for _, d := range []string{".local/state/herdr-core", ".local/share/herdr-core", ".config/herdr-app", ".config/herdr"} {
			own = append(own, resolved(filepath.Join(s.Home, d))...)
		}
	}
	tmp := s.Tmpdir
	switch filepath.Clean(tmp) { // a shared temp root is every executor's, not this run's
	case "", ".", "/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp":
		tmp = ""
	}
	if tmp != "" {
		mine = append(mine, resolved(tmp)...)
	}
	for _, d := range s.Registered {
		mine = append(mine, resolved(d)...)
	}
	var owned []string
	if tmp != "" {
		owned = resolved(tmp)
	}
	if s.Workdir != "" && s.Workdir != "<unknown>" {
		owned = append(owned, resolved(s.Workdir)...)
	}

	found := map[int]HerdrLeak{}
	pids, comm, names := parseLsofF(sockets)
	personal := map[int]bool{} // a process holding any socket in the person's own herdr dirs is theirs, whatever its cwd
	for _, p := range pids {
		for _, n := range names[p] {
			if under(n, own) {
				personal[p] = true
			}
		}
	}
	for _, p := range pids {
		for _, n := range names[p] {
			if personal[p] || !herdrComm(comm[p]) || under(n, own) || !under(n, mine) {
				continue
			}
			found[p] = HerdrLeak{Pid: p, Comm: comm[p], Socket: n}
			break
		}
	}
	cp, ccomm, cnames := parseLsofF(cwds)
	for _, p := range cp {
		if _, ok := found[p]; ok || personal[p] || !herdrComm(ccomm[p]) {
			continue
		}
		for _, n := range cnames[p] {
			if under(n, own) || !under(n, owned) {
				continue
			}
			found[p] = HerdrLeak{Pid: p, Comm: ccomm[p], Socket: "cwd " + n}
			break
		}
	}
	out := make([]HerdrLeak, 0, len(found))
	for _, l := range found {
		if l.Pid == os.Getpid() {
			continue
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pid < out[j].Pid })
	return out
}

// HerdrLeakWarning is the one warning line (empty when nothing leaks).
func HerdrLeakWarning(leaks []HerdrLeak) string {
	if len(leaks) == 0 {
		return ""
	}
	var parts []string
	for _, l := range leaks {
		parts = append(parts, "pid "+strconv.Itoa(l.Pid)+" ("+l.Comm+") "+l.Socket)
	}
	return strconv.Itoa(len(leaks)) + " herdr process(es) of an isolated instance this run started are still alive: " + strings.Join(parts, "; ") +
		" — stop the instance with `herdr-isolated stop <its directory>` (it finds the core by its socket directory); nothing was killed"
}

// registeredInstances reads $HW_ARTIFACTS/tmp/herdr-instances, one directory per line.
func registeredInstances(artifacts string) []string {
	b, err := os.ReadFile(filepath.Join(artifacts, RunTmpDir, "herdr-instances"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "/") {
			out = append(out, l)
		}
	}
	return out
}

// herdrLeakWarning scans the machine's unix sockets and herdr cwds (two lsof calls) and returns the warning for this run; "" when
// lsof is absent or nothing leaks. It never fails a close.
func (c *Ctx) herdrLeakWarning(workdir, artifacts string) string {
	if _, ok := c.lookPath("lsof"); !ok {
		return ""
	}
	socks, _ := c.out("lsof", "-nP", "-U", "-F", "pcn")
	cwds, _ := c.out("lsof", "-nP", "-d", "cwd", "-F", "pcn")
	return HerdrLeakWarning(HerdrLeaks(socks, cwds, leakScope{
		Tmpdir: c.tmpdir(), Workdir: workdir, Home: c.get("HOME"), Registered: registeredInstances(artifacts),
	}))
}
