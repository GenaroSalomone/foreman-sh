package invokers

import (
	"io"
	"os"
	"strings"
)

const (
	askMax        = 3
	challengeMax  = 2
	askMaxChars   = 600
	askTTLMs      = "3600000"
	askChunks     = 8
	messageTrimWS = " \t\n"
)

// Ask is bin/ask-invoker.
func Ask(c *Ctx, args []string) int {
	c.Prog = "ask-invoker"
	return c.finish(c.exitCode(c.ask(args)))
}

func (c *Ctx) waitMs() int {
	v := c.get("HW_INVOKER_WAIT_MS")
	if v == "" || !isDigits(v) {
		return 90000
	}
	return atoi10(v)
}

// trimRead is what `read -r -d ”` leaves of a heredoc: the whole text without
// its leading and trailing blanks and newlines.
func trimRead(s string) string { return strings.Trim(s, messageTrimWS) }

func (c *Ctx) ask(args []string) error {
	if c.get("HW_SANDBOX") == "1" {
		sb := c.BinDir + "/hw-sandbox"
		if st, err := os.Stat(sb); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			return die("running in the sandbox, but %s is missing, so nothing can leave it. NOTHING was sent.", sb)
		}
		// exec: the shell is replaced, so its EXIT trap never runs
		rc := c.run(runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}, sb, append([]string{"request", "ask-invoker"}, args...)...)
		c.exitSpoken = true
		return &Die{Code: rc}
	}
	c.loadRoots()
	witness := false
	if st, err := os.Stat(c.BinDir + "/state-witness.sh"); err == nil && !st.IsDir() {
		witness = true
	}

	challenge := false
	if len(args) > 0 {
		switch {
		case args[0] == "--challenge":
			challenge = true
			args = args[1:]
		case args[0] == "--":
			args = args[1:]
		case args[0] == "-":
		case strings.HasPrefix(args[0], "-"):
			return die("unknown option: %s — ask-invoker takes only the question, or --challenge. A flag left in the text becomes part of what the brainer is asked. Usage: ask-invoker \"<one question>\" | ask-invoker --challenge \"<what the brief forbids> :: <what you observed>\" (budget: HW_INVOKER_WAIT_MS in the environment)", args[0])
		}
	}

	var question string
	if len(args) > 0 && args[0] == "-" {
		if len(args) != 1 {
			return die("the question was given on argv AND as '-' for stdin — pass it ONE way, not both")
		}
		if c.StdinTTY {
			return die("'-' reads the question from stdin, but stdin is a terminal — pipe or redirect the text in")
		}
		var b []byte
		var err error
		if c.Stdin != nil {
			b, err = io.ReadAll(c.Stdin)
		}
		if err != nil {
			return die("could not read the question from stdin")
		}
		question = strings.TrimRight(string(b), "\n")
		if question == "" {
			return die("stdin for '-' produced no bytes — an empty question delivered would be worse than this refusal")
		}
	} else {
		if len(args) < 1 {
			return die("usage: ask-invoker \"<one question>\" | ask-invoker - (reads the question from stdin)")
		}
		question = strings.Join(args, " ")
	}

	if c.has("HW_INVOKER_PANE") && c.get("HW_INVOKER_PANE") == "" { // 68-M03
		return die("HW_INVOKER_PANE is set and EMPTY, so this call has been told there is no brainer to reach. REFUSING before the run lock, any counter and any delivery: nothing was sent, no ask was spent and no marker was written. That is deliberate — an explicitly empty address is how a test or a smoke run makes the live channel unreachable, and honouring it is why a probe can no longer reach a real brainer by accident (2026-09-02). To report for real, UNSET it (`unset HW_INVOKER_PANE`) so the run's .hw/<run>/env can supply the address, or export the pane id you mean.")
	}

	c.adoptEnvFile()
	if err := c.runLockAcquire(); err != nil {
		return err
	}

	pane := c.get("HW_INVOKER_PANE")
	if pane == "" {
		return die("no HW_INVOKER_PANE — this pane was not launched by hw, so there is no brainer to ask. Report the blocker to %s in this pane instead.", c.operator())
	}
	if c.get("HERDR_PANE_ID") == "" {
		return die("no HERDR_PANE_ID — not running inside a herdr pane, so the brainer would have nowhere to reply.")
	}
	task := c.or("HW_TASK", "unknown")
	project := c.or("HW_PROJECT", "unknown")

	if challenge {
		if !strings.Contains(question, " :: ") {
			return die("a challenge needs both halves, separated by ' :: ': what the brief forbids, and what you OBSERVED that contradicts it. Example: ask-invoker --challenge \"the brief forbids substituting another host :: the prod host answers /health 200 and brief X documented that path\". Without the evidence the brainer has only your opinion, which is not enough to change a contract.")
		}
		if question[strings.Index(question, " :: ")+4:] == "" {
			return die("the evidence half of the challenge is empty")
		}
	}

	if n := len(question); n > askMaxChars {
		return die("question is %d chars, limit is %d. Ask the question, do not paste the output. Summarise what you saw in one sentence.", n, askMaxChars)
	}
	if strings.Contains(question, "```") {
		return die("no code fences. Describe what you found; the brainer does not need the raw output.")
	}

	stateDir := c.stateDir()
	if err := os.MkdirAll(stateDir, 0o777); err != nil {
		return &Die{Code: 1}
	}
	max := askMax
	var countFile string
	if challenge {
		countFile = stateDir + "/challenge-count" // 09-M01
		max = challengeMax                        // 09-M02
	} else {
		countFile = stateDir + "/ask-count"
	}
	count := "0"
	if b, err := os.ReadFile(countFile); err == nil {
		count = strings.TrimRight(string(b), "\n")
	}
	if !isDigits(count) {
		count = "0"
	}
	if atoi10(count) >= max {
		if challenge {
			return die("already challenged this brief %s times (limit %d). A brief that is wrong in more than %d places is not a brief to negotiate. Your legal exit is: done-invoker --blocked \"the brief needs another contract correction; include the unresolved prohibition and evidence\".", count, max, max) // 09-M03
		}
		return die("already asked the brainer %s times on this task (limit %d). Repeated round-trips mean the brief was wrong. Your legal exit is: done-invoker --blocked \"<the unresolved question and what would unblock it>\". The cap never requires a silent turn.", count, max) // 45-M01
	}
	seq := atoi10(count) + 1
	label := itoa(seq) + "/" + itoa(max)

	kind := "ask"
	if challenge {
		kind = "challenge"
	}
	publishAsk := func(state string) (int, error) {
		tokens, err := tokensJSON(append([]pair{
			{"hw_project", str(project)}, {"hw_task", str(task)}, {"hw_run", str(c.or("HW_RUN", "norun"))},
			{"hw_invoker", str(pane)}, {"ask_seq", str(label)}, {"ask_kind", str(kind)},
			{"ask_state", str(state)}, {"ask_at", str(c.stamp())},
		}, chunkTokens("ask", askChunks, question)...))
		if err != nil {
			return 0, err
		}
		var rc int
		if state == "delivered" {
			rc, err = c.publish(askTTLMs, tokens, "clear")
		} else {
			rc, err = c.publish(askTTLMs, tokens,
				`{"working":"asking brainer (`+label+`)","blocked":"asking brainer (`+label+`)","idle":"asked brainer (`+label+`)"}`)
		}
		c.cockpitKick()
		return rc, err
	}
	pubRC, err := publishAsk("undelivered")
	if err != nil {
		return err
	}
	if pubRC != 0 {
		if pubRC == 3 {
			return die("cannot reach herdr on $HERDR_SOCKET_PATH, so the question could not be recorded and the brainer cannot be prompted. Nothing was spent; report the blocker to %s in this pane.", c.operator())
		}
		c.errf("ask-invoker: warning: question metadata is unverified (herdr-rpc exit %d); continuing to the brainer without a verified record.\n", pubRC)
	}
	pubNote := publicationNote(pubRC)

	envelope := c.envelopePrefix() + ":ask:" + itoa(seq)
	if challenge {
		envelope = c.envelopePrefix() + ":challenge:" + itoa(seq)
	}
	pending := stateDir + "/pending-reply-" + itoa(seq)
	intent := "answer"
	if challenge {
		intent = "ruling" // 41-M04
	}
	replyCmd, err := c.replyCommand(envelope+":reply", intent, pending)
	if err != nil {
		return err
	}
	var msg string
	if challenge {
		msg = trimRead(sprintf(challengeMessage, project, task, c.get("HERDR_PANE_ID"), label, envelope, question, replyCmd))
	} else {
		msg = trimRead(sprintf(askMessage, project, task, c.get("HERDR_PANE_ID"), label, envelope, question, replyCmd, c.operator()))
	}

	t0 := c.Now().Unix()
	logAsk := func(outcome string) {
		elapsed := c.Now().Unix() - t0
		f, err := os.OpenFile(stateDir+"/ask-log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			return
		}
		defer f.Close()
		io.WriteString(f, c.stamp()+"\t"+label+"\t"+outcome+"\t"+pane+"\twaited="+itoa(int(elapsed))+"s\tbudget="+itoa(c.waitMs()/1000)+"s\n")
	}

	route := c.route()
	rc := c.deliver(msg, "")
	if rc == 4 && route != "herdr" {
		c.errf("ask-invoker: the brainer RECEIVED this (it is in its session) but the turn it started did not complete — see the message above. Counting it as delivered, because it was.\n")
		rc = 0
	}
	if rc == 5 {
		logAsk("uncertain")
		activatePendingReply(pending)
		c.cockpitKick()
		os.WriteFile(stateDir+"/delivery-uncertain",
			[]byte("envelope="+envelope+"\nroute="+route+"\nat="+c.stamp()+"\nchannel_send_exit=5\n"), 0o666)
		return &Die{Code: 5, Msg: "delivery UNCERTAIN via " + route + " (channel-send exit 5): envelope " + envelope + " was admitted but its landing is unconfirmed, so the question MAY already be with the brainer. Do NOT re-ask — wait in this pane for the reply, or read the brainer's pane " + pane + ". No ask was spent. " + pubNote}
	}
	if rc != 0 && route != "herdr" {
		logAsk(route + "-failed")
		return die("%s delivery proof failed (exit %d). %s No ask was spent.", route, rc, pubNote)
	}
	if rc == 3 {
		logAsk("unreachable")
		return die("could not wait on %s: either herdr is unreachable on $HERDR_SOCKET_PATH or that pane no longer exists — herdr answers both the same way. Either way the question was NOT delivered and no ask was spent. %s%s Report the blocker to %s in this pane.", pane, pubNote, c.paneSourceNote(), c.operator())
	}
	if rc == 4 {
		logAsk("rejected")
		return die("herdr rejected the wait on %s, so the question was not delivered and no ask was spent. %s Report the blocker to %s in this pane.", pane, pubNote, c.operator())
	}
	if rc == 2 && route == "herdr" {
		logAsk("timeout")
		c.quiet("herdr", "notification", "show", "executor waiting · "+project+":"+task,
			"--body", "brainer still busy after "+itoa(c.waitMs()/1000)+"s — "+pubNote, "--sound", "request")
		return die("the brainer at %s was still working after %ds, so the question was NOT sent. Sending it mid-turn would not lose it — it would get answered somewhere inside the turn the brainer is already in, against context you never saw, which is the whole point of this gate. %s No ask was spent (%s of %d used). Either re-run the same command, or give it a longer budget with a matching Bash timeout: HW_INVOKER_WAIT_MS=540000 ask-invoker \"…\". If it keeps failing, stop and report the blocker to %s in this pane.", pane, c.waitMs()/1000, pubNote, count, max, c.operator())
	}
	if rc != 0 {
		logAsk("herdr-failed")
		return die("Herdr delivery failed (exit %d). %s No ask was spent.", rc, pubNote)
	}

	if err := activatePendingReply(pending); err != nil {
		return die("the brainer received the request, but its pending-reply fact could not be activated at %s", pending)
	}
	if err := os.WriteFile(countFile, []byte(itoa(seq)), 0o666); err != nil {
		return &Die{Code: 1}
	}
	logAsk("delivered")
	if witness {
		k := "ASK"
		if challenge {
			k = "CHALLENGE"
		}
		c.transcriptAppend(stateDir, k, envelope, question)
	}
	publishAsk("delivered")
	c.cockpitKick()
	c.quiet("herdr", "notification", "show", "executor blocked · "+project+":"+task,
		"--body", "asked the brainer ("+label+") via "+route, "--sound", "request")
	if route == "herdr" {
		c.printf(c.Stdout, "asked the brainer (%s) via %s. The receiver ADMITTED envelope %s (admission only, not proof it was read); wait for the reply in this pane.\n", label, route, envelope)
	} else {
		c.printf(c.Stdout, "asked the brainer (%s) via %s. The receiver proved delivery of envelope %s; wait for the reply in this pane.\n", label, route, envelope)
	}
	return nil
}

