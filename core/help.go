package main

// `hw help [<topic>]` (cmd_help, bin/hw:12398). The static text is the bash
// heredocs, taken verbatim into help/*.txt; the parts bash computes from the
// lane table are computed here.

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed help/*.txt
var helpFS embed.FS

func helpText(name string) string {
	b, err := helpFS.ReadFile("help/" + name + ".txt")
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (a *app) cmdHelp() {
	topic := ""
	if len(a.args) > 0 {
		topic = a.args[0]
	}
	switch topic {
	case "":
		a.usageShort()
	case "dispatch":
		a.usageLaunch()
		a.usagePlacement()
		a.usageProjects()
		a.usageEnvironment()
	case "flags":
		a.usageFlags()
	case "commands":
		a.usageCommands()
	case "exit-codes":
		a.usageExits()
	case "recovery":
		a.usageRecovery()
	case "advanced":
		a.usageAdvanced()
	case "lanes":
		a.usageLanes()
	case "all":
		a.usageLaunch()
		a.usagePlacement()
		a.usageFlags()
		a.usageCommands()
		a.usageRecovery()
		a.usageExits()
		a.usageProjects()
		a.usageEnvironment()
		a.usageAdvanced()
		a.usageLanes()
	default:
		a.die("no help topic '" + topic + "' — hw help lists them: dispatch, flags, commands, exit-codes, recovery, advanced, lanes, all") // # MUTATION-ANCHOR: 816-G13
	}
}

func (a *app) text(name string) { a.printf("%s", helpText(name)) }

func (a *app) usageShort()       { a.text("short-1") }
func (a *app) usageCommands()    { a.text("commands-1") }
func (a *app) usageRecovery()    { a.text("recovery-1") }
func (a *app) usageExits()       { a.text("exits-1") }
func (a *app) usageEnvironment() { a.text("environment-1") }
func (a *app) usageLanes()       { a.text("lanes-1") }

func (a *app) usageLaunch() {
	a.text("launch-1")
	t := a.c.t
	for _, l := range t.names {
		a.printf("    hw %s <task> --brief brain/%s/briefs/<task>.md --sdd none\n", l, l)
		v, m := t.get(l, "vendor"), t.get(l, "model")
		if v == "" {
			v = "claude"
		}
		if v != "claude" || m != "" {
			plus := ""
			if m != "" {
				plus = " + " + m
			}
			a.printf("        vendor: %s%s — no flag needed.\n", v, plus)
		}
		if a.c.checkout(l) != "" {
			b := t.get(l, "branch")
			if b != "task/{task}" {
				a.printf("        branch: %s\n", strings.ReplaceAll(b, "{task}", "<task>"))
			}
		}
	}
	a.text("launch-2")
}

func (a *app) usagePlacement() {
	a.text("placement-1")
	bases := ""
	for _, l := range a.c.t.names {
		if a.c.checkout(l) != "" {
			if bases != "" {
				bases += ", "
			}
			bases += l + " " + a.c.t.get(l, "base")
		}
	}
	for _, ln := range foldS("lane bases: "+bases, 72) {
		a.printf("        %s\n", strings.TrimRight(ln, " "))
	}
	a.text("placement-2")
}

func (a *app) usageFlags() {
	a.text("flags-1")
	db, dev := "", ""
	for _, l := range a.c.t.names {
		if a.c.t.get(l, "db_provisioned") != "" {
			if db != "" {
				db += ", "
			}
			db += l
		}
		if a.c.t.get(l, "devserver") == "opt-in" {
			dev += fmt.Sprintf("\n                         %-19s %s", l, a.c.t.get(l, "devserver_line"))
		}
	}
	if db == "" {
		db = "none" // # MUTATION-ANCHOR: 816-G28
	}
	if dev == "" {
		dev = " none" // # MUTATION-ANCHOR: 816-G27
	}
	a.printf("    --no-db            do not provision a database. Lanes that provision one:\n")
	a.printf("                       %s.\n", db)
	a.printf("    --dev-server       also start the dev server of a lane where it is opt-in,\n")
	a.printf("                       and allocate its HW_PORT_* ports:%s\n", dev)
	a.text("flags-2")
}

func (a *app) usageProjects() {
	a.text("projects-1")
	names, none := "", ""
	for _, l := range a.c.t.names {
		al := a.c.t.get(l, "hint_aliases")
		if names != "" {
			names += "    "
		}
		names += l
		if al != "" {
			names += " | " + strings.ReplaceAll(al, "|", " | ")
		}
		if a.c.checkout(l) == "" {
			if none != "" {
				none += ", "
			}
			none += l
		}
	}
	for _, ln := range foldS(names, 76) {
		a.printf("    %s\n", strings.TrimLeft(strings.TrimRight(ln, " "), " "))
	}
	if none != "" {
		a.printf("    no repository, so no worktree, no branch, no ports, no DB: %s\n\n", none)
	} else {
		a.printf("\n")
	}
}

func (a *app) usageAdvanced() {
	a.text("advanced-1")
	lines := ""
	for _, l := range a.c.t.names {
		acc := a.c.t.get(l, "account")
		if acc == "" {
			acc = "default"
		}
		if acc != "default" {
			if lines != "" {
				lines += ", "
			}
			lines += l + " " + acc
		}
	}
	if lines != "" {
		a.printf("    lanes whose own default account is not `default`: %s\n", lines)
	}
	a.text("advanced-2")
}

// foldS is `fold -s -w <width>` over one line of text, the way macOS's fold
// does it: a line that would pass the width breaks after its last blank, or
// at the width when it has none.
func foldS(text string, width int) []string {
	var out []string
	var buf []rune
	for _, r := range text {
		if len(buf)+1 > width { // # MUTATION-ANCHOR: 816-G24
			cut := -1
			for i := len(buf) - 1; i >= 0; i-- {
				if buf[i] == ' ' || buf[i] == '\t' {
					cut = i
					break
				}
			}
			if cut >= 0 {
				out = append(out, string(buf[:cut+1]))
				buf = append([]rune(nil), buf[cut+1:]...)
			} else {
				out = append(out, string(buf))
				buf = buf[:0]
			}
		}
		buf = append(buf, r)
	}
	out = append(out, string(buf))
	return out
}
