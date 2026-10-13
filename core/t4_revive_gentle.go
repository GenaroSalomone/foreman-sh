package main

// The framework args a gentle run is revived with (_revive_gentle_args, bin/hw:8033) and the readiness of the
// gentle-ai home it reads (_gentle_home_ready and _gentle_home_refusal, bin/hw:5020-5100). Read-only: files and
// two `git -C` questions, which the recorder of the contract world logs.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"unicode"
)

const reviveGentleVersion = "4.0.0"

// reviveFramework is _run_dispatch_framework: the second word of the first `  framework …` line of the dispatch.
func reviveFramework(rundir string) string {
	data, err := os.ReadFile(rundir + "/dispatch")
	if err != nil || !readableFile(rundir+"/dispatch") {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if len(l) > len("  framework") && strings.HasPrefix(l, "  framework") && unicode.IsSpace(rune(l[len("  framework")])) {
			f := strings.FieldsFunc(l, func(r rune) bool { return r == ' ' || r == '\t' })
			if len(f) >= 2 {
				return f[1]
			}
			return ""
		}
	}
	return ""
}

func (a *app) reviveGentleHome() string {
	if d := a.getenv("HW_GENTLE_HOME"); d != "" {
		return d
	}
	return a.getenv("HOME") + "/.local/share/hw/gentle-home"
}

// reviveGentleArgs prints the args, nothing for a run that was not gentle; ok is false, with why, when the home
// is not complete.
func (a *app) reviveGentleArgs(rundir string) (args string, ok bool) {
	fw := reviveFramework(rundir)
	if fw != "gentle" {
		return "", true
	}
	gh := a.reviveGentleHome()
	if why, ready := a.reviveGentleReady(gh); !ready {
		return "gentle-ai's own home is not ready (" + why + "), and this run was launched --sdd gentle: reviving it without ODD would resume it with the protocol silently gone. Check it with: hw gentle-home --check", false
	}
	return "--append-system-prompt-file " + gh + "/odd-block.md --plugin-dir " + gh + "/plugin/gentle-odd --settings " + gh + "/guard-settings.json", true // # MUTATION-ANCHOR: 863-G06
}

func (a *app) reviveGit(args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	return string(out), err == nil
}

// reviveGentleRefusal is _gentle_home_refusal: why a destination is not allowed, or "" when it is.
func (a *app) reviveGentleRefusal(d string) string {
	home := a.getenv("HOME")
	if !strings.HasPrefix(d, "/") {
		return "not an absolute path: " + d
	}
	if strings.IndexFunc(d, unicode.IsSpace) >= 0 {
		return "the path has whitespace, and hw passes it as one word: " + d
	}
	d = strings.TrimSuffix(d, "/")
	h := strings.TrimSuffix(home, "/")
	if d == h {
		return "it is HOME itself"
	}
	for _, cfg := range []string{".claude", ".claude-personal", ".claude.json", ".config/opencode", ".codex", ".gentle-ai"} {
		if strings.HasPrefix(d+"/", h+"/"+cfg+"/") {
			return "it is inside ~/" + cfg + ", which another session reads"
		}
	}
	p := d
	for !isDir(p) && p != "/" {
		p = path.Dir(p)
	}
	if _, in := a.reviveGit("-C", p, "rev-parse", "--is-inside-work-tree"); in {
		if _, ignored := a.reviveGit("-C", p, "check-ignore", "-q", d); !ignored {
			top, _ := a.reviveGit("-C", p, "rev-parse", "--show-toplevel")
			return "it is inside a git work tree (" + bashSub(top) + ") and not ignored there"
		}
	}
	return ""
}

