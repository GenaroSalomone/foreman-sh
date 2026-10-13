package main

// The version floors of the outside tools (lib/hw/deps.conf) and the one check against them: a port of
// lib/hw/deps.sh as `hw preflight` uses it (deps_names, deps_row, deps_status, deps_message, deps_version_at_least).
// The version is cached by the binary's path, mtime and size ($XDG_CACHE_HOME/hw/deps/<name>) exactly as bash caches
// it, so the two implementations read and write one cache; the FLOOR is never cached.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type depRow struct{ name, min, match, cmd, needs, basis, update string }

// depsConfPath is $DEPS_CONF: HW_DEPS_CONF, else deps.conf beside lib/hw/deps.sh (bin/../lib/hw).
func (a *app) depsConfPath() string {
	if p := a.getenv("HW_DEPS_CONF"); p != "" {
		return p
	}
	if d := a.getenv("HW_CORE_BIN_DIR"); d != "" {
		return filepath.Join(d, "..", "lib", "hw", "deps.conf")
	}
	return ""
}

// depsRows is the manifest in file order: one row per line that is not blank or a comment, fields split on '|'
// (the last takes the rest of the line, as `IFS='|' read` gives it).
func (a *app) depsRows() []depRow {
	p := a.depsConfPath()
	if p == "" {
		return nil
	}
	body, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	// `while read … done < file` never sees a last line that has no newline
	text := string(body)
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	} else {
		text = ""
	}
	var rows []depRow
	for _, line := range strings.Split(text, "\n") {
		f := strings.SplitN(line, "|", 7)
		for len(f) < 7 {
			f = append(f, "")
		}
		if f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		rows = append(rows, depRow{f[0], f[1], f[2], f[3], f[4], f[5], f[6]})
	}
	return rows
}

// depsRow is deps_row: the FIRST row of that name.
func depsRow(rows []depRow, name string) (depRow, bool) {
	for _, r := range rows {
		if r.name == name {
			return r, true
		}
	}
	return depRow{}, false
}

// depsAtLeast is deps_version_at_least: A >= B, three numeric parts, a missing part is 0.
func depsAtLeast(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, y := depsPart(pa, i), depsPart(pb, i)
		if x > y {
			return true
		}
		if x < y {
			return false
		}
	}
	return true
}

func depsPart(p []string, i int) int64 {
	if i >= len(p) || p[i] == "" {
		return 0
	}
	n, _ := strconv.ParseInt(p[i], 10, 64)
	return n
}

var reDepsVersion = regexp.MustCompilePOSIX(`[0-9]+\.[0-9]+(\.[0-9]+)?`)

// lookPath is `type -P`: the first executable regular file of that name on PATH (an empty entry is the current
// directory); a name with a slash is itself.
func (a *app) lookPath(name string) string {
	if name == "" {
		return ""
	}
	exe := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
	}
	if strings.Contains(name, "/") {
		if exe(name) {
			return name
		}
		return ""
	}
	for _, d := range strings.Split(a.getenv("PATH"), ":") {
		if d == "" {
			d = "."
		}
		if p := d + "/" + name; exe(p) {
			return p
		}
	}
	return ""
}

