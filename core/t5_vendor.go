package main

// Train T5, area B: the vendor, the model, the account and the guards of `hw <lane> <task> --dry-run`.
// bin/hw: _apply_model_default, _check_model_floor, _model_pin_for, _model_suggested, _model_suggestion_note,
// _model_why_short; bin/project-spaces.sh: model_is_build_only, model_tier, project_model_floor(_accepts),
// project_artifacts, project_default_model.

import (
	"os"
	"strings"
)

// vdState is what area B keeps for itself (anything another area reads is a field of dsp).
type vdState struct {
	vdTable      *jval // projects.json as jq reads it, loaded on first use
	vdTableTried bool
	vdEngramOver string // OPENCODE_ENGRAM_OVERLAY
}

// vdLaneObj is the lane's object in the lane table file, nil when it cannot be read.
func (d *dsp) vdLaneObj(proj string) *jval {
	if !d.vdTableTried {
		d.vdTableTried = true
		if data, err := os.ReadFile(d.c.t.file); err == nil {
			if vals, perr := parseStream(data); perr == nil && len(vals) > 0 {
				d.vdTable = vals[0]
			}
		}
	}
	lanes := field(d.vdTable, "lanes")
	if lanes == nil || lanes.kind != vObj {
		return nil
	}
	for _, m := range lanes.obj {
		if m.k == proj {
			return m.v
		}
	}
	return nil
}

// vdLane is lane_get for the fields the Go lane table does not load (lane_config_load's emit lines).
func (d *dsp) vdLane(proj, name string) string {
	if !d.c.t.known(proj) {
		return ""
	}
	switch name {
	case "vendor", "model", "account", "engram":
		return d.c.t.get(proj, name)
	}
	v := d.vdLaneObj(proj)
	switch name {
	case "model_floor":
		return orDefault(field(field(v, "model_floor"), "tier"), "") // # MUTATION-ANCHOR: 844-B01
	case "model_floor_accepts":
		return joinStrs(field(field(v, "model_floor"), "accepts"), " ") // # MUTATION-ANCHOR: 844-B02
	case "model_pins":
		pins := field(v, "model_pins")
		if pins == nil || pins.kind != vObj {
			return ""
		}
		parts := make([]string, len(pins.obj))
		for i, m := range pins.obj {
			parts[i] = m.k + "=" + m.v.tostring() // # MUTATION-ANCHOR: 844-B03
		}
		return strings.Join(parts, " ")
	case "brain_guard":
		if bg := field(v, "brain_guard"); bg != nil && bg.kind == vBool && !bg.b { // # MUTATION-ANCHOR: 844-B04
			return "0"
		}
		return "1"
	case "opencode_config_dir":
		return expandHome(orDefault(field(v, "opencode_config_dir"), ""), d.c.home) // # MUTATION-ANCHOR: 844-B05
	case "artifacts":
		return orDefault(field(v, "artifacts"), orDefault(field(v, "account"), "default")) // # MUTATION-ANCHOR: 844-B06
	}
	return ""
}

// vdIFSFields is `for x in $var`: split on space, tab and newline.
func vdIFSFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// modelIsBuildOnly is model_is_build_only.
func modelIsBuildOnly(model string) bool {
	return model == "openai/gpt-5.6-sol" || strings.HasPrefix(model, "openai/gpt-5.6-sol-")
}

// modelTier is model_tier: the claude ladder on the last path segment, "" for what it cannot rank.
func modelTier(model string) string {
	seg := model[strings.LastIndex(model, "/")+1:]
	switch {
	case strings.Contains(seg, "haiku"): // # MUTATION-ANCHOR: 844-B07
		return "1"
	case strings.Contains(seg, "sonnet"): // # MUTATION-ANCHOR: 844-B08
		return "2"
	case strings.Contains(seg, "opus"): // # MUTATION-ANCHOR: 844-B09
		return "3"
	}
	return ""
}

func (d *dsp) modelIsBuildOnly(model string) bool { return modelIsBuildOnly(model) }
func (d *dsp) modelTier(model string) string      { return modelTier(model) }

// projectModelFloor is project_model_floor.
func (d *dsp) projectModelFloor(proj string) string { return d.vdLane(proj, "model_floor") }

