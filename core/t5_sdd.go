package main

// Train T5, area C (sdd): the framework mode of a dispatch, from bin/hw `_speckit_skills_in`, `_speckit_on_disk`,
// `_sdd_live_mode`, `_framework_entry`, `_run_dispatch_framework`, `_resolve_effective_sdd_mode`,
// `_validate_sdd_vendor_pair`, `_sdd_model_is_below_orchestrator`, `_validate_sdd_model_pair`,
// `_codex_validation_surface`, `_next_dispatch_model` and `_next_dispatch_effort`.

import (
	"os"
	"regexp"
	"strings"
)

// rtState is what area C keeps for itself (the bash globals only its functions read).
type rtState struct {
	rtInvokerExported bool // _resolve_invoker_context reached its `export` (HW_INVOKER_VENDOR/SESSION/ENDPOINT)
}

// rtSpeckitSkillsDir is bin/hw `_speckit_skills_in`: the skills directory holding a speckit-* entry.
func rtSpeckitSkillsDir(dir string) (string, bool) {
	skills := dir + "/.claude/skills"
	if !isDir(skills) {
		return "", false
	}
	ents, err := os.ReadDir(skills)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "speckit-") {
			continue
		}
		if _, err := os.Stat(skills + "/" + e.Name()); err == nil { // # MUTATION-ANCHOR: 844-R138
			return skills, true
		}
	}
	return "", false
}

// speckitSkillsIn is `_speckit_skills_in <dir> >/dev/null`: success only.
func (d *dsp) speckitSkillsIn(dir string) bool {
	_, ok := rtSpeckitSkillsDir(dir)
	return ok
}

// rtSpeckitOnDiskDir is `_speckit_on_disk`'s output: the project's install, else the machine's.
func (d *dsp) rtSpeckitOnDiskDir(dir string) (string, bool) {
	if w, ok := rtSpeckitSkillsDir(dir); ok {
		return w, true
	}
	return rtSpeckitSkillsDir(d.env("HOME")) // # MUTATION-ANCHOR: 844-R139
}

// speckitOnDisk is `_speckit_on_disk <dir> >/dev/null`: success only.
func (d *dsp) speckitOnDisk(dir string) bool {
	_, ok := d.rtSpeckitOnDiskDir(dir)
	return ok
}

// sddLiveMode is `_sdd_live_mode`: the mode actually in force for a directory, from disk.
func (d *dsp) sddLiveMode(dir string) string {
	if d.speckitSkillsIn(dir) {
		return "speckit"
	}
	if d.speckitSkillsIn(d.env("HOME")) { // # MUTATION-ANCHOR: 844-R142
		return "speckit (global)" // # MUTATION-ANCHOR: 844-R141
	}
	return ""
}

// frameworkEntry is `_framework_entry`: what starts the mode's flow, or nothing.
func (d *dsp) frameworkEntry(mode string) string {
	if mode == "speckit" { // # MUTATION-ANCHOR: 844-R144
		return "/speckit-specify" // # MUTATION-ANCHOR: 844-R143
	}
	return ""
}

