package main

// `hw-core stop-hook stop`: bin/hw-stop-hook.sh in Go. The Stop hook every wired Claude Code and Codex session runs at
// each turn end, with its stdin the Stop payload. bin/hw-stop-hook.sh execs this when HW_IMPL_STOP_HOOK (or HW_IMPL, or
// the `stop-hook=` line of the switch file) says `go`; the bash it carries is the rollback and stays.
//
// What it does, in bash's own order (every branch is a line of the bash it ports, and a fix to one goes to both):
//
//	guards 1-3   the action is `stop`, the session is a herdr pane, the payload is not a subagent's (`"agent_id"`)
//	             -- anything else is inert, exit 0
//	brainer      renew the brainer's engram session lease; then, when ENGRAM_PROJECT is `brain` and this is no hw run,
//	             refuse a handback in the final paragraph (the Python heredoc of the bash, ported below)
//	guards 4-6   an hw EXECUTOR (HW_TASK, HW_RUN, HW_WORKDIR), whose current task has not reported, and whose session is
//	             the one hw registered for the run
//	turn end     `hw executor-turn-end --hook-json`, IN PROCESS (t3_turnend.go) instead of a second `hw` process: that
//	             process is the most of what the bash hook costs. A switch that pins that verb to bash keeps it a process.
//
// IT MUST NEVER BREAK A SESSION: the exit status is 0 whatever happens, a panic included, and stderr is never written.
// What differs from the bash, on purpose: no python3, jq or `hw` has to be on PATH for the Go to run (the bash goes
// silent without them), and Python's `json` accepts `NaN` where Go's refuses it (a transcript line with a NaN inside an
// object is read by the bash and skipped here).

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

func runStopHook(args []string, stdout io.Writer, getenv func(string) string, stdin io.Reader) (rc int) {
	defer func() {
		if r := recover(); r != nil {
			rc = 0
		}
	}()
	h := &stopHook{out: stdout, getenv: getenv}
	h.run(args, stdin)
	return 0
}

type stopHook struct {
	out    io.Writer
	getenv func(string) string
	input  string
}

func (h *stopHook) run(args []string, stdin io.Reader) {
	if len(args) == 0 || args[0] != "stop" { // guard 1: the action this file was wired for  # MUTATION-ANCHOR: 855-G01
		return
	}
	g := h.getenv
	if g("HERDR_ENV") != "1" || g("HERDR_SOCKET_PATH") == "" || g("HERDR_PANE_ID") == "" { // # MUTATION-ANCHOR: 855-G02
		return
	}
	// guard 3: the completed turn, read once (`head -c 262144`; a command substitution drops NULs and trailing newlines)
	data, _ := io.ReadAll(io.LimitReader(stdin, 262144))
	h.input = bashSub(string(data))
	if strings.Contains(h.input, `"agent_id"`) { // # MUTATION-ANCHOR: 855-G03
		return
	}

	if g("HW_RUN") == "" && g("BRAIN_ENGRAM_SESSION") != "" && g("ENGRAM_PROJECT") != "" { // # MUTATION-ANCHOR: 855-E01
		h.renewBrainerLease()
	}

	if g("HW_RUN") == "" && g("ENGRAM_PROJECT") == "brain" { // # MUTATION-ANCHOR: 855-B01
		h.brainerHandback()
		return
	}

	// guard 4: an hw executor
	if g("HW_TASK") == "" || g("HW_RUN") == "" || g("HW_WORKDIR") == "" {
		return
	}
	// guard 5: has this run's CURRENT task already reported (task 1 is the run dir, task N the t<N>/ of `hw next`)
	stateDir := g("HW_WORKDIR") + "/.hw/" + g("HW_RUN")
	seq := ""
	if b, err := os.ReadFile(stateDir + "/task"); err == nil {
		seq = t3Digits(string(b))
	}
	done := stateDir + "/done"
	switch seq {
	case "", "0", "1":
	default:
		done = stateDir + "/t" + seq + "/done" // # MUTATION-ANCHOR: 855-X01
	}
	if isFile(done) { // # MUTATION-ANCHOR: 855-X02
		return
	}
	// guard 6: is THIS session the executor hw registered for the run. Only a registered session narrows it.
	registered := lastLine(receiptValues(stateDir, "session_id"))
	payload := jqRawOr(h.input, "", "session_id")
	if registered != "" && !strings.HasPrefix(registered, "none") && payload != "" && payload != registered { // # MUTATION-ANCHOR: 855-X03
		return
	}
	h.turnEnd()
}

