package main

// Train T5, area B: the brief's `plugins:`, installed code-quality plugins loaded into ONE executor.
// The pinned copies are an optional install step (setup/vendor/plugins/install, install.sh --with-review-plugins)
// into <brain>/plugins; hw reads only that directory.
// bin/hw: _plugins_dir, _plugins_installed, _plugin_is_installed, _plugins_declared, _plugins_declared_installed,
// _plugins_names_line, _check_plugins, _plugins_args, _plugins_lines, _plugins_receipt_value/_source.
//
// pi is not here: a pi dispatch is not ported (t5.go refuses it, and bin/hw sends it to bash), so the
// `-e <plugin dir>` half lives in bash alone.

import (
	"os"
	"sort"
	"strings"
)

// pluginsDir is _plugins_dir.
func (d *dsp) pluginsDir() string { return d.vdRoot() + "/plugins" }

// pluginsInstalled is _plugins_installed: the directories that hold a .claude-plugin/plugin.json, sorted as
// `LC_ALL=C sort` does. The `*/` glob it walks skips what starts with a dot.
func (d *dsp) pluginsInstalled() []string {
	var out []string
	ents, err := os.ReadDir(d.pluginsDir())
	if err != nil {
		return nil
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if isFile(d.pluginsDir() + "/" + n + "/.claude-plugin/plugin.json") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// pluginIsInstalled is _plugin_is_installed.
func (d *dsp) pluginIsInstalled(n string) bool { return has(d.pluginsInstalled(), n) }

// pluginsDeclared is _plugins_declared: the names the brief's first `plugins:` line holds, in its order, once each.
// Brackets and quotes are dropped, then the value splits on spaces, tabs and commas.
func (d *dsp) pluginsDeclared() []string {
	v := strings.Map(func(r rune) rune {
		switch r {
		case '[', ']', '"', '\'':
			return -1
		}
		return r
	}, d.briefDecl("plugins"))
	var out []string
	seen := map[string]bool{}
	for _, n := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' }) {
		if !seen[n] { // # MUTATION-ANCHOR: 844-P02
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// pluginsDeclaredInstalled is _plugins_declared_installed.
func (d *dsp) pluginsDeclaredInstalled() []string {
	var out []string
	for _, n := range d.pluginsDeclared() {
		if d.pluginIsInstalled(n) {
			out = append(out, n)
		}
	}
	return out
}

// checkPlugins is _check_plugins: the refusal, before anything is built, naming the command that installs them.
func (d *dsp) checkPlugins() {
	declared := d.pluginsDeclared()
	if len(declared) == 0 {
		return
	}
	var bad []string
	for _, n := range declared {
		if !d.pluginIsInstalled(n) {
			bad = append(bad, n)
		}
	}
	if len(bad) > 0 { // # MUTATION-ANCHOR: 844-P03
		have := strings.Join(d.pluginsInstalled(), ", ")
		if have == "" {
			have = "none"
		}
		root := d.vdRoot()
		d.die("the brief declares plugins: " + strings.Join(bad, ", ") + ", which are not installed here (installed: " + have + "). The pinned review plugins (thermo, ponytail, pstack) are an optional install step, off by default: run 'bash " + root + "/setup/vendor/plugins/install' in a checkout, or 'install.sh --brain " + root + " --with-review-plugins' for an installed brain. NOTHING was built — no worktree, no database, no pane.")
	}
	if vdSpace(d.pluginsDir()) {
		d.die("the brief declares plugins:, and they cannot be passed as one word: the checkout path has whitespace (" + d.vdRoot() + "). NOTHING was built.")
	}
	d.brAppendDeclared("plugins")
}

// pluginsArgs is _plugins_args: claude gets one --plugin-dir per plugin, after the guard's.
func (d *dsp) pluginsArgs() {
	if d.agent != "claude" { // # MUTATION-ANCHOR: 847-P-02
		return
	}
	for _, n := range d.pluginsDeclaredInstalled() {
		d.agentArgs = vdJoinArg(d.agentArgs, "--plugin-dir "+d.pluginsDir()+"/"+n) // # MUTATION-ANCHOR: 847-P-01
	}
}

// pluginsLines is _plugins_lines: the manifest's field, leading newline included, nothing when none is declared.
func (d *dsp) pluginsLines() string {
	declared := d.pluginsDeclared()
	if len(declared) == 0 {
		return ""
	}
	names := strings.Join(declared, ", ")
	if d.dispatchRoute == "reuse" { // # MUTATION-ANCHOR: 844-P06
		return "\n  plugins     " + names + "  (brief plugins:) — NOT APPLIED: this dispatch re-tasks the live executor in " + orDefaultStr(d.reusePane, "the reused pane") + ", and a plugin is loaded when a session starts. Add --fresh to launch one that loads them."
	}
	if d.agent == "claude" {
		var flags []string
		for _, n := range declared {
			flags = append(flags, "--plugin-dir "+d.pluginsDir()+"/"+n)
		}
		return "\n  plugins     " + names + "  (brief plugins:; installed pinned copies) — " + strings.Join(flags, " ") + ": this session only; the plugins' hooks run natively in it, through hw/hook-env, and what a hook writes goes to $HW_ARTIFACTS/plugin-state, never ~/.claude"
	}
	return "\n  plugins     " + names + "  (brief plugins:) — NOT APPLIED: this executor is " + d.agent + ", and only claude (--plugin-dir) and pi (-e) load plugins. Its skills are whatever " + d.agent + " finds on its own."
}

// pluginsReceiptValue is _plugins_receipt_value.
func (l *lch) pluginsReceiptValue() string {
	asked := strings.Join(l.pluginsDeclared(), ",")
	if l.agent == "claude" || l.agent == "pi" { // # MUTATION-ANCHOR: 847-P-03
		return asked
	}
	return "NOT APPLIED — " + orDefaultStr(l.agent, "unknown") + " does not load plugins (asked: " + asked + ")"
}

// pluginsReceiptSource is _plugins_receipt_source.
func (l *lch) pluginsReceiptSource() string {
	return "brief frontmatter plugins: at dispatch; installed pinned copies in " + l.pluginsDir() + " (claude: --plugin-dir, hooks through hw/hook-env; pi: -e)"
}
