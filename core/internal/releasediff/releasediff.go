// Package releasediff is the release: prod standard's env comparison, shared by hw (at dispatch) and done-invoker (at
// close). Two sources, both read-only: the NAMES of each app (`env list`, which returns no values) and the
// BOOLEAN values inside each backend container (printenv, filtered on the host to true/false/0/1: nothing else leaves it).
// Everything it returns or prints is a name and "missing"/"differs"; a value never crosses this package's boundary
// except true/false/0/1, and those are compared, never reported.
package releasediff

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Spec is what hw pins in the receipt (key release_t<N>) at dispatch and done-invoker re-reads at close.
type Spec struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	Ruled   map[string]string `json:"env_diff"` // the brief's rulings: NAME -> needed|not-needed
	Screens []string          `json:"screens"`
	QALane  string            `json:"qa_lane"`
	QATask  string            `json:"qa_task"`
	Start   int64             `json:"start"`
}

// Cfg names the two external commands. Tests stub them with HW_RELEASE_ENVLIST / HW_RELEASE_SSH (whitespace-split prefixes).
type Cfg struct {
	EnvList []string // run as: <EnvList...> env list <uuid>
	SSH     []string // run as: <SSH...> '<remote command>'
	Host    string   // what an authorizes: destination must name to cover the SSH read
}

// FromEnv reads the two commands from HW_RELEASE_ENVLIST / HW_RELEASE_SSH, else from <brain>/setup/release-prod.env (local to a
// machine, not exported: it names the host and the key). {brain} and $HOME expand. Neither set: the diff cannot run, and that refuses.
func FromEnv(brain string) Cfg {
	kv := map[string]string{}
	if b, err := os.ReadFile(brain + "/setup/release-prod.env"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(l, "="); ok && !strings.HasPrefix(k, "#") {
				kv[strings.TrimSpace(k)] = v
			}
		}
	}
	get := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			v = kv[k]
		}
		return os.ExpandEnv(strings.ReplaceAll(v, "{brain}", brain))
	}
	return Cfg{EnvList: strings.Fields(get("HW_RELEASE_ENVLIST")), SSH: strings.Fields(get("HW_RELEASE_SSH")), Host: get("HW_RELEASE_HOST")}
}