// projectArtifacts is project_artifacts.
func (d *dsp) projectArtifacts(proj string) string {
	return orDefaultStr(d.vdLane(proj, "artifacts"), "default")
}

// modelPinFor is _model_pin_for for this dispatch's lane: the id the lane pins for an alias, or "".
func (d *dsp) modelPinFor(model string) string {
	switch model {
	case "fable", "opus", "sonnet", "haiku": // # MUTATION-ANCHOR: 844-B10
	default:
		return ""
	}
	for _, pair := range vdIFSFields(d.vdLane(d.proj, "model_pins")) {
		if k, v, _ := strings.Cut(pair, "="); k == model { // # MUTATION-ANCHOR: 844-B11
			return v
		}
	}
	return ""
}

// modelSuggested is _model_suggested.
func (d *dsp) modelSuggested() string {
	floor := d.projectModelFloor(d.proj)
	if d.modelWhy != "" || floor == "opus" { // # MUTATION-ANCHOR: 844-B12
		return "opus"
	}
	return "sonnet"
}

// vdUTF8 is whether `cut -c` counts characters: the locale the dispatch runs in (LC_ALL, else LC_CTYPE, else LANG) names UTF-8.
func (d *dsp) vdUTF8() bool {
	loc := d.env("LC_ALL")
	if loc == "" {
		loc = d.env("LC_CTYPE")
	}
	if loc == "" {
		loc = d.env("LANG")
	}
	loc = strings.ToLower(loc)
	return strings.Contains(loc, "utf-8") || strings.Contains(loc, "utf8")
}

// modelWhyShort is _model_why_short: tr '\n' ' ' | cut -c1-72 (bytes outside a UTF-8 locale).
func (d *dsp) modelWhyShort() string {
	s := strings.ReplaceAll(d.modelWhy, "\n", " ")
	if d.vdUTF8() {
		if r := []rune(s); len(r) > 72 { // # MUTATION-ANCHOR: 844-B13
			s = string(r[:72])
		}
	} else if len(s) > 72 {
		s = s[:72]
	}
	return s
}

// modelSuggestionNote is _model_suggestion_note: the manifest's annotation of the model field.
func (d *dsp) modelSuggestionNote() string {
	suggested := d.modelSuggested()
	if d.model == "" {
		return ""
	}
	if strings.HasPrefix(d.modelSrc, "default;") { // # MUTATION-ANCHOR: 844-B14
		return ""
	}
	chosen := d.modelTier(d.model)
	if chosen == "" {
		return ""
	}
	suggestedTier := d.modelTier(suggested)
	if chosen <= suggestedTier { // both are one digit // # MUTATION-ANCHOR: 844-B15
		if d.modelWhy == "" { // # MUTATION-ANCHOR: 844-B16
			return ""
		}
		return " / " + suggested + " suggested — open decision (" + d.modelWhySrc + "): " + d.modelWhyShort()
	}
	kindPart := ""
	switch d.taskKindSrc {
	case "declared":
		kindPart = "kind: " + d.taskKind + ", "
	case "inferred":
		kindPart = "kind: " + d.taskKind + " (inferred from the task name), "
	}
	return " / " + suggested + " suggested — " + kindPart + "no open decision declared"
}

// vdBriefKind is _brief_kind_for_close: the kind the check recorded, else the brief's own `kind:` line.
func (d *dsp) vdBriefKind() string {
	if d.taskKind != "" {
		return d.taskKind
	}
	if !isFile(d.brief) {
		return ""
	}
	data, err := os.ReadFile(d.brief)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" { // the file's last newline ends a record, it does not start one
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		if i == 0 {
			if l != "---" { // # MUTATION-ANCHOR: 844-B17
				return ""
			}
			continue
		}
		if l == "---" {
			return ""
		}
		low := strings.ToLower(l) // # MUTATION-ANCHOR: 844-B18
		t := strings.TrimLeft(low, " \t\n\v\f\r")
		if !strings.HasPrefix(t, "kind") {
			continue
		}
		t = strings.TrimLeft(t[len("kind"):], " \t\n\v\f\r")
		if !strings.HasPrefix(t, ":") {
			continue
		}
		// sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); gsub(/["'[:space:]]/, "")
		v := low[strings.Index(low, ":")+1:]
		v = strings.TrimLeft(v, " \t\n\v\f\r")
		if h := strings.Index(v, "#"); h >= 0 {
			v = strings.TrimRight(v[:h], " \t\n\v\f\r")
		}
		return strings.Map(func(r rune) rune {
			switch r {
			case '"', '\'', ' ', '\t', '\n', '\v', '\f', '\r':
				return -1
			}
			return r
		}, v)
	}
	return ""
}

