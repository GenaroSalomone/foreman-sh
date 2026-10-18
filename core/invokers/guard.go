package invokers

import "strings"

// exitMeaning is _inv_exit_meaning of each invoker.
func (c *Ctx) exitMeaning(rc int) string {
	if c.Prog == "ask-invoker" {
		switch rc {
		case 1:
			return "NOT DELIVERED — no ask was spent and no marker written, so nothing was lost; retry the identical command"
		case 4:
			return "DELIVERED and provably in the brainer session, but its TURN failed — do NOT re-ask"
		case 5:
			return "DELIVERY UNCERTAIN — the question MAY already be with the brainer; do NOT re-ask, wait for the reply or read its pane"
		}
		return "a failure, not a delivery"
	}
	switch rc {
	case 1:
		return "NOT DELIVERED — nothing was reported, no marker written, this pane is the only copy; retry the identical command"
	case 4:
		return "DELIVERED AND SAFE, but the owned tab/workspace could not be closed — do NOT re-run this, close it by hand"
	case 5:
		return "DELIVERY UNCERTAIN — the report MAY already be in the brainer's session; do NOT re-run this, read its pane first"
	case 6:
		return "HELD FOR A RULING — nothing was reported; act on the ruling printed above, THEN run done-invoker again (not before)"
	}
	return "a failure, not a report"
}

// exitGuard is _inv_exit_guard: once, for a non-zero status, when stdout is not
// a terminal, say on stderr that a pipe can swallow the number. (899-M07)
func (c *Ctx) exitGuard(rc int) {
	if c.exitSpoken {
		return
	}
	c.exitSpoken = true
	if rc == 0 || c.StdoutTTY {
		return
	}
	n := itoa(rc)
	c.errf("%s: ══ MY EXIT CODE IS %s: %s ══\n", c.Prog, n, c.exitMeaning(rc))
	c.errf("%s: my stdout is NOT a terminal, so this number can be swallowed. If you piped me — `%s … | tail` — then `$?` is TAIL's status (0 whenever it reads a byte), NOT mine: run me unpiped — `%s … ; echo rc=$?` reads the real status in bash and zsh (`${PIPESTATUS[0]}` is bash-only: zsh spells it `${pipestatus[1]}` and prints nothing for the bash spelling). If you captured me with `$(…)` or `|| rc=$?` you already have it, and it is %s. Either way the %s fact is %s, not whatever the shell shows.\n",
		c.Prog, c.Prog, c.Prog, n, guardNoun(c.Prog), n)
}

func guardNoun(prog string) string {
	if prog == "done-invoker" {
		return "report"
	}
	return "delivery"
}

var _ = strings.TrimSpace