var (
	uuidRe = regexp.MustCompile(`^[a-z0-9]+$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	envRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func run(argv []string, extra ...string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("HW_RELEASE_ENVLIST / HW_RELEASE_SSH are not configured (env, or setup/release-prod.env)")
	}
	cmd := exec.Command(argv[0], append(append([]string{}, argv[1:]...), extra...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s failed: %v", argv[0], err)
	}
	return string(out), nil
}

func names(c Cfg, uuid string) (map[string]string, error) {
	out, err := run(c.EnvList, "env", "list", uuid)
	if err != nil {
		return nil, fmt.Errorf("env list %s: %v", uuid, err)
	}
	set := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		n, _, _ := strings.Cut(strings.TrimSpace(l), "=")
		if envRe.MatchString(n) {
			set[n] = ""
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("env list %s returned no names", uuid)
	}
	return set, nil
}

// Container resolves backend-<uuid>-<suffix> from a `docker ps` name list: exactly one match, or a refusal naming the candidates.
func Container(psOut, uuid string) (string, error) {
	var m []string
	for _, l := range strings.Fields(psOut) {
		// the suffix is the deployment id (digits); a preview deployment (-pr-N) is not the app's env. Measured live 2026-11-09.
		if suf, ok := strings.CutPrefix(l, "backend-"+uuid+"-"); ok && suf != "" && strings.Trim(suf, "0123456789") == "" {
			m = append(m, l)
		}
	}
	if len(m) != 1 {
		return "", fmt.Errorf("app %s: %d backend containers match backend-%s-<deployment id> (%s); exactly one is required", uuid, len(m), uuid, strings.Join(m, ", "))
	}
	return m[0], nil
}

const boolFilter = `awk '{i=index($0,"="); if(i<2)next; v=tolower(substr($0,i+1)); if(v=="true"||v=="false"||v=="0"||v=="1")print substr($0,1,i-1)"="v}'`

// readBools runs on the host: the exec's own status first (a pipe into awk would report awk's), then "#<entries>", then the booleans.
const readBools = `e=$(docker exec %s printenv) || exit 97; printf '#%%s\n' "$(printf '%%s\n' "$e" | wc -l)"; printf '%%s\n' "$e" | ` + boolFilter

func bools(c Cfg, uuid string) (map[string]string, error) {
	ps, err := run(c.SSH, "docker ps --format '{{.Names}}'")
	if err != nil {
		return nil, err
	}
	name, err := Container(ps, uuid)
	if err != nil {
		return nil, err
	}
	// The exec's own status is checked (a pipe into awk would report awk's), and its first output line is "#<entries>": a read
	// that fails, or comes back without that line or without one boolean, is a refusal, never a quiet names-only diff.
	out, err := run(c.SSH, fmt.Sprintf(readBools, name))
	if err != nil {
		return nil, fmt.Errorf("app %s: reading the environment of %s failed on the host (%v)", uuid, name, err)
	}
	head, body, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(head, "#") {
		return nil, fmt.Errorf("app %s: the host's answer for %s carries no entry count; the boolean read is not trusted", uuid, name)
	}
	set := map[string]string{}
	for _, l := range strings.Split(body, "\n") {
		if n, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && envRe.MatchString(n) {
			set[n] = v
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("app %s: %s has %s environment entries and none is true/false/0/1; the boolean read is not trusted", uuid, name, strings.TrimPrefix(head, "#"))
	}
	return set, nil
}

// Diff is name -> "missing"|"differs" between two apps. A name present on one side only is missing; a boolean both sides carry
// with another value differs. Non-boolean values are compared by presence only (KNOWN-LIMITATIONS).
func Diff(c Cfg, from, to string) (map[string]string, error) {
	for _, u := range []string{from, to} {
		if !uuidRe.MatchString(u) {
			return nil, fmt.Errorf("%q is not an app uuid", u)
		}
	}
	nf, err := names(c, from)
	if err != nil {
		return nil, err
	}
	nt, err := names(c, to)
	if err != nil {
		return nil, err
	}
	bf, err := bools(c, from)
	if err != nil {
		return nil, err
	}
	bt, err := bools(c, to)
	if err != nil {
		return nil, err
	}
	d := map[string]string{}
	for n := range nf {
		if _, ok := nt[n]; !ok {
			d[n] = "missing"
		}
	}
	for n := range nt {
		if _, ok := nf[n]; !ok {
			d[n] = "missing"
		}
	}
	for n, v := range bf {
		if w, ok := bt[n]; !ok {
			d[n] = "missing"
		} else if v != w && d[n] == "" {
			d[n] = "differs"
		}
	}
	for n := range bt {
		if _, ok := bf[n]; !ok {
			d[n] = "missing"
		}
	}
	return d, nil
}

// Rulings parses `{NAME: needed, OTHER: not-needed}`.
func Rulings(v string) (map[string]string, error) {
	r := map[string]string{}
	for _, p := range strings.Split(strings.Trim(strings.TrimSpace(v), "{}"), ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		k, val, ok := strings.Cut(p, ":")
		k, val = strings.TrimSpace(k), strings.TrimSpace(val)
		if !ok || !envRe.MatchString(k) || (val != "needed" && val != "not-needed") {
			return nil, fmt.Errorf("env-diff entry %q is not NAME: needed|not-needed", strings.TrimSpace(p))
		}
		r[k] = val
	}
	return r, nil
}

func sorted(m map[string]string, keep func(k string) bool) []string {
	var o []string
	for k := range m {
		if keep(k) {
			o = append(o, k)
		}
	}
	sort.Strings(o)
	return o
}

func lines(d map[string]string, ks []string) string {
	var b strings.Builder
	for _, k := range ks {
		b.WriteString("\n  " + k + " " + d[k])
	}
	return b.String()
}

// CheckDispatch: every differing name is ruled, and every ruled name still differs.
func CheckDispatch(d, ruled map[string]string) error {
	if un := sorted(d, func(k string) bool { _, ok := ruled[k]; return !ok }); len(un) > 0 {
		return fmt.Errorf("env differs between env-from and env-to and nobody ruled on it; add each to env-diff: {NAME: needed|not-needed}:%s", lines(d, un))
	}
	if st := sorted(ruled, func(k string) bool { _, ok := d[k]; return !ok }); len(st) > 0 {
		return fmt.Errorf("env-diff rules on names that no longer differ (stale ruling; remove them): %s", strings.Join(st, ", "))
	}
	return nil
}

// CheckClose: nothing ruled `needed` may still differ.
func CheckClose(d, ruled map[string]string) error {
	if n := sorted(d, func(k string) bool { return ruled[k] == "needed" }); len(n) > 0 {
		return fmt.Errorf("env ruled `needed` still differs between env-from and env-to; set it on the target (by a person) and re-run:%s", lines(d, n))
	}
	return nil
}

// CheckRehearsal: the rehearsal's env-from against the release's env-to is empty, or every difference is ruled not-needed.
func CheckRehearsal(d, ruled map[string]string) error {
	if bad := sorted(d, func(k string) bool { return ruled[k] != "not-needed" }); len(bad) > 0 {
		return fmt.Errorf("a rehearsal on an env that differs from the release's target proves another env's numbers; differing and not ruled not-needed in the release brief:%s", lines(d, bad))
	}
	return nil
}

// Authorized: the brief's authorizes: entries cover the SSH host, printenv, and both apps.
func Authorized(authorizes, host string, uuids ...string) bool {
	if host == "" {
		return false
	}
	for _, e := range strings.Split(authorizes, "\n") {
		dest, rest, _ := strings.Cut(e, " :: ")
		op, _, _ := strings.Cut(rest, " :: ")
		ok := strings.Contains(dest, host) && strings.Contains(op, "printenv")
		for _, u := range uuids {
			ok = ok && strings.Contains(dest, u)
		}
		if ok {
			return true
		}
	}
	return false
}

// CheckScreens: each name has <dir>/<name>.png newer than since. Existence only, never correctness.
func CheckScreens(dir string, screens []string, since time.Time) error {
	var bad []string
	for _, s := range screens {
		fi, err := os.Stat(dir + "/" + s + ".png")
		if err != nil || !fi.Mode().IsRegular() || !fi.ModTime().After(since) {
			bad = append(bad, s)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("no screen newer than the run start for: %s (expected %s/<name>.png)", strings.Join(bad, ", "), dir)
	}
	return nil
}

// ValidScreen refuses a name that could leave the screens directory.
func ValidScreen(s string) bool {
	return s != "" && !strings.HasPrefix(s, "/") && !strings.Contains(s, "..") && !strings.ContainsAny(s, " \t\n")
}
