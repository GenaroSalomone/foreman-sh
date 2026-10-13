package main

// Train T4: the large writers — done, next, revive, preview, reap --apply and
// sweep --apply. Each is a port of the bash it names (lib/hw/done.sh,
// lib/hw/next.sh, lib/hw/reap.sh, cmd_revive, cmd_preview, cmd_sweep in bin/hw),
// byte for byte, and each lives in its own t4_<verb>.go.
//
// What they share with bash is the commands they run — git, herdr, psql — and
// the order they run them in. The contract cases (setup/tests/contract-hw/
// cases-t4, world-t4.sh) record every call a case makes (calls.log) beside the
// bytes and the tree, so Go is held to the same calls as bash, not only the same
// answer.
//
// SHADOW NEVER WRITES. A writer under HW_IMPL=shadow is not run twice: bin/hw
// answers with bash alone (a message into a pane, a worktree removed, a database
// dropped cannot be done in two implementations at once). The writers are held
// by the contract run on fixture copies instead.

// t4Verbs are the verbs of this train; the value is whether an executor may run
// it (always false here: bin/hw's _executor_refuse gate lives in bash, and the
// shim sends every executor-role call to bash so the refusal text stays bash's).
var t4Verbs = map[string]bool{"done": false, "next": false, "revive": false, "preview": false, "reap": false, "sweep": false}

func isT4Verb(v string) bool { _, ok := t4Verbs[v]; return ok }

func (a *app) cmdT4(verb string) int {
	switch verb {
	case "done":
		return a.cmdDone()
	case "next":
		return a.cmdNext()
	case "revive":
		return a.cmdRevive()
	case "preview":
		return a.cmdPreview()
	case "reap":
		return a.cmdReapApply()
	case "sweep":
		return a.cmdSweepApply()
	}
	return 64
}

// hasApply reports whether argv carries `--apply` as a word of its own: bin/hw sends `reap` and `sweep` to the T4
// writer on exactly that (`case " $* " in *" --apply "*`), and to the T2 reader on its absence.
func hasApply(args []string) bool {
	for _, x := range args {
		if x == "--apply" {
			return true
		}
	}
	return false
}
