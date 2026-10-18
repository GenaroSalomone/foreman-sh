package invokers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// wantedKeys is what an env file may hand an invoker: ENVFIND's WANTED, which is
// also invoker_adopt_env_file's whitelist.
var wantedKeys = []string{
	"HW_PROJECT", "HW_TASK", "HW_WORKDIR", "HW_RUN",
	"HW_ARTIFACTS", "HW_INVOKER_PANE", "ENGRAM_PROJECT", "HW_EXECUTOR_VENDOR",
	"HW_INVOKER_VENDOR", "HW_INVOKER_SESSION", "HW_INVOKER_ENDPOINT",
	"HW_CHAINING_ENABLED",
	"HW_CHAINING_LEASE_SECONDS", "HW_CHAINING_LEASE_REASON",
	"HW_CHAINING_LEASE_HOLDER",
}

const maxHops = 12

func (c *Ctx) brainRoot() string {
	if v := c.get("HW_BRAIN_ROOT"); v != "" {
		return v
	}
	return filepath.Dir(c.BinDir)
}

// loadRoots is the top of invoker-common.sh: INVOKER_ENV_ROOTS, derived from
// project-spaces.sh (its lane_config_load and all_lane_worktree_roots), else
// read straight from the lane table.
func (c *Ctx) loadRoots() {
	c.envRoots = ""
	if st, err := os.Stat(c.BinDir + "/project-spaces.sh"); err == nil && !st.IsDir() {
		const script = `set -euo pipefail
INVOKER_BIN_DIR="$1"
. "$INVOKER_BIN_DIR/project-spaces.sh" \
  || printf 'invoker-common: %s/project-spaces.sh did not load (exit %s); the lane roots fall back to reading the lane table directly\n' "$INVOKER_BIN_DIR" "$?" >&2
if command -v lane_config_load >/dev/null 2>&1 \
   && lane_config_load "${HW_BRAIN_ROOT:-$(dirname "$INVOKER_BIN_DIR")}" 2>/dev/null \
   && command -v all_lane_worktree_roots >/dev/null 2>&1; then
  _derived_roots="$(all_lane_worktree_roots 2>/dev/null || true)"
  printf '%s\0' "${HW_OPERATOR:-}" >&3
  if [ -n "$_derived_roots" ]; then
    printf '%s\n%s' "$WORK" "$_derived_roots" >&3
  fi
fi`
		r, w, err := os.Pipe()
		if err == nil {
			done := make(chan string)
			go func() {
				var b bytes.Buffer
				b.ReadFrom(r)
				done <- b.String()
			}()
			c.run(runOpts{stdout: c.Stdout, stderr: c.Stderr, extra: []*os.File{w}}, "bash", "-c", script, "_", c.BinDir)
			w.Close()
			got := <-done
			r.Close()
			if i := strings.IndexByte(got, 0); i >= 0 {
				c.operatorDefault = got[:i]
				got = got[i+1:]
			}
			c.envRoots = got
		}
	}
	if c.envRoots == "" {
		c.envRoots = c.rootsFromTable() + "\n"
	}
}

func expandHome(home, p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return home + p[1:]
	}
	return p
}

// rootsFromTable is _invoker_roots_from_table: the lane table read directly.
func (c *Ctx) rootsFromTable() string {
	path := c.or("HW_PROJECTS_JSON", c.brainRoot()+"/projects.json")
	work := c.get("WORK")
	var doc map[string]any
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if jerr := json.Unmarshal(b, &doc); jerr != nil {
			c.errf("invoker-common: the lane table %s could not be read (%s); env roots are only the work directory\n", path, pyJSONError(b, jerr))
			doc = nil
		}
	case os.IsNotExist(err):
	default:
		c.errf("invoker-common: the lane table %s could not be read (%s); env roots are only the work directory\n", path, pyOSError(err))
	}
	home := c.get("HOME")
	if work == "" {
		if w, ok := doc["work"].(string); ok {
			work = expandHome(home, w)
		}
	}
	var roots []string
	if work != "" {
		roots = append(roots, work)
	}
	lanes, _ := doc["lanes"].(map[string]any)
	names := make([]string, 0, len(lanes))
	for n := range lanes {
		names = append(names, n)
	}
	// python iterates the dict in file order; Go maps lose it, so re-read the keys in order
	names = laneOrder(b, names)
	for _, lane := range names {
		v, _ := lanes[lane].(map[string]any)
		wr, _ := v["worktree_root"].(string)
		if wr == "" {
			continue
		}
		wr = strings.ReplaceAll(expandHome(home, wr), "{work}", work)
		wr = strings.ReplaceAll(wr, "{lane}", lane)
		if strings.Contains(wr, "{checkout}") {
			co, _ := v["checkout"].(string)
			if co == "" {
				continue
			}
			wr = strings.ReplaceAll(wr, "{checkout}", expandHome(home, co))
		}
		if strings.Contains(wr, "{") || (work != "" && (wr == work || strings.HasPrefix(wr, work+"/"))) {
			continue
		}
		dup := false
		for _, r := range roots {
			dup = dup || r == wr
		}
		if !dup {
			roots = append(roots, wr)
		}
	}
	return strings.Join(roots, "\n")
}

// laneOrder returns the keys of the "lanes" object in the order the file has them.
func laneOrder(doc []byte, fallback []string) []string {
	if len(doc) == 0 {
		return fallback
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fallback
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return fallback
		}
		if kt.(string) != "lanes" {
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return fallback
			}
			continue
		}
		if t, err := dec.Token(); err != nil || t != json.Delim('{') {
			return fallback
		}
		var names []string
		for dec.More() {
			lt, err := dec.Token()
			if err != nil {
				return fallback
			}
			names = append(names, lt.(string))
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return fallback
			}
		}
		seen := map[string]bool{}
		var uniq []string
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				uniq = append(uniq, n)
			}
		}
		sort.SliceStable(uniq, func(i, j int) bool { return false })
		return uniq
	}
	return fallback
}