// rtAwkSplit is awk's default field splitting: runs of blanks, tabs and newlines.
func rtAwkSplit(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// rtRecords is awk's records of a file: split on newline, a last line without one still counts.
func rtRecords(data string) []string {
	if data == "" {
		return nil
	}
	recs := strings.Split(data, "\n")
	if recs[len(recs)-1] == "" {
		recs = recs[:len(recs)-1]
	}
	return recs
}

// rtReadable is bash's `[ -r f ]` followed by a read: the text, or false when either fails.
func rtReadable(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

var rtFrameworkLineRe = regexp.MustCompile(`^  framework[ \t\n\v\f\r]`)

// runDispatchFramework is `_run_dispatch_framework`: awk '/^  framework[[:space:]]/{ print $2; exit }' dispatch.
func (d *dsp) runDispatchFramework(rundir string) string {
	text, ok := rtReadable(rundir + "/dispatch")
	if !ok {
		return ""
	}
	for _, line := range rtRecords(text) {
		if rtFrameworkLineRe.MatchString(line) {
			f := rtAwkSplit(line)
			if len(f) >= 2 {
				return f[1] // # MUTATION-ANCHOR: 844-R145
			}
			return ""
		}
	}
	return ""
}

var (
	rtMarkRe     = regexp.MustCompile(`  \([^()]*\)$`)
	rtModelLine  = regexp.MustCompile(`^[ \t\n\v\f\r]*model=`)
	rtEffortLine = regexp.MustCompile(`^[ \t\n\v\f\r]*effort=`)
)

// rtDispatchValue is the awk of `_next_dispatch_model`/`_next_dispatch_effort`: the value after the key on the
// first matching line, with a trailing "  (mark)" taken off.
func rtDispatchValue(rundir string, key *regexp.Regexp) string {
	text, ok := rtReadable(rundir + "/dispatch")
	if !ok {
		return ""
	}
	for _, line := range rtRecords(text) {
		if loc := key.FindStringIndex(line); loc != nil {
			line = line[loc[1]:]
			if m := rtMarkRe.FindStringIndex(line); m != nil { // # MUTATION-ANCHOR: 844-136-M02
				line = line[:m[0]]
			}
			return line
		}
	}
	return ""
}

// nextDispatchModel is `_next_dispatch_model`.
func (d *dsp) nextDispatchModel(rundir string) string { return rtDispatchValue(rundir, rtModelLine) }

// nextDispatchEffort is `_next_dispatch_effort`.
func (d *dsp) nextDispatchEffort(rundir string) string { return rtDispatchValue(rundir, rtEffortLine) }

// resolveEffectiveSddMode is `_resolve_effective_sdd_mode`: the typed mode, else what the surface has on disk,
// else the inherited launch record. Published as d.sddEffectiveMode (SDD_EFFECTIVE_MODE).
func (d *dsp) resolveEffectiveSddMode(requested, surface, inherited string) {
	mode := requested
	if mode == "" { // # MUTATION-ANCHOR: 844-R146
		if d.speckitSkillsIn(surface) {
			mode = "speckit" // # MUTATION-ANCHOR: 844-R147
		}
		if mode == "" {
			mode = inherited // # MUTATION-ANCHOR: 844-R148
		}
	}
	d.sddEffectiveMode = mode
}

// validateSddVendorPair is `_validate_sdd_vendor_pair <vendor> <requested> <surface>`.
func (d *dsp) validateSddVendorPair(agent, sdd, surface string) {
	d.rtValidateSddVendorPair(agent, sdd, surface, "")
}

func (d *dsp) rtValidateSddVendorPair(vendor, requested, surface, inherited string) { // # MUTATION-ANCHOR: 844-164-M05
	d.resolveEffectiveSddMode(requested, surface, inherited)
	switch vendor + ":" + d.sddEffectiveMode {
	case "codex:", "codex:none", "opencode:", "opencode:none":
		return
	case "codex:speckit": // # MUTATION-ANCHOR: 844-R149
		d.die("Codex framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + d.sddEffectiveMode + " requires a demonstrated native entry and completed flow, but that capability is unverified. Nothing was built. Use --sdd none for direct delivery, or validate Codex framework execution before re-dispatching.")
	case "opencode:speckit": // # MUTATION-ANCHOR: 844-R150
		d.die("OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + d.sddEffectiveMode + " is Spec Kit, but this machine exposes Spec Kit only as Claude Code skills. Nothing was built. Use --agent claude for Spec Kit, or --sdd none for direct delivery.")
	}
}

// sddModelIsBelowOrchestrator is `_sdd_model_is_below_orchestrator`: the last path segment names a sonnet or haiku.
func (d *dsp) sddModelIsBelowOrchestrator(model string) bool {
	m := model
	if i := strings.LastIndexByte(m, '/'); i >= 0 {
		m = m[i+1:]
	}
	return strings.Contains(m, "sonnet") || strings.Contains(m, "haiku") // # MUTATION-ANCHOR: 844-R151 // # MUTATION-ANCHOR: 844-R152
}

// validateSddModelPair is `_validate_sdd_model_pair <model>`.
func (d *dsp) validateSddModelPair(model string) { d.rtValidateSddModelPair(model, "") }

func (d *dsp) rtValidateSddModelPair(model, where string) {
	if d.sddEffectiveMode != "speckit" { // # MUTATION-ANCHOR: 844-R153
		return
	}
	if model == "" {
		return
	}
	if !d.sddModelIsBelowOrchestrator(model) { // # MUTATION-ANCHOR: 844-R154
		return
	}
	declared := ""
	if d.brief != "" { // # MUTATION-ANCHOR: 844-R155
		declared = rtDeclValue(d.rtBriefDecls(), "sdd")
	}
	exits := "Relaunch with --model opus --effort medium, or dispatch outside the flow with --sdd none."
	if declared == "none" { // # MUTATION-ANCHOR: 844-R156
		exits = "The brief already declared `sdd: none`, and this dispatch did not carry it: relaunch with --sdd none, which is what the brief asked for. Only if the brief is what is wrong, relaunch with --model opus --effort medium instead."
	}
	w := ""
	if where != "" {
		w = " (" + where + ")"
	}
	d.die("SDD orchestrator model REFUSED BEFORE BUILDING ANYTHING: --model " + model + " with --sdd " + d.sddEffectiveMode + w + ". Under " + d.sddEffectiveMode + " this session is the ORCHESTRATOR -- it decides what the change is, partitions it and sequences the phases, and the phase agents pin their own models regardless of --model. A sonnet or haiku orchestrator over opus phase agents is inverted. Nothing was built. " + exits)
}

// codexValidationSurface is `_codex_validation_surface`.
func (d *dsp) codexValidationSurface() string {
	if d.c.t.get(d.proj, "product_repo") == "1" {
		return d.c.checkout(d.proj)
	}
	return d.c.work + "/" + d.proj + "/" + d.task
}

// rtBriefDecls is `_load_brief_decls` followed by a read of BRIEF_DECLS (area A owns both).
func (d *dsp) rtBriefDecls() string { return d.loadBriefDecls() }

// rtSetBriefDecls sets what rtBriefDecls reads (the tests' door to area A's BRIEF_DECLS).
func (d *dsp) rtSetBriefDecls(s string) { d.brDecls, d.brDeclsRead = s, true }

// rtDeclValue is awk -F'\t' '$1 == "<key>" { print $2; exit }' over "key<TAB>value" lines, as `$(…)` returns it.
func rtDeclValue(decls, key string) string {
	for _, line := range strings.Split(decls, "\n") {
		f := strings.Split(line, "\t")
		if f[0] == key {
			if len(f) > 1 {
				return strings.TrimRight(f[1], "\n")
			}
			return ""
		}
	}
	return ""
}