// renewBrainerLease is the block of the bash that keeps a brainer's engram session selectable: POST /sessions carries a
// 30-minute lease, renewed here at every brainer turn end, only on a serve proven to be this machine's store.
func (h *stopHook) renewBrainerLease() {
	t := &t3{app: &app{getenv: h.getenv}, fs: newFsx(h.getenv)}
	cwd, _ := os.Getwd()
	t.engramLeasePost(h.getenv("BRAIN_ENGRAM_SESSION"), h.getenv("ENGRAM_PROJECT"), cwd)
}

// turnEnd is `hw executor-turn-end --hook-json` with HW_TURN_END_PAYLOAD the hook's input: its stdout (the block JSON)
// is the hook's; its stderr is not.
func (h *stopHook) turnEnd() {
	inner := h.getenv
	getenv := func(k string) string {
		if k == "HW_TURN_END_PAYLOAD" {
			return h.input
		}
		return inner(k)
	}
	if m := implModeFor(getenv, "executor-turn-end"); m == "bash" || m == "shadow" { // # MUTATION-ANCHOR: 855-T01
		h.turnEndProcess(getenv)
		return
	}
	t := newTerm(h.out, io.Discard, false)
	brain := brainRoot(getenv)
	home := getenv("HOME")
	file := getenv("HW_PROJECTS_JSON")
	if file == "" {
		file = filepath.Join(brain, "projects.json")
	}
	tab, _ := loadTable(file, home) // a table that does not load: `hw` dies before the verb, and the hook is silent
	if tab == nil {
		return
	}
	work := getenv("WORK")
	if work == "" {
		work = tab.work
	}
	a := &app{term: t, args: []string{"--hook-json"}, getenv: getenv, stdin: strings.NewReader("")}
	a.c = &ctx{t: tab, brain: brain, work: work, home: home, getenv: getenv}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(exitNow); !ok {
					panic(r)
				}
			}
		}()
		a.cmdT3("executor-turn-end")
	}()
}

// turnEndProcess is the bash hook's last line, for a machine that pins executor-turn-end to bash (or to shadow, which
// `hw` runs): the `hw` beside this binary, its stderr dropped, its stdout the hook's.
func (h *stopHook) turnEndProcess(getenv func(string) string) {
	bin := getenv("HW_CORE_BIN_DIR")
	if bin == "" {
		return
	}
	hw := filepath.Join(bin, "hw")
	if !t3IsExec(hw) {
		return
	}
	cmd := exec.Command(hw, "executor-turn-end", "--hook-json")
	cmd.Env = append(os.Environ(), "HW_TURN_END_PAYLOAD="+h.input)
	cmd.Stdout = h.out
	cmd.Run()
}

// implModeFor is bin/hw's _core_impl_for for the switch as the shell hooks read it: HW_IMPL_<VERB>, HW_IMPL, the
// switch file's `<verb>=` line, the file's default line, bash.
func implModeFor(getenv func(string) string, verb string) string {
	up := strings.NewReplacer("-", "_").Replace(strings.ToUpper(verb))
	if v := getenv("HW_IMPL_" + up); v != "" {
		return v
	}
	if v := getenv("HW_IMPL"); v != "" {
		return v
	}
	file := getenv("HW_IMPL_FILE")
	if file == "" {
		cfg := getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = getenv("HOME") + "/.config"
		}
		file = cfg + "/hw/impl"
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "bash"
	}
	def, mine := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.Join(strings.FieldsFunc(line, unicode.IsSpace), "")
		switch {
		case line == "":
		case strings.Contains(line, "="):
			k, v, _ := strings.Cut(line, "=")
			if strings.NewReplacer("-", "_").Replace(strings.ToUpper(k)) == up {
				mine = v
			}
		default:
			def = line
		}
	}
	if mine == "" {
		mine = def
	}
	if mine == "" {
		return "bash"
	}
	return mine
}

// ── the brainer's handback refusal: the Python heredoc of the bash ───────────────────────────────────────────────

