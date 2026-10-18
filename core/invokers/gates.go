package invokers

// Ports of two done-invoker gate blocks (bin/done-invoker, "A TASK WHOSE
// BRIEF DECLARED A BOUNDARY", "A BUILD CLOSES ON checks.md"). Pure functions: the receipt lookup
// stays with the caller, the refusal text is byte-identical to the bash and
// the caller adds the "done-invoker: " prefix.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// isRegular is bash's `[ -f path ]`: exists, follows symlinks, regular file.
func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// posixSpace is the [[:space:]] set.
func posixSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

func trimLeftFn(s string, f func(byte) bool) string {
	i := 0
	for i < len(s) && f(s[i]) {
		i++
	}
	return s[i:]
}

func trimRightFn(s string, f func(byte) bool) string {
	j := len(s)
	for j > 0 && f(s[j-1]) {
		j--
	}
	return s[:j]
}

// fileField is `sed -n "s/^KEY:[[:space:]]*//p" FILE | tail -1 | sed
// 's/[[:space:]]*$//'`: the last line that starts with "KEY:", prefix and
// surrounding whitespace removed. An unreadable file yields "".
func fileField(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + ":"
	last := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, prefix) {
			last = trimLeftFn(line[len(prefix):], posixSpace)
		}
	}
	return trimRightFn(last, posixSpace)
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// awkLower is awk's tolower under a UTF-8 locale (macOS awk 20200816 lowers
// whole characters, not just ASCII); bytes that are not valid UTF-8 are kept.
func awkLower(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && w == 1 {
			b.WriteByte(s[i])
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
		i += w
	}
	return b.String()
}

// resolveUnderRoot is `case "$x" in ""|/*) ;; *) x="$ROOT/$x" ;; esac`.
func resolveUnderRoot(root, p string) string {
	if p == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return root + "/" + p
}

// DesignGate is the inside of `if [ -n "$_dz_boundary" ]`: the design judgment
// must name the declared boundary, end APPROVED and judge the design on disk.
func DesignGate(artifactsRoot, boundary string) error {
	file := artifactsRoot + "/design-judgment.md"
	how := fmt.Sprintf("Judge the design first (judgment-day, design mode: references/design-mode.md), then write %s with the lines  boundary: %s  /  design: <file>  /  design_sha256: <sha256>  /  JUDGMENT: APPROVED. An ESCALATED design is reported with --blocked. NOTHING was sent or marked.", file, boundary)
	if !isRegular(file) {
		return die("%s", fmt.Sprintf("this task's brief declared a boundary, and there is no %s. %s", file, how))
	}
	if got := fileField(file, "boundary"); got != boundary {
		return die("%s", fmt.Sprintf("design-judgment.md judges boundary '%s', and this task's brief declared '%s'. A verdict about another boundary is not this task's. %s", got, boundary, how))
	}
	verdict := fileField(file, "JUDGMENT")
	switch {
	case strings.HasPrefix(verdict, "APPROVED"):
	case strings.HasPrefix(verdict, "ESCALATED"):
		return die("%s", "the design judgment is ESCALATED. That is a blocker, not a done: report it with done-invoker --blocked, naming the bypass classes the judges found. NOTHING was sent or marked.")
	default:
		last := verdict
		if last == "" {
			last = "none"
		}
		return die("%s", fmt.Sprintf("design-judgment.md has no 'JUDGMENT: APPROVED' line (last verdict: '%s'). %s", last, how))
	}
	rawDesign := fileField(file, "design")
	design := resolveUnderRoot(artifactsRoot, rawDesign)
	if design == "" || !isRegular(design) {
		return die("%s", fmt.Sprintf("design-judgment.md names no design file that exists ('%s'). %s", rawDesign, how))
	}
	want := fileField(file, "design_sha256")
	have := ""
	if b, err := os.ReadFile(design); err == nil {
		sum := sha256.Sum256(b)
		have = hex.EncodeToString(sum[:])
	}
	if want == "" || want != have {
		haveShown, wantShown := have, want
		if haveShown == "" {
			haveShown = "<unreadable>"
		}
		if wantShown == "" {
			wantShown = "<no design_sha256>"
		}
		return die("%s", fmt.Sprintf("the design file %s hashes to %s, and the verdict judged %s. The approved design is not the one on disk: judge the current one. NOTHING was sent or marked.", design, haveShown, wantShown))
	}
	return nil
}

