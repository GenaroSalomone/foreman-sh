package main

// `--agent pi`, the private agent directory a session with plugins gets (setup/briefs/pi-ask-duplicada-070.md).
//
// THE DEFECT THIS CLOSES. pi's `-ne` turns extension DISCOVERY off for the session hw types into the pane, so it loads
// exactly the extension set hw names. A plugin is not the only process pi starts, though: pstack's own `agent` tool
// spawns its children with `--extension <plugin>/pi/index.ts` and NO `-ne`
// (setup/vendor/plugins/pstack/pi/agents.ts:238, its argv built by `piInvocation()` in pi/config.ts:33-40), so the child
// runs with discovery ON and finds the ambient packages of ~/.pi/agent/settings.json. The rpiv-ask-user-question
// package (settings.json's `packages`) registers `ask_user_question`, pstack's vendored extension registers it too
// (setup/vendor/plugins/pstack/pi/ask.ts:51), and pi refuses the second registration: the child dies at startup with
// `Tool "ask_user_question" conflicts with …`, which is what a pi executor sees when it runs a pstack review agent.
//
// THE FIX, and why it is here. The one lever a launch has over discovery in a child pi process is the environment it
// inherits: `PI_CODING_AGENT_DIR` moves pi's agent directory, and therefore the settings.json that lists the packages.
// So a pi launch whose brief declares plugins gets a PRIVATE agent directory under the run's artifacts: symlinks to the
// real one (auth, models, trust, npm, bin, sessions — the identity the run must keep) and a settings.json copied from
// it with `packages` and `extensions` removed. Discovery in a child then finds nothing ambient, while the top-level
// session is unaffected (`-ne` already loaded nothing ambient) and the user's own pi sessions keep their file untouched.
// A plugin is the only thing that starts such a child, so the shadow is only taken when one is declared.
//
// `extensions` (the directory) is deliberately NOT linked: pi discovers .ts files there too, and herdr's pane-state
// extension reporting from inside a subagent would report the agent's state as the pane's. The top-level session gets
// that extension by absolute path from hw's argument list, never by discovery. The private directory's OWN
// `extensions/` is written here, with one file: the write guard a plugin's child must find (piChildGuardShim).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// piRealAgentDirEnv carries pi's REAL agent directory next to the private PI_CODING_AGENT_DIR, so a dispatch made from
// inside a shadowed executor resolves the real one instead of taking the parent's private directory for it.
const piRealAgentDirEnv = "HW_PI_REAL_AGENT_DIR"

// piShadowSkip are the entries of the real agent directory the private one does not carry: the settings file it
// replaces, and the two discovery directories whose extensions a child must not load (see the header).
func piShadowSkip(name string) bool {
	switch name {
	case "extensions", "extensions-disabled":
		return true
	}
	// `themes` is not linked as a directory either: a link would put the private directory's writes into the real one,
	// and pi reads a theme only from `<agent-dir>/themes/<name>.json`. piShadowPrepare links the ONE file the settings
	// names (piShadowThemeFile), read-only, from the real directory or from a package.
	return name == "settings.json" || strings.HasPrefix(name, "settings.json.bak") ||
		name == "themes" || strings.HasSuffix(name, ".lock")
}