// pySpaceClass is the characters Python's `\s` (and str.split, str.strip) take for white space.
const pySpaceClass = `\t-\r\x1c-\x20\x85\xa0\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

func pyFields(s string) []string { return strings.FieldsFunc(s, pyIsSpace) }

func pyNorm(s string) string { return strings.Join(pyFields(pyCasefold(s)), " ") }

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

var (
	reParagraph = regexp.MustCompile(`\n[` + pySpaceClass + `]*\n`)
	reBoundary  = regexp.MustCompile(`(?im)^human boundary:[` + pySpaceClass + `]+[^` + pySpaceClass + `]`)
	reProposal  = regexp.MustCompile(`(?im)^(?:not requested|no pedido):[` + pySpaceClass + `]+[^` + pySpaceClass + `]`)
	reAmbiguous = regexp.MustCompile(`(?im)^(?:ambiguous|ambiguo):[` + pySpaceClass + `]*[«"“]([^»"”\n]{3,})[»"”][` + pySpaceClass + `]*[-—–:,]*[` + pySpaceClass + `]*(.+)$`)
	reReadings  = regexp.MustCompile(`[` + pySpaceClass + `]+(?:/|\||o|or|vs\.?)[` + pySpaceClass + `]+`)
	reTwoWord   = regexp.MustCompile(`[\p{L}\p{N}_]{2}`)
	reFence     = regexp.MustCompile("(?s)```.*?```")
	reInline    = regexp.MustCompile("`[^`]*`")
	reQuoted    = regexp.MustCompile(`"[^"\n]*"`)
)

// the concrete handback forms. Each starts and ends on a word character, and Python's `\b` there is Unicode-aware where
// RE2's is not, so the boundary is checked by hand (hasWord).
var handbackPatterns = func() []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range []string{
		`decime si (?:querés|quieres) que`,
		`avisame si (?:querés|quieres) que`,
		`(?:querés|quieres) que`,
		`sigue(?:n)? siendo (?:tuyo|tuya|tuyos|tuyas|suyo|suya|suyos|suyas)`,
		`necesita(?:n)? (?:un |una )?dueñ[oa]`,
		`let me know if you want me to`,
		`tell me if you want me to`,
		`do you want me to`,
		`would you like me to`,
		`should i`,
		`remains? yours`,
		`needs? an? owner`,
	} {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}()

// hasWord is re.search(r"\b<re>\b", s).
func hasWord(re *regexp.Regexp, s string) bool {
	for from := 0; from <= len(s); {
		loc := re.FindStringIndex(s[from:])
		if loc == nil {
			return false
		}
		start, end := from+loc[0], from+loc[1]
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if (start == 0 || !isWordRune(before)) && (end == len(s) || !isWordRune(after)) {
			return true
		}
		from = start + 1
		for from < len(s) && !utf8.RuneStart(s[from]) {
			from++
		}
	}
	return false
}

// the reason is a constant but for the operator's name.
func handbackReason(operator string) string {
	return "A mechanical handback signature appeared in your final paragraph. " +
		"Do not ask " + operator + " to choose or own reversible, scoped work: decide and act now. " +
		"If action crosses a genuine human boundary (money, a third party, an irreversible " +
		"or outward action, or conflicting priorities), do not act; end with " +
		"'Human boundary: <specific boundary and why only " + operator + " can decide>.' " +
		"If the work is something " + operator + " did not ask for, do not launch it either: " +
		"propose it and end with 'Not requested: <what you propose, and that it was not asked for>.' " +
		"If an order of " + operator + "'s reads two ways that lead to different actions, hold only " +
		"what depends on it, ask one closed question, and end with " +
		"'Ambiguous: «<" + operator + "'s exact words>» — <reading A> / <reading B>.'"
}