// awkSpace is awk's [ \t].
func awkSpace(c byte) bool { return c == ' ' || c == '\t' }

// awkSpaceMark is awk's [ \t`*].
func awkSpaceMark(c byte) bool { return c == ' ' || c == '\t' || c == '`' || c == '*' }

// isSeparatorRow is /^[|: \t-]+$/.
func isSeparatorRow(l string) bool {
	if l == "" {
		return false
	}
	for i := 0; i < len(l); i++ {
		switch l[i] {
		case '|', ':', ' ', '\t', '-':
		default:
			return false
		}
	}
	return true
}

// ChecksGate is the inside of `if [ -n "$_ck" ]`: checks.md needs at least one
// row and a pass/fail/unresolved verdict on every row. It returns CHECKS_NOTE.
func ChecksGate(artifactsRoot string) (note string, err error) {
	file := artifactsRoot + "/checks.md"
	how := fmt.Sprintf("Write %s as a table, one row per requirement of the brief's Proof and Verification:  requirement | verdict | evidence | correction  (verdict: pass, fail or unresolved). NOTHING was sent or marked.", file)
	if !isRegular(file) {
		return "", die("%s", fmt.Sprintf("this is a build and there is no %s. %s", file, how))
	}
	var rows, pass, fail, unres, bad int
	why := ""
	hdr := false
	// awk (macOS, UTF-8 locale) aborts on the first record with an invalid
	// byte sequence, END never runs and the bash sees an empty summary: no rows.
	if b, rerr := os.ReadFile(file); rerr == nil && utf8.Valid(b) {
		for _, line := range strings.Split(string(b), "\n") {
			l := trimRightFn(trimLeftFn(line, awkSpace), awkSpace)
			if !strings.Contains(l, "|") || isSeparatorRow(l) {
				continue
			}
			l = strings.TrimPrefix(l, "|")
			c := strings.Split(l, "|")
			v := ""
			if len(c) > 1 {
				v = trimRightFn(trimLeftFn(awkLower(c[1]), awkSpaceMark), awkSpaceMark)
			}
			c1 := trimRightFn(trimLeftFn(c[0], awkSpace), awkSpace)
			if !hdr && v == "verdict" {
				hdr = true
				continue
			}
			rows++
			switch v {
			case "pass":
				pass++
			case "fail":
				fail++
			case "unresolved":
				unres++
			default:
				if bad == 0 {
					bad = rows
					if v == "" {
						why = "an empty verdict"
					} else {
						why = "the verdict \"" + v + "\""
					}
					why += " (" + c1 + ")"
				}
			}
		}
	}
	if rows <= 0 {
		return "", die("%s", fmt.Sprintf("%s has no rows: a table of requirement | verdict | evidence | correction with nothing under its header proves nothing. %s", file, how))
	}
	vocab := "Accepted verdicts, exactly and alone in the cell: pass, fail, unresolved (case and surrounding spaces or backticks do not matter; a qualifier such as \"pass (n/a)\" does). Put the qualifier in the evidence or correction column."
	if bad != 0 {
		return "", die("%s", fmt.Sprintf("row %d of %s has %s. %s %s", bad, file, why, vocab, how))
	}
	parts := ""
	if pass != 0 {
		parts += fmt.Sprintf(", %d pass", pass)
	}
	if fail != 0 {
		parts += fmt.Sprintf(", %d fail", fail)
	}
	if unres != 0 {
		parts += fmt.Sprintf(", %d unresolved", unres)
	}
	return "\nchecks.md: " + strings.TrimPrefix(parts, ", ") + " (" + file + ")", nil
}

var _ = filepath.Join
