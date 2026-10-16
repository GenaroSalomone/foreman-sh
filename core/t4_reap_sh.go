package main

// The shell this port stands where `hw reap` ran: the small tools bash shells out to, run the same way (same
// argv, same stream, same exit status), so the commands a case records in `calls` are the ones bash ran.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// reapApplyExec runs name with args (in dir when it is not empty, feeding stdin), and answers both streams and the
// status: 127 when the program cannot be run at all, as the shell says.
func reapApplyExec(dir, stdin, name string, args ...string) (string, string, int) {
	r := extRun(extCall{Dir: dir, Stdin: stdin, Name: name, Args: args, KeepErr: true})
	return r.Out, r.Err, r.RC
}

// reapApplyOut is `name args… 2>/dev/null`: stdout and the status.
func reapApplyOut(name string, args ...string) (string, int) {
	o, _, rc := reapApplyExec("", "", name, args...)
	return o, rc
}

// reapApplyRC is `name args… >/dev/null 2>&1`.
func reapApplyRC(name string, args ...string) int {
	_, _, rc := reapApplyExec("", "", name, args...)
	return rc
}

// reapApplyGit is `git -C dir args…`: stdout, stderr and the status.
func reapApplyGit(dir string, args ...string) (string, string, int) {
	return reapApplyExec("", "", "git", append([]string{"-C", dir}, args...)...)
}

func reapApplyGitRC(dir string, args ...string) int {
	_, _, rc := reapApplyGit(dir, args...)
	return rc
}

// reapApplyFd is `fd args… 2>/dev/null`: what fd listed and its status (127 when fd is not installed).
func reapApplyFd(args ...string) (string, int) { return reapApplyOut("fd", args...) }

// reapApplySort is `sort [-u|-r]` over text, in the locale the case runs under; the text itself when sort cannot run.
func reapApplySort(text string, flags ...string) string {
	if text == "" {
		return ""
	}
	o, _, rc := reapApplyExec("", text, "sort", flags...)
	if rc != 0 {
		return text
	}
	return o
}

// reapApplyLines are the non-empty lines of a captured text, the way `while IFS= read -r l; do [ -n "$l" ] || continue`
// walks it.
func reapApplyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// reapApplyBasename is basename(1): trailing slashes dropped, "" for "".
func reapApplyBasename(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

// reapApplyDirname is dirname(1): "a/b/" is "a", "a" is ".", "/a" is "/".
func reapApplyDirname(p string) string {
	if p == "" {
		return "."
	}
	t := strings.TrimRight(p, "/")
	if t == "" {
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	d := strings.TrimRight(t[:i], "/")
	if d == "" {
		return "/"
	}
	return d
}

// reapApplyPad is printf's %-Ns: padded to N BYTES, as the C library does.
func reapApplyPad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// reapApplyPwdP is `cd dir && pwd -P`: the physical path of a directory, false when it cannot be entered.
func reapApplyPwdP(dir string) (string, bool) {
	if !isDir(dir) {
		return "", false
	}
	p, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return p, true
}

// reapApplyExists is `[ -e p ]`; reapApplyIsLink is `[ -L p ]`.
func reapApplyExists(p string) bool { _, err := os.Stat(p); return err == nil }
func reapApplyIsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func reapApplyExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// reapApplyAllDigits is not `case x in ”|*[!0-9]*)`: true for a non-empty run of digits.
func reapApplyAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// reapApplyAtoi is shell arithmetic over a digit string (0 when it is not one).
func reapApplyAtoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// reapApplyNow is `date +%s`: the clock of the program the PATH finds, so a world that fixes `date` fixes it for hw-core
// too. The Go clock only when date cannot answer.
func reapApplyNow() int64 {
	if o, rc := reapApplyOut("date", "+%s"); rc == 0 {
		if n, err := strconv.ParseInt(strings.TrimSpace(o), 10, 64); err == nil {
			return n
		}
	}
	return time.Now().Unix()
}

// reapApplyDate is `date +<format>`, with the Go layout to use when date cannot answer.
func reapApplyDate(format, layout string) string {
	if o, rc := reapApplyOut("date", format); rc == 0 {
		return bashSub(o)
	}
	return time.Now().Format(layout)
}

// reapApplyShellMatch is bash's `case x in pattern)`: * ? [set] and \ escapes, where * crosses a /.
func reapApplyShellMatch(pat, s string) bool {
	if pat == "" {
		return s == ""
	}
	switch pat[0] {
	case '*':
		for i := 0; i <= len(s); i++ {
			if reapApplyShellMatch(pat[1:], s[i:]) {
				return true
			}
		}
		return false
	case '?':
		return s != "" && reapApplyShellMatch(pat[1:], s[1:])
	case '[':
		if s == "" {
			return false
		}
		end := strings.IndexByte(pat[1:], ']')
		// a ']' right after '[' or '[!' belongs to the set
		j := 1
		neg := false
		if j < len(pat) && (pat[j] == '!' || pat[j] == '^') {
			neg = true
			j++
		}
		k := j
		if k < len(pat) && pat[k] == ']' {
			k++
		}
		close := strings.IndexByte(pat[k:], ']')
		if end < 0 || close < 0 {
			return s[0] == '[' && reapApplyShellMatch(pat[1:], s[1:])
		}
		set := pat[j : k+close]
		hit := false
		for i := 0; i < len(set); i++ {
			if i+2 < len(set) && set[i+1] == '-' {
				if s[0] >= set[i] && s[0] <= set[i+2] {
					hit = true
				}
				i += 2
			} else if set[i] == s[0] {
				hit = true
			}
		}
		return hit != neg && reapApplyShellMatch(pat[k+close+1:], s[1:])
	case '\\':
		if len(pat) > 1 {
			return s != "" && s[0] == pat[1] && reapApplyShellMatch(pat[2:], s[1:])
		}
	}
	return s != "" && s[0] == pat[0] && reapApplyShellMatch(pat[1:], s[1:])
}