// piShadowThemeNames is what the real settings' `theme` selects that pi does not ship: pi's own dark, light and system
// are always there (docs/themes.md), and an automatic setting is a pair, "light/dark", whose two names both matter.
// Empty for no setting, an unparsable file, or a theme pi brings itself.
func piShadowThemeNames(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	var setting string
	if err := json.Unmarshal(fields["theme"], &setting); err != nil {
		return nil
	}
	var out []string
	for _, name := range strings.Split(setting, "/") { // a name cannot hold a slash: it is the light/dark separator
		name = strings.TrimSpace(name)
		switch name {
		case "", "dark", "light", "system":
			continue
		}
		if !has(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// piShadowThemeFile is the JSON file a theme name is read from, "" when nothing on disk holds it. pi looks in
// `<agent-dir>/themes/<name>.json` (docs/themes.md); a package brings its themes through its `pi.themes` manifest or a
// conventional `themes/` directory (docs/packages.md), installed under `<agent-dir>/npm/node_modules/` for an npm:
// source. `extensions/` is never read: this is a theme file, not a thing a child can discover.
func piShadowThemeFile(real string, raw []byte, name string) string {
	file := name + ".json"
	if isFile(real + "/themes/" + file) {
		return real + "/themes/" + file
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	// an explicit `themes` setting names files or directories; one of them may hold the theme.
	var themes []string
	if err := json.Unmarshal(fields["themes"], &themes); err == nil {
		for _, t := range themes {
			p := expandHome(t, os.Getenv("HOME"))
			if !filepath.IsAbs(p) {
				p = real + "/" + p
			}
			if isFile(p+"/"+file) {
				return p + "/" + file
			}
			if isFile(p) && filepath.Base(p) == file {
				return p
			}
		}
	}
	var packages []string
	if err := json.Unmarshal(fields["packages"], &packages); err != nil {
		return ""
	}
	for _, entry := range packages {
		dir := piPackageDir(real, entry)
		if dir == "" {
			continue
		}
		if found := piThemeInPackage(dir, file); found != "" {
			return found
		}
	}
	return ""
}

// piPackageDir is where pi keeps a package a settings `packages` entry names: an npm: source lands in the agent
// directory's managed npm root under the package's name (package-manager.js getManagedNpmInstallPath), an absolute or
// ~ path is itself. "" for a source this does not resolve: the theme then falls back rather than guessing a path.
func piPackageDir(real, entry string) string {
	spec := strings.TrimSpace(entry)
	if name, ok := strings.CutPrefix(spec, "npm:"); ok {
		name = strings.TrimSpace(name)
		// npm:pkg@1, npm:@scope/pkg@1: the version starts at the last @, a scoped name at its second @.
		if at := strings.LastIndexByte(name, '@'); at > 0 {
			name = name[:at]
		}
		if name == "" {
			return ""
		}
		dir := real + "/npm/node_modules/" + name
		if isDir(dir) {
			return dir
		}
		return ""
	}
	if strings.HasPrefix(spec, "git:") || strings.HasPrefix(spec, "http:") || strings.HasPrefix(spec, "https:") || strings.HasPrefix(spec, "ssh:") {
		return ""
	}
	p := expandHome(spec, os.Getenv("HOME"))
	if !filepath.IsAbs(p) {
		p = real + "/" + p
	}
	if isDir(p) {
		return p
	}
	return ""
}

// piThemeInPackage is <name>.json inside a package's declared theme places: the directories and files its
// `pi.themes` manifest names (globs are read as their directory), then the conventional `themes/` pi discovers when the
// manifest says nothing. "" when the package does not hold it.
func piThemeInPackage(dir, file string) string {
	var manifest struct {
		Pi struct {
			Themes []string `json:"themes"`
		} `json:"pi"`
	}
	place := func(p string) string {
		p = strings.TrimPrefix(p, "./")
		if p == "" {
			return ""
		}
		full := dir + "/" + p
		if i := strings.IndexAny(filepath.Base(full), "*?["); i >= 0 {
			full = filepath.Dir(full)
		}
		if isFile(full) && filepath.Base(full) == file {
			return full
		}
		if isFile(full + "/" + file) {
			return full + "/" + file
		}
		return ""
	}
	if data, err := os.ReadFile(dir + "/package.json"); err == nil {
		if json.Unmarshal(data, &manifest) == nil {
			for _, t := range manifest.Pi.Themes {
				if found := place(t); found != "" {
					return found
				}
			}
		}
	}
	if isFile(dir + "/themes/" + file) {
		return dir + "/themes/" + file
	}
	return ""
}

// piShadowDir is where a pi launch's private agent directory goes: "" when this launch does not need one (another
// vendor, or no plugin declared). `cwd` is the work directory the artifacts directory is resolved for.
func (l *lch) piShadowDir(cwd string) string {
	if l.agent != "pi" || len(l.pluginsDeclaredInstalled()) == 0 {
		return ""
	}
	// THIS run's directory, never the HW_ARTIFACTS of the environment: a dispatch from inside another hw run inherits the
	// parent's, and exportHwEnv sets this run's only after the pi block.
	art := l.artifactsDirFor(l.proj, cwd)
	if art == "" {
		return ""
	}
	return art + "/pi-agent"
}

// piShadowSettings is the real settings.json with discovery removed and the startup listing off: `packages` and
// `extensions` go, `quietStartup` is set, every other field is carried through byte for byte (json.RawMessage), so the
// private directory differs from the real one by exactly what discovery reads. An unreadable or unparsable file leaves
// no packages — the point — and the reason.
//
// THE LISTING, and why it is this setting. A launch with plugins loads every skill of every declared plugin (~68 of
// thermo's, ponytail's and pstack's), and pi prints each of them plus the extensions under a `[Skills]`/`[Extensions]`
// block unless `quietStartup` is true (interactive-mode.js `shouldShowStartupDetails`); the same launch without plugins
// prints almost nothing, so the pane opened on an inventory instead of on the task. It is a settings field, not a flag:
// pi has no `--quiet`. pi KEEPS ITS DIAGNOSTICS quiet (`showDiagnosticsWhenQuiet`, interactive-mode.js:1633), so a real
// malformed skill or a conflicting command is still named on the first screen.
func piShadowSettings(raw []byte) ([]byte, string, bool) {
	fields := map[string]json.RawMessage{}
	why, ok := "", true
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			fields, why, ok = map[string]json.RawMessage{}, err.Error(), false
		}
	}
	delete(fields, "packages")
	delete(fields, "extensions")
	fields["quietStartup"] = json.RawMessage("true")
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return []byte("{}\n"), err.Error(), false
	}
	return append(out, '\n'), why, ok
}

// piShadowPrepare creates (or refreshes) the private agent directory and reports what it did, "" on success; `usable`
// is false when the directory cannot be exported (the reason is then a failure), true when it can, with or without a note. It is
// idempotent: an existing link is left alone unless a rule now skips its name (that LINK is dropped), a theme link
// is refreshed when its source moved (and dropped when the theme it named is gone), and the settings file is
// rewritten from the real one every time.
//
// KNOWN LIMIT (KNOWN-LIMITATIONS, pi section): `auth.json` is linked, so a refresh writes the one real file (pi writes it
// in place with writeFileSync, core/auth-storage.js:71 and :147, which follows the link), but pi's lock is taken on the
// link's own path with `realpath: false` (auth-storage.js:50 and :91-92), `<private>/auth.json.lock`, and no pi
// setting or variable moves auth.json apart from the agent directory (config.js getAgentDir/getAuthPath). The lock
// does not exclude another pi session on the real directory.
func (l *lch) piShadowPrepare(dir string) (string, bool) {
	real := l.piAgentDir()
	if dir == "" || real == "" || dir == real {
		return "no agent directory to shadow", false
	}
	if err := l.fs.mkdirAll(dir); err != nil {
		return "cannot create " + dir + ": " + err.Error(), false
	}
	ents, err := os.ReadDir(real)
	if err != nil {
		return "cannot read " + real + ": " + err.Error(), false
	}
	// a reused directory may hold a link a later rule skips (a `*.lock` from before): drop skipped LINKS only
	if old, err := os.ReadDir(dir); err == nil {
		for _, e := range old {
			if piShadowSkip(e.Name()) && e.Type()&os.ModeSymlink != 0 {
				l.fs.remove(dir + "/" + e.Name())
			}
		}
	}
	for _, e := range ents {
		name := e.Name()
		if piShadowSkip(name) {
			continue
		}
		link := dir + "/" + name
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := l.fs.symlink(real+"/"+name, link); err != nil {
			return "cannot link " + link + ": " + err.Error(), false
		}
	}
	raw, rerr := os.ReadFile(real + "/settings.json")
	if rerr != nil && !os.IsNotExist(rerr) {
		return "cannot read " + real + "/settings.json: " + rerr.Error(), false
	}
	// THE THEME THE SETTINGS NAME. The private settings carry `theme` through, so without its file pi prints
	// `Failed to load theme "<name>"` and falls back to the system theme (measured 2026-11-04). pi reads a custom theme
	// from `<agent-dir>/themes/<name>.json` (docs/themes.md); the file is linked read-only, from the real themes/
	// directory or a package's pi.themes, and never from extensions/.
	var missing []string
	for _, name := range piShadowThemeNames(raw) {
		src := piShadowThemeFile(real, raw, name)
		link := dir + "/themes/" + name + ".json"
		if src == "" {
			missing = append(missing, name)
			// a link from an earlier run whose file is GONE is dangling: pi would read it as the theme. A link that
			// still resolves (the package stopped being declared but is still installed) is left alone.
			if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				if _, serr := os.Stat(link); serr != nil {
					l.fs.remove(link)
				}
			}
			continue
		}
		if err := l.fs.mkdirAll(dir+"/themes"); err != nil {
			return "cannot create " + dir + "/themes: " + err.Error(), false
		}
		// a reused directory may hold a link to where the theme USED to be (the real themes/ before it moved into a
		// package, a package reinstalled elsewhere): the resolved source wins, or pi keeps failing to load the theme
		if to, err := os.Readlink(link); err == nil {
			if to == src {
				continue
			}
			l.fs.remove(link)
		} else if _, lerr := os.Lstat(link); lerr == nil {
			continue // a theme file someone put there is not our link: left in place, the directory still works
		}
		if err := l.fs.symlink(src, link); err != nil {
			return "cannot link " + link + ": " + err.Error(), false
		}
	}
	data, why, ok := piShadowSettings(raw)
	// an unparsable real file still exports the "{}" one: no packages is the point
	if err := l.fs.write(dir+"/settings.json", data, 0o644); err != nil {
		return "cannot write " + dir + "/settings.json: " + err.Error(), false
	}
	l.piChildGuardShim(dir)
	if !ok {
		return "the real settings.json could not be read as JSON (" + why + "): the private one carries no settings at all", true
	}
	if len(missing) > 0 {
		return "the theme " + strings.Join(missing, ", ") + " is named by the real settings but no file holds it (the real themes/ directory or a package's pi.themes): pi falls back to the system theme", true
	}
	return "", true
}