func (h *stopHook) brainerHandback() {
	g := h.getenv
	brain := brainRoot(g)
	file := g("HW_PROJECTS_JSON")
	if file == "" {
		file = filepath.Join(brain, "projects.json")
	}
	tab, _ := loadTable(file, g("HOME"))
	if tab == nil || len(tab.names) == 0 { // a table that does not load disables this refusal
		return
	}
	operator := g("HW_OPERATOR")
	if operator == "" {
		operator = tab.operator
	}
	if operator == "" {
		operator = "the operator"
	}

	if !utf8.ValidString(h.input) { // Python reads the event with a strict decoder
		return
	}
	var event map[string]any
	if json.Unmarshal([]byte(h.input), &event) != nil || event == nil {
		return
	}
	cwdV, has := event["cwd"]
	if !has {
		cwdV = ""
	}
	cwd, ok := cwdV.(string)
	if !ok { // re.fullmatch of a non-string raises: no output
		return
	}
	quoted := make([]string, len(tab.names))
	for i, n := range tab.names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	inLane := regexp.MustCompile(`^` + regexp.QuoteMeta(brain) + `/(?:` + strings.Join(quoted, "|") + `)(?:/.*)?$`)
	if !inLane.MatchString(cwd) { // # MUTATION-ANCHOR: 855-B02
		return
	}

	message, ok := event["last_assistant_message"].(string)
	if !ok || pyStrip(message) == "" {
		return
	}
	var paragraphs []string
	for _, part := range reParagraph.Split(message, -1) {
		if p := pyStrip(part); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	if len(paragraphs) == 0 {
		return
	}
	final := paragraphs[len(paragraphs)-1]

	// the three legal exits: a human boundary, a proposal of unrequested work, an order that reads two ways
	if reBoundary.MatchString(final) { // # MUTATION-ANCHOR: 855-B03
		return
	}
	if reProposal.MatchString(final) { // # MUTATION-ANCHOR: 855-B04
		return
	}
	if m := reAmbiguous.FindStringSubmatch(final); m != nil && namesTwoReadings(m[2]) && operatorSaid(m[1], event["transcript_path"]) { // # MUTATION-ANCHOR: 855-B05
		return
	}

	// examples and quoted material are not what the brainer hands back
	candidate := reFence.ReplaceAllString(final, " ")
	candidate = reInline.ReplaceAllString(candidate, " ")
	candidate = reQuoted.ReplaceAllString(candidate, " ")
	candidate = strings.Join(pyFields(pyCasefold(candidate)), " ")
	hit := false
	for _, re := range handbackPatterns {
		if hasWord(re, candidate) { // # MUTATION-ANCHOR: 855-B06
			hit = true
			break
		}
	}
	if !hit {
		return
	}
	h.out.Write([]byte(`{"decision":"block","reason":` + stopPyJSON(handbackReason(operator)) + "}\n"))
}

// stopPyJSON is json.dumps(s) for a str: ensure_ascii, so every non-ASCII character is \uXXXX.
func stopPyJSON(s string) string { return t3PyJSONStr(s) }

func namesTwoReadings(rest string) bool {
	rest = pyStrip(rest)
	loc := reReadings.FindStringIndex(rest)
	if loc == nil {
		return false
	}
	return reTwoWord.MatchString(rest[:loc[0]]) && reTwoWord.MatchString(rest[loc[1]:])
}

// operatorSaid is operator_said: the quote is in one of the operator's recent messages, or the transcript is unreadable.
func operatorSaid(words string, transcript any) bool {
	path := ""
	switch p := transcript.(type) {
	case nil:
	case string:
		path = p
	default: // a non-string path is neither a file nor a failure the bash survives: unreadable
		return true
	}
	said, ok := operatorTexts(path)
	if !ok {
		return true
	}
	w := pyNorm(words)
	for _, text := range said {
		if strings.Contains(pyNorm(text), w) {
			return true
		}
	}
	return false
}

func stopTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// operatorTexts is operator_texts: the operator's last ten messages of the tail (4 MiB) of the transcript, and false
// when it cannot be read (the bash's None).
func operatorTexts(path string) (texts []string, ok bool) {
	const keep, tail = 10, 4 << 20
	f, err := os.Open(path)
	if err != nil || path == "" {
		return nil, false
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, false
	}
	off := end - tail
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, false
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	var sb strings.Builder
	for i := 0; i < len(raw); { // bytes.decode("utf-8", "replace"): one U+FFFD per bad byte
		r, sz := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && sz == 1 {
			sb.WriteRune(0xfffd)
		} else {
			sb.Write(raw[i : i+sz])
		}
		i += sz
	}
	for _, line := range pySplitLines(sb.String()) {
		var entry any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		e, isDict := entry.(map[string]any)
		if !isDict || stopTruthy(e["isMeta"]) {
			continue
		}
		var content any
		if e["type"] == "user" { // Claude Code
			msg := e["message"]
			if !stopTruthy(msg) {
				content = nil
			} else if m, isMap := msg.(map[string]any); isMap {
				content = m["content"]
			} else {
				return nil, false // .get of a non-dict raises: the whole read is None
			}
		} else {
			payload := e["payload"]
			var role any
			var pm map[string]any
			if stopTruthy(payload) {
				m, isMap := payload.(map[string]any)
				if !isMap {
					return nil, false
				}
				pm, role = m, m["role"]
			}
			if role != "user" { // Codex
				continue
			}
			content = pm["content"]
		}
		if parts, isList := content.([]any); isList {
			var joined []string
			for _, part := range parts {
				pm, isMap := part.(map[string]any)
				if !isMap {
					continue
				}
				tv, has := pm["text"]
				if !has {
					continue
				}
				s, isStr := tv.(string)
				if !isStr {
					return nil, false // " ".join of a non-str raises
				}
				joined = append(joined, s)
			}
			content = strings.Join(joined, " ")
		}
		if s, isStr := content.(string); isStr && pyStrip(s) != "" && !strings.HasPrefix(s, "Stop hook feedback") {
			texts = append(texts, s)
			if len(texts) > keep {
				texts = texts[len(texts)-keep:]
			}
		}
	}
	return texts, len(texts) > 0
}
