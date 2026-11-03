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
// that extension by absolute path from hw's argument list, never by discovery.

import (
	"encoding/json"
	"os"
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
	// a lock directory (proper-lockfile's `auth.json.lock`) is the real session's, never a thing to share by link
	return name == "settings.json" || strings.HasPrefix(name, "settings.json.bak") || strings.HasSuffix(name, ".lock")
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

// piShadowSettings is the real settings.json with discovery removed: `packages` and `extensions` go, every other field
// is carried through byte for byte (json.RawMessage), so the private directory differs from the real one by exactly
// what discovery reads. An unreadable or unparsable file leaves "{}" — no packages is the point — and the reason.
func piShadowSettings(raw []byte) ([]byte, string, bool) {
	if len(raw) == 0 {
		return []byte("{}\n"), "", true
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return []byte("{}\n"), err.Error(), false
	}
	delete(fields, "packages")
	delete(fields, "extensions")
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return []byte("{}\n"), err.Error(), false
	}
	return append(out, '\n'), "", true
}

// piShadowPrepare creates (or refreshes) the private agent directory and reports what it did, "" on success; `usable`
// is false when the directory cannot be exported (the reason is then a failure), true when it can, with or without a note. It is
// idempotent: an existing link is left alone unless a rule now skips its name (that LINK is dropped), the settings file is rewritten from the real one every time.
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
	data, why, ok := piShadowSettings(raw)
	// an unparsable real file still exports the "{}" one: no packages is the point
	if err := l.fs.write(dir+"/settings.json", data, 0o644); err != nil {
		return "cannot write " + dir + "/settings.json: " + err.Error(), false
	}
	if !ok {
		return "the real settings.json could not be read as JSON (" + why + "): the private one carries no settings at all", true
	}
	return "", true
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