// piChildGuardShim writes the one extension a plugin's child discovers under the private agent directory: a one-line
// re-export, by the relative path from that directory, of setup/pi/hw-child-guard.ts. Discovery in the child is ON
// (pstack's `agent` passes no `-ne`), so this is how the write guard reaches a child that pi-subagents' required-child
// API cannot: measured, a pstack child ran `echo x > <product repo>/x` unguarded with this file absent and was
// refused with it present (setup/tests/983). Rewritten every launch, so a moved install is not a stale path.
//
// It FAILS CLOSED, like the adapter itself (t6_pi.go dies when hw-executor.ts is not on disk): the guard lives under
// HW_CORE_ROOT, the same root as the adapter it mirrors — never the brain tree, which a harness layout keeps apart
// (HW_BRAIN_ROOT names another checkout) — and a guard that could not be written refuses the plugin dispatch instead
// of shipping children that discover nothing.
func (l *lch) piChildGuardShim(dir string) {
	guard := l.vdRoot() + "/setup/pi/hw-child-guard.ts"
	if !isFile(guard) {
		l.die(l.piShadowRefusal("the guard a plugin's child discovers is not on disk at " + guard + " — a plugin dispatch is not launched without it (its pstack children would run unguarded)."))
	}
	spec, err := filepath.Rel(dir+"/extensions", guard)
	if err != nil || !strings.HasPrefix(spec, ".") {
		spec = guard
	}
	// one line: a file found under $HW_ARTIFACTS cannot show that hw wrote it, and where the guard lives
	body := "// Written by hw at launch (core/t6_pi_shadow.go piChildGuardShim).\nexport { default } from " + strconv.Quote(spec) + ";\n"
	if err := l.fs.mkdirAll(dir + "/extensions"); err != nil {
		l.die(l.piShadowRefusal("cannot create " + dir + "/extensions, so a plugin's children would discover no write guard: " + err.Error() + "."))
	}
	// THE DIRECTORY IS hw's. Discovery in a child runs every .ts it finds here, and $HW_ARTIFACTS is a path the
	// executor may write — so anything but the guard is removed before the shim lands, never left to run (Judgment
	// Day, judge B, 2026-11-04: a foreign extension would be discovered by every pstack child).
	if ents, err := os.ReadDir(dir + "/extensions"); err == nil {
		for _, e := range ents {
			// a REGULAR file of our name is ours; a symlink with our name is not (fs.write follows it, so keeping it
			// would write the guard body through to whatever the link names), and nothing else belongs here at all
			p := dir + "/extensions/" + e.Name()
			if e.Name() == "hw-child-guard.ts" && e.Type().IsRegular() {
				continue
			}
			l.fs.remove(p)
			if !l.fs.readonly { // a recording run removes nothing, so it must not verify against the disk
				if _, err := os.Lstat(p); err == nil {
					l.die(l.piShadowRefusal("cannot remove " + p + ", which a plugin's child would discover as an extension."))
				}
			}
			l.warn("removed " + p + ": a plugin's child discovers every extension in that directory, and only hw's guard belongs there")
		}
	}
	if err := l.fs.write(dir+"/extensions/hw-child-guard.ts", []byte(body), 0o644); err != nil {
		l.die(l.piShadowRefusal("cannot write the child guard " + dir + "/extensions/hw-child-guard.ts, so a plugin's children would run unguarded: " + err.Error() + "."))
	}
}

