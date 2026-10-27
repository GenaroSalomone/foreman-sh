package main

// The picker: `hw` with no arguments on a terminal (go-060 S2) — _interactive and _new_brief of bin/hw, ported.
// It lists every brief of the lanes (the rows of ledgerPickerRows, aligned by `column -t`), lets fzf choose one and
// becomes `hw <lane> <task>`; the last row authors a new brief from BRIEF-TEMPLATE.md and launches it. The same external
// programs bash ran run here (column, fzf, sed, the editor), so what they print and how they fail is theirs, not a copy.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// pickerFzf runs fzf over input with the arguments bash gave it. The terminal is fzf's own (it opens /dev/tty); what it
// prints is returned without its trailing newlines (`$(…)`), and ok is false when it ended without a choice (ctrl-c, no match).
func (a *app) pickerFzf(input string, args ...string) (pick string, ok bool) {
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return strings.TrimRight(out.String(), "\n"), true
}

// cmdPicker is _interactive.
func (a *app) cmdPicker() int {
	if _, err := exec.LookPath("fzf"); err != nil {
		a.die("fzf not installed — brew install fzf")
	}
	if !isDir(a.c.brain) {
		a.die("no brain at " + a.c.brain)
	}
	// `rows="$(_picker_rows)"$'\n'` strips the trailing newlines of the rows, then ends them with one
	rows := strings.TrimRight(ledgerPickerRows(a.c.brain, a.c.t.names), "\n") + "\n" + "+ new brief\t\t(author one from BRIEF-TEMPLATE.md)"
	col := exec.Command("column", "-t", "-s", "\t")
	col.Stdin = strings.NewReader(rows + "\n")
	col.Stderr = os.Stderr
	table, err := col.Output()
	if err != nil {
		return 0
	}
	pick, ok := a.pickerFzf(string(table), "--height=40%", "--reverse", "--prompt=task > ", "--header=enter = launch    ctrl-c = cancel")
	if !ok || pick == "" {
		return 0
	}
	for _, ln := range strings.Split(pick, "\n") {
		if strings.HasPrefix(ln, "+ new brief") { // grep -q '^+ new brief'
			return a.pickerNewBrief()
		}
	}
	f := strings.Fields(pick) // awk '{print $1}', '{print $2}'
	proj, task := "", ""
	if len(f) > 0 {
		proj = f[0]
	}
	if len(f) > 1 {
		task = f[1]
	}
	if task == "" {
		a.die("could not parse the selection")
	}
	a.info("launching " + proj + " / " + task)
	return a.pickerExecHw(proj, task)
}

// pickerExecHw is bash's `exec "$0" <argv>`: this process becomes `hw <argv>`.
func (a *app) pickerExecHw(argv ...string) int {
	hw := a.hwBin()
	err := execReplace(hw, append([]string{hw}, argv...), os.Environ())
	a.die("cannot run " + hw + ": " + err.Error())
	return 1
}

// pickerReadLine is `read -r`: one line from stdin, a byte at a time so nothing past the newline is consumed (the editor and
// the hw that follows read the same terminal), with the leading and trailing blanks of the default IFS removed.
func pickerReadLine(r io.Reader) string {
	var sb strings.Builder
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				break
			}
			sb.WriteByte(b[0])
		}
		if err != nil {
			break
		}
	}
	return strings.Trim(sb.String(), " \t\n")
}

// untouchedTemplate is `! grep -qv -e '^[[:space:]]*$' -e '^#' -e '^<' -e '^- <' -e '^- \[ \] <'`: every line is blank, a heading
// or an angle-bracket placeholder (an empty file too: grep -v finds no line that is not).
var pickerTemplateLine = regexp.MustCompile(`^[ \t\r\f\v]*$|^#|^<|^- <|^- \[ \] <`)

func pickerUntouched(body string) bool {
	if body == "" {
		return true
	}
	for _, ln := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if !pickerTemplateLine.MatchString(ln) {
			return false
		}
	}
	return true
}

// pickerNewBrief is _new_brief: author a brief, then launch it. The file is created in brain/, never in a repo.
func (a *app) pickerNewBrief() int {
	proj, ok := a.pickerFzf(strings.Join(a.c.t.names, "\n")+"\n", "--height=20%", "--reverse", "--prompt=project > ")
	if !ok || proj == "" {
		return 0
	}
	a.printf("task name (kebab-case): ")
	task := pickerReadLine(a.stdin)
	if task == "" {
		a.die("no task name given")
	}
	if strings.Contains(task, "/") {
		a.die("task must be a bare name: " + task)
	}
	file := filepath.Join(a.c.brain, proj, "briefs", task+".md")
	if _, err := os.Lstat(file); err == nil {
		a.warn("brief already exists, opening it: " + file)
	} else {
		(&dsp{app: a}).archivedBriefGate(proj, task)
		if err := os.MkdirAll(filepath.Dir(file), 0o777); err != nil {
			a.die("cannot create " + filepath.Dir(file) + ": " + err.Error())
		}
		// a project may carry its own template; fall back to the shared one (docs/, else the root it used to sit in)
		tpl := filepath.Join(a.c.brain, proj, "BRIEF-TEMPLATE.md")
		if !isFile(tpl) {
			tpl = filepath.Join(a.c.brain, "docs", "BRIEF-TEMPLATE.md")
		}
		if !isFile(tpl) {
			tpl = filepath.Join(a.c.brain, "BRIEF-TEMPLATE.md")
		}
		dst, err := os.Create(file)
		if err != nil {
			a.die("cannot create " + file + ": " + err.Error())
		}
		sed := exec.Command("sed", "s/<task-name>/"+task+"/", tpl)
		sed.Stdout, sed.Stderr = dst, os.Stderr
		runErr := sed.Run()
		dst.Close()
		if runErr != nil {
			return pickerRC(runErr)
		}
		a.ok("created " + file)
	}
	ed := a.getenv("VISUAL")
	if ed == "" {
		ed = a.getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	edit := exec.Command(ed, file)
	edit.Stdin, edit.Stdout, edit.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := edit.Run(); err != nil {
		return pickerRC(err)
	}
	// an untouched template is a brief nobody wrote; launching on it would hand the agent a page of angle brackets
	if body, err := os.ReadFile(file); err == nil && pickerUntouched(string(body)) {
		a.warn("brief still looks like the untouched template — launching anyway")
	}
	return a.pickerExecHw(proj, task, "--brief", file)
}

// pickerRC is the status a failed child leaves the picker with (127 when it could not be started, as a shell says).
func pickerRC(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 127
}