// reviveJqFileAlt is `jq -r '<path> // empty' FILE`: the output and jq's status (5 when it cannot index or parse).
// jq's own diagnostics reach stderr as they are found (the callers in bash do not silence them): a value it cannot
// index is `jq: error (at FILE:N): …` with N the lines read, a syntax error `jq: parse error: …` once the values
// before it were answered.
func reviveJqFileAlt(file string, steps func(*jval) (*jval, string)) (string, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	vals, perr := parseStream(data)
	var outs []string
	good := true
	for _, v := range vals {
		r, msg := steps(v)
		if msg != "" {
			reviveJqError(file, data, msg)
			good = false
			continue
		}
		good = true
		if r != nil && r.truthy() {
			outs = append(outs, reviveRaw(r))
		}
	}
	if perr != nil {
		reviveJqParseError(data, perr)
		good = false
	}
	return bashSub(strings.Join(outs, "\n")), good
}

func reviveJqError(file string, data []byte, msg string) {
	fmt.Fprintf(os.Stderr, "jq: error (at %s:%d): %s\n", file, strings.Count(string(data), "\n"), msg)
}

func reviveJqParseError(data []byte, perr error) {
	fmt.Fprintf(os.Stderr, "jq: parse error: %s\n", reviveJqParseText(data, perr.Error()))
}

// reviveJqParseText words a bare-word syntax error the way jq 1.7.1 does. The shared parser stops at the end of
// the token and calls every such word "Invalid literal"; jq calls it that only for a word that starts like
// true/false/null (and is not one), "Invalid numeric literal" otherwise, adds "at EOF" when the data ended with the
// word, and counts the delimiter that ended it in the column (a newline starts the next line at column 0). Any
// other error is left as the parser has it.
func reviveJqParseText(data []byte, msg string) string {
	var line, col int
	if _, err := fmt.Sscanf(msg, "Invalid literal at line %d, column %d", &line, &col); err != nil {
		return msg
	}
	off, l, c := 0, 1, 0
	for off < len(data) && (l < line || c < col) {
		if data[off] == '\n' {
			l++
			c = 0
		} else {
			c++
		}
		off++
	}
	start := off
	for start > 0 && !isDelim(data[start-1]) {
		start--
	}
	tok := string(data[start:off])
	kind := "Invalid numeric literal"
	switch {
	case tok == "":
		return msg
	case tok[0] == 't' || tok[0] == 'f' || (tok[0] == 'n' && len(tok) > 1 && (tok[1] == 'u' || tok[1] == 'a')):
		kind = "Invalid literal"
	}
	if off >= len(data) {
		return fmt.Sprintf("%s at EOF at line %d, column %d", kind, line, col)
	}
	if data[off] == '\n' {
		return fmt.Sprintf("%s at line %d, column 0", kind, line+1)
	}
	return fmt.Sprintf("%s at line %d, column %d", kind, line, col+1)
}

// reviveJqStep is `.a.b.c` on one value, with jq's message when it cannot.
func reviveJqStep(v *jval, path ...string) (*jval, string) {
	cur := v
	for _, p := range path {
		n, ok := cur.get(p)
		if !ok {
			return nil, "Cannot index " + kindName(cur) + " with string \"" + p + "\""
		}
		cur = n
	}
	return cur, ""
}

// reviveJqIndex is `.[i]` of one value, with jq's message when it cannot.
func reviveJqIndex(v *jval, i int) (*jval, string) {
	switch v.kind {
	case vNull:
		return jnull, ""
	case vArr:
		if i < len(v.arr) {
			return v.arr[i], ""
		}
		return jnull, ""
	}
	return nil, "Cannot index " + kindName(v) + " with number"
}

// reviveHooksEmpty is `.hooks // {} | length` as the test `= 0` reads it: whether that length is zero, and
// whether jq could say.
func reviveHooksEmpty(file string) (zero, ok bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return false, false
	}
	vals, perr := parseStream(data)
	var last string
	good := true
	for _, v := range vals {
		h, msg := reviveJqStep(v, "hooks")
		if msg != "" {
			reviveJqError(file, data, msg)
			good = false
			continue
		}
		good = true
		if !h.truthy() {
			last = "0"
			continue
		}
		switch h.kind {
		case vArr:
			last = strconv.Itoa(len(h.arr))
		case vObj:
			last = strconv.Itoa(len(h.obj))
		case vStr:
			last = strconv.Itoa(len([]rune(h.s)))
		case vNum:
			if n, err := strconv.ParseFloat(h.s, 64); err == nil && n == 0 {
				last = "0"
			} else {
				last = "n"
			}
		default: // true has no length
			reviveJqError(file, data, "boolean (true) has no length")
			good = false
		}
	}
	if perr != nil {
		reviveJqParseError(data, perr)
		good = false
	}
	return last == "0", good
}