// depsStatus is deps_status: DEPS_STATE (ok | below | absent | unknown) and DEPS_VERSION for a row. bin names the
// binary to ask ("" = the first word of the row's command, on PATH).
func (a *app) depsStatus(r depRow, bin string) (state, version string) {
	words := strings.Fields(r.cmd)
	cmd0 := ""
	var args []string
	if len(words) > 0 {
		cmd0, args = words[0], words[1:]
	}
	if bin == "" {
		bin = a.lookPath(cmd0)
	}
	if bin == "" {
		return "absent", ""
	}
	if st, err := os.Stat(bin); err != nil || st.Mode()&0o111 == 0 {
		return "absent", ""
	}
	key := ""
	if st, err := os.Stat(bin); err == nil {
		key = strconv.FormatInt(st.ModTime().Unix(), 10) + "-" + strconv.FormatInt(st.Size(), 10)
	}
	home := a.getenv("HOME")
	if home == "" {
		home = "/nonexistent"
	}
	cache := a.getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = home + "/.cache"
	}
	cdir := cache + "/hw/deps"
	cfile := cdir + "/" + r.name
	if a.getenv("DEPS_NO_CACHE") != "" {
		key = "" // no key: nothing read, nothing written
	}
	ver := ""
	if key != "" {
		if body, ok := readText(cfile); ok {
			line := body
			if i := strings.IndexByte(line, '\n'); i >= 0 {
				line = line[:i]
			}
			if f := strings.SplitN(line, "\t", 4); len(f) == 4 && f[0] == bin && f[1] == key && f[2] == r.cmd {
				ver = f[3]
			}
		}
	}
	if ver == "" {
		// the 10 s ceiling is only there where `timeout` is (stock macOS has none), as in deps.sh
		ctx, cancel := context.Background(), context.CancelFunc(func() {})
		if a.lookPath("timeout") != "" {
			ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		}
		c := exec.CommandContext(ctx, bin, args...)
		c.Stdin = nil // /dev/null: a tool that reads stdin must not wait on the caller's
		out, _ := c.CombinedOutput()
		cancel()
		lines := strings.SplitAfter(string(out), "\n")
		if len(lines) > 5 {
			lines = lines[:5]
		}
		ver = reDepsVersion.FindString(strings.Join(lines, ""))
		if ver == "" {
			return "unknown", ""
		}
		// bash writes this cache; under shadow bash already did, and Go only reads
		if key != "" && a.getenv("HW_CORE_READONLY") == "" && os.MkdirAll(cdir, 0o777) == nil {
			tmp := cfile + "." + strconv.Itoa(os.Getpid())
			if os.WriteFile(tmp, []byte(bin+"\t"+key+"\t"+r.cmd+"\t"+ver+"\n"), 0o666) == nil {
				if os.Rename(tmp, cfile) != nil {
					os.Remove(tmp)
				}
			}
		}
	}
	if r.match == "exact" {
		if depsAtLeast(ver, r.min) && depsAtLeast(r.min, ver) {
			return "ok", ver
		}
	} else if depsAtLeast(ver, r.min) {
		return "ok", ver
	}
	return "below", ver
}

// depsUpdateFor is _deps_update_for: the update command depends on HOW claude was installed, so it is read from
// the binary (symlinks followed), not assumed. `claude update` is the native installer's own command and does
// nothing for a Homebrew install; the stable cask trails the floor, so it is replaced by @latest. Any other path,
// and any other tool, keeps the manifest's command. bin names the binary asked ("" = the row's command on PATH).
func (a *app) depsUpdateFor(r depRow, bin string) depRow {
	if r.name != "claude" {
		return r
	}
	if bin == "" {
		if w := strings.Fields(r.cmd); len(w) > 0 {
			bin = a.lookPath(w[0])
		}
	}
	if bin == "" {
		return r
	}
	real := bin
	if p, err := filepath.EvalSymlinks(bin); err == nil {
		real = p
	}
	switch {
	case strings.Contains(real, "/Caskroom/claude-code@latest/"):
		r.update = "brew upgrade --cask claude-code@latest"
	case strings.Contains(real, "/Caskroom/claude-code/"):
		r.update = "brew uninstall --cask claude-code && brew install --cask claude-code@latest"
	}
	return r
}

// depsMessage is deps_message for the state depsStatus returned.
func depsMessage(r depRow, state, ver string) string {
	switch state {
	case "below":
		if r.match == "exact" {
			return r.name + " " + ver + " is not the pinned " + r.min + "; fix: " + r.update
		}
		return r.name + " " + ver + " is below the floor " + r.min + "; update: " + r.update
	case "unknown":
		return "could not read " + r.name + "'s version from `" + r.cmd + "`"
	case "absent":
		return r.name + " is not installed"
	}
	return r.name + " " + ver + " (>= " + r.min + ")"
}