// piShadowRefusal is a LATE refusal's words: exportHwEnv runs after the worktree and its receipts exist, so it says
// what survives and how to clean it up, in the shape agent_start uses — never "nothing was built".
func (l *lch) piShadowRefusal(what string) string {
	return what + " The worktree this launch already made is intact: read `hw receipt " + l.proj + " " + l.task +
		"`, then close it with `hw done " + l.proj + " " + l.task + "` and dispatch again."
}

// piShadowExport prepares this launch's private agent directory once and returns it: "" when the launch has none, or
// when it could not be made, and then the second value says why (the caller names the directory pi gets instead).
func (l *lch) piShadowExport(cwd string) (dir, failure string) {
	dir = l.piShadowDir(cwd)
	if dir == "" {
		return "", ""
	}
	why, usable := l.piShadowPrepare(dir)
	if !usable {
		return "", why
	}
	if why != "" {
		l.warn("pi agent dir: " + why)
	}
	return dir, ""
}

// piRunDirs is the one answer both emitters (piExportEnv and piHwEnvFlags) use: `run` is the PI_CODING_AGENT_DIR this
// launch's pi must get ("" = leave the environment's as it is) and `real` is pi's real agent directory. A launch with
// plugins gets its private directory; one whose inherited directory is a shadowed parent's gets the real one, never the
// parent's (its `hw done` deletes it under this child); a failed private directory falls back to that same rule.
func (l *lch) piRunDirs(cwd string) (string, string) {
	if !l.piRunDone {
		l.piRun, l.piReal = l.piRunDirsOnce(cwd)
		l.piRunDone = true
	}
	return l.piRun, l.piReal
}

func (l *lch) piRunDirsOnce(cwd string) (run, real string) {
	real = l.piAgentDir()
	dir, failure := l.piShadowExport(cwd)
	if dir != "" {
		return dir, real
	}
	if l.piIsShadowDir(l.env("PI_CODING_AGENT_DIR")) {
		run = real
	}
	if failure != "" {
		gets := run
		if gets == "" {
			gets = orDefaultStr(l.env("PI_CODING_AGENT_DIR"), real)
		}
		l.warn("pi agent dir: " + failure + " — pi gets " + gets + ", and a plugin's children will discover its packages (a pstack agent then dies on ask_user_question)")
	}
	return run, real
}
