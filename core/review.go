package main

// review-range and review-delta (train T2): the base a Judgment Day target is built against, and what a rebase
// after the judging left to re-judge. Ports of cmd_review_range and cmd_review_delta (bin/hw:455-545). Both only
// read git; the text is the bash's.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// reviewWorkdir is `${HW_WORKDIR:-$PWD}`.
func (a *app) reviewWorkdir() string {
	if wd := a.getenv("HW_WORKDIR"); wd != "" {
		return wd
	}
	wd, _ := os.Getwd()
	return wd
}

// gitRun is `git -C dir <args> 2>/dev/null`: stdout, and whether it succeeded. stdin is optional.
func gitRun(dir string, stdin []byte, args ...string) ([]byte, bool) {
	return gitRunErr(dir, stdin, nil, args...)
}

// gitRunErr is gitRun with git's stderr sent to errw (nil: dropped). `_file_pid` in bash redirects nothing, so what
// git says about a file it cannot diff is part of the answer.
func gitRunErr(dir string, stdin []byte, errw io.Writer, args ...string) ([]byte, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = errw
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	err := cmd.Run()
	return out.Bytes(), err == nil
}

// reviewRange is cmd_review_range: the SHA on stdout and 0, or a line on stderr and 1.
func (a *app) reviewRange(print bool) (string, int) {
	wd, ref := a.reviewWorkdir(), a.getenv("HW_REVIEW_BASE_BRANCH")
	if ref == "" {
		fmt.Fprintf(a.err, "hw review-range: HW_REVIEW_BASE_BRANCH is not set, so there is no base to judge from (no worktree or no base was resolved at dispatch)\n")
		return "", 1
	}
	if _, ok := gitRun(wd, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); !ok {
		fmt.Fprintf(a.err, "hw review-range: base ref %s does not resolve in %s\n", ref, wd)
		return "", 1
	}
	out, _ := gitRun(wd, nil, "merge-base", "HEAD", ref)
	sha := strings.TrimRight(string(out), "\n")
	if sha == "" { // # MUTATION-ANCHOR: 871-G41
		fmt.Fprintf(a.err, "hw review-range: HEAD and %s share no history in %s, so there is no merge-base\n", ref, wd)
		return "", 1
	}
	if print {
		fmt.Fprintf(a.out, "%s\n", sha)
	}
	return sha, 0
}

func (a *app) cmdReviewRange() int {
	_, rc := a.reviewRange(true)
	return rc
}

// filePatchID is _file_pid: the patch-id of one file's diff, an empty line for no diff, "h:<hash>" for a diff git
// cannot patch-id; ok is false when git failed ("cannot tell").
func filePatchID(top string, errw io.Writer, from, to, path string) (string, bool) {
	d, ok := gitRunErr(top, nil, errw, "diff", "--no-color", "--no-renames", from, to, "--", ":(top,literal)"+path)
	if !ok {
		return "", false
	}
	if len(d) == 0 {
		return "", true
	}
	out, ok := gitRunErr(top, d, errw, "patch-id", "--stable")
	if !ok {
		return "", false
	}
	id := strings.TrimRight(string(out), "\n")
	if i := strings.Index(id, " "); i >= 0 { // ${id%% *}: up to the first space
		id = id[:i]
	}
	if id == "" {
		h, ok := gitRunErr(top, d, errw, "hash-object", "--stdin")
		if !ok {
			return "", false
		}
		id = "h:" + strings.TrimRight(string(h), "\n")
	}
	return id, true
}

// cmdReviewDelta is cmd_review_delta: 0 and silence when the verdict stands, 3 and one path per line when files
// must be re-judged, 1 when it cannot tell.
func (a *app) cmdReviewDelta() int {
	wd := a.reviewWorkdir()
	if len(a.args) != 2 || a.args[0] == "" || a.args[1] == "" {
		fmt.Fprintf(a.err, "usage: hw review-delta <judged-base> <judged-head>  (the range the last Judgment Day judged)\n")
		return 1
	}
	jb, jh := a.args[0], a.args[1]
	for _, f := range []string{jb, jh} {
		if _, ok := gitRun(wd, nil, "rev-parse", "--verify", "--quiet", f+"^{commit}"); !ok {
			fmt.Fprintf(a.err, "hw review-delta: judged ref %s does not resolve in %s\n", f, wd)
			return 1
		}
	}
	nb, rc := a.reviewRange(false)
	if rc != 0 {
		return 1
	}
	topOut, ok := gitRun(wd, nil, "rev-parse", "--show-toplevel")
	top := strings.TrimRight(string(topOut), "\n")
	if !ok || top == "" {
		fmt.Fprintf(a.err, "hw review-delta: %s is not inside a git work tree\n", wd)
		return 1
	}
	listErr := func() int {
		fmt.Fprintf(a.err, "hw review-delta: git could not list the changed files in %s\n", top)
		return 1
	}
	n1, ok1 := gitRun(top, nil, "diff", "--no-renames", "--name-only", "-z", jb, jh)
	n2, ok2 := gitRun(top, nil, "diff", "--no-renames", "--name-only", "-z", nb, "HEAD")
	if !ok1 || !ok2 {
		return listErr()
	}
	// sort -zu: the names of both diffs, once each
	seen := map[string]bool{}
	var names []string
	for _, chunk := range [][]byte{n1, n2} {
		for _, n := range bytes.Split(chunk, []byte{0}) {
			if len(n) == 0 || seen[string(n)] {
				continue
			}
			seen[string(n)] = true
			names = append(names, string(n))
		}
	}
	names = sortLocale(names, true)
	var changed strings.Builder
	for _, f := range names {
		idA, okA := filePatchID(top, a.err, jb, jh, f)
		var idB string
		okB := false
		if okA {
			idB, okB = filePatchID(top, a.err, nb, "HEAD", f)
		}
		if !okA || !okB {
			fmt.Fprintf(a.err, "hw review-delta: git could not diff %s, so it cannot tell whether it changed\n", f)
			return 1
		}
		if idA != idB { // # MUTATION-ANCHOR: 871-G40
			changed.WriteString(f + "\n")
		}
	}
	if changed.Len() == 0 {
		fmt.Fprintf(a.err, "hw review-delta: unchanged — the own diff %s..HEAD is the one judged as %s..%s; the verdict stands\n",
			cut12(nb), cut12(jb), cut12(jh))
		return 0
	}
	fmt.Fprint(a.out, changed.String())
	return 3
}

// cut12 is ${s:0:12}.
func cut12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