// reviveGentleReady is _gentle_home_ready: what is missing, or ok.
func (a *app) reviveGentleReady(d string) (string, bool) {
	if why := a.reviveGentleRefusal(d); why != "" {
		return why, false
	}
	m := d + "/manifest.json"
	if !isFile(m) {
		return "no " + m, false
	}
	v, ok := reviveJqFileAlt(m, func(x *jval) (*jval, string) { return reviveJqStep(x, "version") })
	if !ok {
		return m + " is not readable JSON", false
	}
	if v != reviveGentleVersion {
		return m + " is not version " + reviveGentleVersion, false
	}
	fo := d + "/odd-block.md"
	if st, err := os.Stat(fo); err != nil || st.Size() == 0 {
		return "no ODD block at " + fo, false
	}
	block, _ := os.ReadFile(fo)
	if !strings.Contains(string(block), "<!-- gentle-ai:agent-routing -->") {
		return fo + " carries no gentle-ai agent-routing block", false
	}
	if strings.Contains(string(block), "<!-- gentle-ai:orchestrator -->") {
		return fo + " carries the orchestrator block, and only the ODD protocol travels", false
	}
	sum := sha256.Sum256(block)
	want, _ := reviveJqFileAlt(m, func(x *jval) (*jval, string) { return reviveJqStep(x, "odd_block_sha256") })
	if hex.EncodeToString(sum[:]) != want {
		return fo + " changed after provisioning", false
	}
	if !isFile(d + "/plugin/gentle-odd/.claude-plugin/plugin.json") {
		return "no plugin at " + d + "/plugin/gentle-odd", false
	}
	if !isFile(d + "/guard-settings.json") {
		return "no " + d + "/guard-settings.json", false
	}
	task := d + "/bin/gentle-ai-task"
	if st, err := os.Stat(task); err != nil || st.Mode()&0o111 == 0 || !reviveFileHas(task, "rdd-log.jsonl") {
		return "no bin/gentle-ai-task that runs the native reviewer and logs it in " + d + " (re-provision: rm -rf " + d + " && hw gentle-home)", false
	}
	guard, ok := reviveJqFileAlt(d+"/guard-settings.json", func(x *jval) (*jval, string) {
		cur, m := reviveJqStep(x, "hooks", "PreToolUse")
		if m != "" {
			return nil, m
		}
		if cur, m = reviveJqIndex(cur, 0); m != "" {
			return nil, m
		}
		if cur, m = reviveJqStep(cur, "hooks"); m != "" {
			return nil, m
		}
		if cur, m = reviveJqIndex(cur, 0); m != "" {
			return nil, m
		}
		return reviveJqStep(cur, "command")
	})
	if !ok {
		return d + "/guard-settings.json is not readable JSON", false
	}
	if i := strings.LastIndexByte(guard, ' '); i >= 0 {
		guard = guard[i+1:]
	}
	if guard == "" || !isFile(guard) {
		shown := guard
		if shown == "" {
			shown = "<none>"
		}
		return "the guard " + d + "/guard-settings.json names does not exist: " + shown + " (re-provision from the brain checkout: rm -rf " + d + " && hw gentle-home)", false
	}
	if isFile(d + "/home/.claude/settings.json") {
		zero, ok := reviveHooksEmpty(d + "/home/.claude/settings.json")
		if !ok {
			return d + "/home/.claude/settings.json is not readable JSON", false
		}
		if !zero {
			return d + "/home/.claude/settings.json carries hooks, and this mode runs none of gentle-ai's", false
		}
	}
	return "", true
}

func reviveFileHas(file, needle string) bool {
	b, err := os.ReadFile(file)
	return err == nil && strings.Contains(string(b), needle)
}