// applyModelDefault is _apply_model_default: sonnet for a claude executor with no model, opus for kind design.
func (d *dsp) applyModelDefault() {
	if !(d.model == "" && d.agent == "claude" && d.reusePane == "") { // # MUTATION-ANCHOR: 844-B19
		return
	}
	if d.projectModelFloor(d.proj) != "" { // # MUTATION-ANCHOR: 844-B20
		return
	}
	if d.vdLane(d.proj, "model") != "" { // # MUTATION-ANCHOR: 844-B21
		return
	}
	if d.sddEffectiveMode == "speckit" {
		return
	}
	kind := d.vdBriefKind()
	switch kind {
	case "design":
		d.model, d.modelSrc = "opus", "default; kind design" // # MUTATION-ANCHOR: 844-B22
	default:
		d.model, d.modelSrc = "sonnet", "default; kind "+orDefaultStr(kind, "none") // # MUTATION-ANCHOR: 844-B23
	}
}

// checkModelFloor is _check_model_floor.
func (d *dsp) checkModelFloor() {
	floor := d.projectModelFloor(d.proj)
	if floor == "" {
		return
	}
	accepts := d.vdLane(d.proj, "model_floor_accepts")
	floorTier := d.modelTier(floor)
	if d.model == "" {
		if d.agent == "claude" && d.reusePane == "" { // # MUTATION-ANCHOR: 844-B24
			d.model, d.modelSrc = floor, "floor of "+d.proj
			d.modelFloorLine = floor + " — met (pinned: no model was named, and claude's inherited default is unmeasured)"
		} else {
			d.modelFloorLine = floor + " — UNESTABLISHED: " + d.agent + "'s inherited default is not a model hw can rank"
			d.warn("model floor: " + d.proj + " requires at least " + floor + ", and this " + d.agent + " dispatch names no model, so what it runs was not checked") // # MUTATION-ANCHOR: 844-B25
		}
		return
	}
	tier := d.modelTier(d.model)
	if tier != "" { // # MUTATION-ANCHOR: 844-B26
		if tier >= floorTier { // # MUTATION-ANCHOR: 844-B27
			d.modelFloorLine = floor + " — met"
			return
		}
		if d.belowFloorWhy == "" { // # MUTATION-ANCHOR: 844-B28
			d.die(d.model + " is below " + d.proj + "'s model floor (" + floor + "), declared in projects.json. NOTHING was built. The floor is the lane's own decision (setup/decisions.md). Launch with --model " + floor + ", or, if this dispatch really must run lower, say why with --below-floor-why \"<reason>\" — it is written to the manifest and the receipt.")
		}
		if strings.Contains(d.belowFloorWhy, "\n") {
			d.die("--below-floor-why must be one line so the manifest and the receipt stay parseable")
		}
		d.modelFloorLine = floor + " — BELOW, by --below-floor-why: " + d.belowFloorWhy
		d.warn("model floor: " + d.model + " is below " + d.proj + "'s floor (" + floor + "); launching on the stated reason: " + d.belowFloorWhy) // # MUTATION-ANCHOR: 844-B29
		return
	}
	for _, m := range vdIFSFields(accepts) {
		if m == d.model { // # MUTATION-ANCHOR: 844-B30
			d.modelFloorLine = floor + " — met (" + d.model + " is accepted by " + d.proj + "'s own decisions)"
			return
		}
	}
	d.modelFloorLine = floor + " — UNESTABLISHED: " + d.model + " is not on the Claude ladder and " + d.proj + "'s floor does not list it"
	d.warn("model floor: " + d.proj + " requires at least " + floor + ", and whether " + d.model + " meets it is UNESTABLISHED — it is not a Claude tier and projects.json does not accept it") // # MUTATION-ANCHOR: 844-B31
}
