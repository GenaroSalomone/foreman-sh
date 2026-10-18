package main

// Train T6: the verbs that were bash-only when foreman-sh 0.4.0 made `hw` Go-only — train, suite, unstick,
// ledger, briefs, and the two that bin/hw only exec'd (cockpit-state, secret). Each is a port of
// the bash it names, with the same stdout, stderr and exit status, in its own t6_<verb>.go. `--version` is T6's
// too (bin/hw answered it before the lane table loaded).

// t6Verbs are the verbs of this train.
var t6Verbs = map[string]bool{
	"train": true, "suite": true, "unstick": true, "ledger": true, "briefs": true,
	"cockpit-state": true, "secret": true, "-V": true, "--version": true,
}

func isT6Verb(v string) bool { return t6Verbs[v] }

func (a *app) cmdT6(verb string) int {
	switch verb {
	case "train":
		return a.cmdTrain()
	case "suite":
		return a.cmdSuite()
	case "unstick":
		return a.cmdUnstick()
	case "ledger":
		return a.cmdLedger()
	case "briefs":
		return a.cmdBriefs()
	case "cockpit-state":
		return a.cmdExecHelper("cockpit-state")
	case "secret":
		return a.cmdExecHelper("hw-secret")
	}
	return 64
}
