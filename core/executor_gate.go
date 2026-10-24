package main

// An executor does not have the brainer's verbs: the gate of bin/hw's main (`_HW_CALLER_EXECUTOR`, `_executor_refuse`,
// bin/hw:13347-13400), in Go. `hw` exports HW_TASK and HW_RUN into every executor, so their presence is the caller's
// role. What an executor runs: done on its OWN task (done-invoker), executor-turn-end, receipt, status, handle, suite,
// human-boundary, review-range/-delta, output, help, and the rest of the list below. What it does not: another task's
// done, next, ruling, unstick, reap/sweep --apply, preview, revive, a dispatch, and the operator's
// HW_DONE_ALLOW_BRAIN_LEAK exit. An `env -u HW_TASK -u HW_RUN` gets past THIS check by design (not a sandbox); the brain
// guard still refuses that spelling. The brainer path sets neither variable.
//
// The gate reads the argv the way bash's `$1…` were: `hw next …` is ["next", …], and a dispatch is the project word
// and what follows it. hw-core's own verb names that bash does not have (status-full, outbox-flush, dispatch) are
// turned back into bash's by gateArgv.

import "strings"

// executorAllowed is the `-*|help|status|…` arm of the gate: the words an executor may run.
var executorAllowed = map[string]bool{
	"help": true, "status": true, "receipt": true, "handle": true, "secret": true, "preflight": true, "suite": true, // # MUTATION-ANCHOR: 826-G30
	"human-boundary": true, "review-range": true, "review-delta": true, "output": true, "executor-turn-end": true,
	"chaining-lease-start": true, "chaining-lease-expire": true, "reports": true, "stage": true, "worktrees": true,
	"shadow": true, "shadow-verify": true, "wait": true, "wait-one": true, "wait-monitor": true,
}

// callerIsExecutor is `{ [ -z "$HW_TASK" ] && [ -z "$HW_RUN" ]; } || executor`.
func (a *app) callerIsExecutor() bool {
	return a.getenv("HW_TASK") != "" || a.getenv("HW_RUN") != "" // # MUTATION-ANCHOR: 826-G31
}

// gateArgv is the argv bin/hw's gate saw for a call hw-core answers under `verb`.
func gateArgv(verb string, args []string) []string {
	switch verb {
	case "status-full":
		return append([]string{"status"}, args...)
	case "outbox-flush":
		return append([]string{"outbox", "flush"}, args...)
	case "dispatch": // bin/hw hands hw-core `dispatch "$@"`: the words are the caller's own
		return args
	case "executor-gate":
		return args
	case "picker": // bin/hw's bare `hw`: the gate sees no words, and refuses with the picker's own name
		return nil
	}
	return append([]string{verb}, args...)
}

// executorRefuse is `_executor_refuse`.
func (a *app) executorRefuse(verb, why string) {
	proj, task := a.getenv("HW_PROJECT"), a.getenv("HW_TASK")
	if proj == "" {
		proj = "?"
	}
	if task == "" {
		task = "?"
	}
	a.die("hw " + verb + " is a brainer verb: this shell is executor " + proj + "/" + task + " (HW_TASK/HW_RUN are set), and an executor " + why + ". Report with done-invoker, or ask-invoker \"…\" if your brainer must act; do not work around it.")
}

// executorGate refuses what an executor may not run. argv is bash's "$@".
func (a *app) executorGate(argv []string) {
	if !a.callerIsExecutor() {
		return
	}
	first := ""
	if len(argv) > 0 {
		first = argv[0]
	}
	hasApply := func() bool {
		for _, x := range argv[1:] {
			if x == "--apply" {
				return true
			}
		}
		return false
	}
	switch first {
	case "":
		a.executorRefuse("with no arguments (the picker)", "does not dispatch")
	case "next": // # MUTATION-ANCHOR: 826-G32
		a.executorRefuse("next", "is re-tasked by its brainer, not by itself")
	case "ruling":
		a.executorRefuse("ruling", "does not rule on another run")
	case "unstick":
		a.executorRefuse("unstick", "does not restart another pane")
	case "reap":
		if hasApply() {
			a.executorRefuse("reap --apply", "does not remove worktrees")
		}
	case "sweep":
		if hasApply() {
			a.executorRefuse("sweep --apply", "does not close other runs")
		}
	case "briefs":
		if len(argv) > 1 && (argv[1] == "archive" || argv[1] == "restore" || argv[1] == "unarchive") {
			a.executorRefuse("briefs "+argv[1], "does not move the brain's briefs")
		}
	case "preview":
		a.executorRefuse("preview", "does not keep or release another run's preview")
	case "revive":
		a.executorRefuse("revive", "does not reopen another run's pane")
	case "train":
		a.executorRefuse("train", "does not merge other tasks' branches or advance main")
	case "done":
		if len(argv) >= 3 { // shorter: the verb's own usage error follows
			proj, task := a.getenv("HW_PROJECT"), a.getenv("HW_TASK")
			if proj == "" {
				proj = "?"
			}
			if task == "" {
				task = "?"
			}
			if !(a.c.t.canonProject(argv[1]) == a.c.t.canonProject(proj) && argv[2] == task) { // # MUTATION-ANCHOR: 826-G33
				a.executorRefuse("done "+argv[1]+" "+argv[2], "closes only its own task ("+proj+" "+task+")")
			}
		}
		if a.getenv("HW_DONE_ALLOW_BRAIN_LEAK") == "1" { // # MUTATION-ANCHOR: 826-G34
			a.executorRefuse("done (HW_DONE_ALLOW_BRAIN_LEAK=1)", "does not use the operator's leak-check exit")
		}
	default:
		if strings.HasPrefix(first, "-") || executorAllowed[first] {
			return
		}
		a.executorRefuse(first+" …", "does not dispatch executors") // anything else is `hw <project> <task> …`, a dispatch
	}
}