func pyOSError(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		err = pe.Err
	}
	if en, ok := err.(syscall.Errno); ok {
		return "[Errno " + itoa(int(en)) + "] " + capitalize(en.Error())
	}
	return err.Error()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// pyJSONError approximates json.JSONDecodeError's text ("msg: line L column C (char N)").
func pyJSONError(doc []byte, err error) string {
	off := 0
	msg := err.Error()
	if se, ok := err.(*json.SyntaxError); ok {
		off = int(se.Offset)
		msg = se.Error()
	}
	line, col := 1, 1
	for i := 0; i < off && i < len(doc); i++ {
		if doc[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return msg + ": line " + itoa(line) + " column " + itoa(col) + " (char " + itoa(off) + ")"
}

func (c *Ctx) canon(p string) string {
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(c.Cwd, abs)
	}
	abs = filepath.Clean(abs)
	resolved := abs
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = r
			break
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			break
		}
		rest = "/" + filepath.Base(resolved) + rest
		resolved = parent
	}
	return strings.ToLower(strings.TrimRight(resolved+rest, "/"))
}

func (c *Ctx) underARoot(path string) bool {
	p := c.canon(path)
	for _, root := range strings.Split(c.envRoots, "\n") {
		if root == "" {
			continue
		}
		r := c.canon(root)
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// usableEnv: owned by us, a regular file, not group- or world-writable.
func usableEnv(path string) bool {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	if ownedByOther(st) {
		return false
	}
	return st.Mode().Perm()&0o022 == 0
}

// candidates are the run env files of a work directory, newest run first.
func candidates(dir string) []string {
	hw := filepath.Join(dir, ".hw")
	ents, err := os.ReadDir(hw)
	if err != nil {
		return nil
	}
	type cand struct {
		mt   int64
		path string
	}
	var found []cand
	for _, e := range ents {
		p := filepath.Join(hw, e.Name(), "env")
		if usableEnv(p) {
			if st, err := os.Stat(p); err == nil {
				found = append(found, cand{st.ModTime().UnixNano(), p})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].mt != found[j].mt {
			return found[i].mt > found[j].mt
		}
		return found[i].path > found[j].path
	})
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out
}

// parseEnvFile is ENVFIND's parse: bin/runenv --lenient reads it, the WANTED
// keys are kept in WANTED order. A file runenv calls unusable is named.
func (c *Ctx) parseEnvFile(path string) (keys []string, vals map[string]string) {
	var out, errb bytes.Buffer
	rc := c.run(runOpts{stdout: &out, stderr: &errb}, c.BinDir+"/runenv", "dump", "--nul", "--lenient", path)
	if rc == 2 {
		msg := strings.TrimSuffix(errb.String(), "\n")
		msg = strings.TrimPrefix(msg, "runenv: ")
		c.errf("invoker-common: %s\n", msg)
		return nil, nil
	}
	if rc != 0 {
		return nil, nil
	}
	all := map[string]string{}
	for _, rec := range strings.Split(out.String(), "\x00") {
		if i := strings.IndexByte(rec, '='); i > 0 {
			all[rec[:i]] = rec[i+1:]
		}
	}
	vals = map[string]string{}
	for _, k := range wantedKeys {
		if v, ok := all[k]; ok {
			vals[k] = v
			keys = append(keys, k)
		}
	}
	return keys, vals
}

// envFileSearch is the python of invoker_adopt_env_file: the file adopted and its
// values, or none.
func (c *Ctx) envFileSearch() (string, []string, map[string]string) {
	if rooted := c.get("HW_WORKDIR"); rooted != "" {
		if !c.underARoot(rooted) {
			return "", nil, nil
		}
		var paths []string
		if run := c.get("HW_RUN"); run != "" {
			if p := filepath.Join(rooted, ".hw", run, "env"); usableEnv(p) {
				paths = []string{p}
			}
		} else {
			paths = candidates(rooted)
		}
		for _, p := range paths {
			if keys, vals := c.parseEnvFile(p); len(vals) > 0 {
				return p, keys, vals
			}
		}
		return "", nil, nil
	}
	dir := c.Cwd
	for i := 0; i < maxHops; i++ {
		if !c.underARoot(dir) {
			break
		}
		for _, p := range candidates(dir) {
			if keys, vals := c.parseEnvFile(p); len(vals) > 0 {
				return p, keys, vals
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil, nil
}

// adoptEnvFile is invoker_adopt_env_file: fills only what is unset or empty.
func (c *Ctx) adoptEnvFile() {
	found, keys, vals := c.envFileSearch()
	for _, k := range keys {
		if c.get(k) != "" {
			continue
		}
		c.Env[k] = vals[k]
		if k == "HW_INVOKER_PANE" {
			c.paneFromFile = true
		}
	}
	c.envFile = found
}

// paneSourceNote is invoker_pane_source_note.
func (c *Ctx) paneSourceNote() string {
	if !c.paneFromFile {
		return ""
	}
	return " That pane id came from " + c.envFile + ", which was written before this pane lost its environment: pane ids do survive a herdr restart, but a brainer pane that was closed and recreated has a different id and no file can know that. herdr answers \"no such pane\" by closing the connection, so it is indistinguishable from \"herdr is gone\" — this cannot be narrowed further from here."
}