// activatePendingReply is the python of ask-invoker: the one `state=undelivered`
// row becomes `state=delivered`, through a temporary file and a rename.
func activatePendingReply(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rows := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	idx, n := -1, 0
	for i, r := range rows {
		if r == "state=undelivered" {
			n++
			idx = i
		}
	}
	if n != 1 {
		return os.ErrInvalid
	}
	rows[idx] = "state=delivered"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(rows, "\n")+"\n"), 0o666); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

const challengeMessage = `[CONTRACT CHALLENGE — %s:%s, pane %s, challenge %s]
Envelope: %s. Process this envelope even if your inbox coalesced it
with other messages; each envelope needs its own reply.

This is NOT a question about the work. The executor believes the BRIEF is wrong
and has brought evidence:

%s

Rule on it, in one of exactly two ways, and reply straight to the executor:

  %s

  · "the prohibition stands, because <why the evidence does not overturn it>"
  · "the prohibition is lifted for this task: <what is now permitted>"

Do not answer with a discussion. Until you rule, the executor is holding, which
is the correct thing for it to do and an expensive place for it to sit. If the
brief is wrong, fix the brief too — the next task inherits it.
`

const askMessage = `[executor blocked — %s:%s, pane %s, ask %s]
Envelope: %s. Process this envelope even if your inbox coalesced it
with other messages; each envelope needs its own reply.

%s

Answer it or say it is not yours to answer. Reply straight to the executor:

  %s

Do not open its work in your window. If the answer needs a credential, send the
resolver command, never the value. If it needs %s, say so — they are watching
both panes.
`
