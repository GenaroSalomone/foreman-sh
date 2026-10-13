package main

// The shell sorts and globs in the user's locale: `sort`, and bash's `*`, put `a.txt` before `README` and `b c`
// before `bc` under en_US.UTF-8, and that is a table in the C library, not a rule to port. Where the order of the
// bash is the contract, hw-core asks the same `sort` the bash asks, under the same environment.

import (
	"bytes"
	"os/exec"
	"strings"
)

// sortLocale is `printf '%s\0' items | sort -z [-u]`.
func sortLocale(items []string, unique bool) []string {
	if len(items) < 2 && !unique {
		return items
	}
	args := []string{"-z"}
	if unique {
		args = append(args, "-u")
	}
	cmd := exec.Command("sort", args...)
	var in bytes.Buffer
	for _, s := range items {
		in.WriteString(s)
		in.WriteByte(0)
	}
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		return items // no sort(1): the order we were given
	}
	var res []string
	for _, s := range strings.Split(string(out), "\x00") {
		if s != "" {
			res = append(res, s)
		}
	}
	return res
}

// sortLinesNumeric is `sort -n` over newline-terminated lines.
func sortLinesNumeric(lines []string) []string {
	cmd := exec.Command("sort", "-n")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return lines
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}
