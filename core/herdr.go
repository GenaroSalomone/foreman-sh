package main

// The herdr client of hw-core (train T1). Two transports, each for what it is
// good at, and both read-only:
//
//   - the CLI (`herdr workspace list`, `pane list`, `agent list`, `agent read`):
//     the reads bash makes today, run the way bash runs them, so a dead server
//     says the same words. They are processes, so they are issued in parallel.
//   - the socket (newline-delimited JSON, a unix socket, or on Windows the
//     named pipe of the same name): `events.subscribe` on pane.exited, which
//     only the socket exposes. This replaces `bin/herdr-rpc subscribe --drain`
//     for hw, a Python process whose start-up was part of every `hw status`;
//     herdr-rpc stays for the Python consumers.
//
// Nothing here writes to herdr. A subscription is closed when the drain ends.

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

type herdrClient struct {
	getenv func(string) string
	env    []string // the child's environment when the caller has exported variables of its own (nil: the process's)
}

// capture is `_capture herdr <args>` (bin/hw:1501): stdout and stderr in one
// stream, and the status. Trailing newlines are the caller's to strip, as `$()`
// does.
func (h herdrClient) capture(args ...string) (string, int) {
	cmd := exec.Command("herdr", args...)
	cmd.Env = h.env
	if cmd.Env == nil {
		cmd.Env = childEnv()
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), exitStatus(err)
}

// stdout is `herdr <args> 2>/dev/null`: what it printed, and whether it
// succeeded. A caller that reads `... || x='{}'` discards the text on failure.
func (h herdrClient) stdout(args ...string) (string, bool) {
	cmd := exec.Command("herdr", args...)
	cmd.Env = h.env
	if cmd.Env == nil {
		cmd.Env = childEnv()
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	err := cmd.Run()
	return buf.String(), err == nil
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
		return 128 + 9
	}
	return 127
}

// readPane is `herdr agent read <pane> --source visible`, bounded: its text, or
// false when it failed, timed out or could not run.
func (h herdrClient) readPane(pane string, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", "agent", "read", pane, "--source", "visible")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	// the timeout kills herdr, not what herdr started: a child that outlives it still holds the stdout pipe, and
	// without a delay Wait would sit on that pipe until the child exits instead of at the bound
	cmd.WaitDelay = 100 * time.Millisecond
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return buf.String(), true
}

// ── the socket: the pane.exited backlog ──────────────────────────────────────

// exitedEvent is the part of a pane_exited event hw reads.
type exitedEvent struct{ pane, workspace string }

// drainExited connects to HERDR_SOCKET_PATH, subscribes to pane.exited and
// reads the replayed backlog until the connection has been quiet for idle, or
// total has passed — herdr-rpc's `subscribe pane.exited --drain` (bin/herdr-rpc
// cmd_subscribe), with the same two bounds and the same answer to every
// failure: whatever arrived before it is kept, and nothing is an error.
func (h herdrClient) drainExited(idle, total time.Duration) []jmap {
	path := h.getenv("HERDR_SOCKET_PATH")
	if path == "" {
		return nil
	}
	conn, err := dialHerdr(path, total+5*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	chunks := make(chan []byte, 16)
	go func() {
		defer close(chunks)
		buf := make([]byte, 64<<10)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				chunks <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	req, _ := json.Marshal(map[string]any{"id": "1", "method": "events.subscribe",
		"params": map[string]any{"subscriptions": []any{map[string]any{"type": "pane.exited"}}}})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil
	}
	var pending []byte
	// next returns the next whole line, or false at the deadline / hang-up.
	next := func(wait time.Duration) (string, bool, bool) {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		for {
			if i := bytes.IndexByte(pending, '\n'); i >= 0 {
				line := string(pending[:i])
				pending = pending[i+1:]
				return line, true, false
			}
			select {
			case c, ok := <-chunks:
				if !ok {
					return "", false, true
				}
				pending = append(pending, c...)
			case <-timer.C:
				return "", false, false
			}
		}
	}
	// the acknowledgement: the line carrying our id; anything before it is not for us
	ackWait := 5*time.Second + 2*time.Second
	if total < 5*time.Second {
		ackWait = total + 2*time.Second
	}
	deadline := time.Now().Add(ackWait)
	for {
		line, ok, hangup := next(time.Until(deadline))
		if hangup || !ok {
			return nil
		}
		var obj jmap
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &obj) != nil {
			continue
		}
		if id, _ := obj["id"].(string); id == "1" {
			if _, bad := obj["error"]; bad {
				return nil
			}
			break
		}
	}
	var out []jmap
	overall := time.Now().Add(total)
	for {
		left := time.Until(overall)
		if left <= 0 {
			break
		}
		wait := idle
		if left < wait {
			wait = left
		}
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		line, ok, hangup := next(wait)
		if hangup {
			break
		}
		if !ok { // quiet for idle: the backlog is drained
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj jmap
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		if _, isResp := obj["id"]; isResp {
			continue
		}
		out = append(out, obj) // # MUTATION-ANCHOR: 826-G01
	}
	return out
}

// exitedPanes is what status does with that backlog: pane id -> workspace id,
// in the order a pane first appeared, a later event for the same pane updating
// its workspace (a Python dict assigned in a loop).
func exitedPanes(events []jmap) (order []string, ws map[string]any) {
	ws = map[string]any{}
	for _, ev := range events {
		data := pyDict(ev["data"])
		pid := data["pane_id"]
		if !pyTruthy(pid) {
			continue
		}
		key := pyStr(pid)
		if _, seen := ws[key]; !seen {
			order = append(order, key)
		}
		ws[key] = data["workspace_id"]
	}
	return order, ws
}

// runBounded runs a command with a deadline and returns what it printed on
// stdout: false when it could not run or did not finish in time. Its exit
// status is not looked at, as subprocess.run without check=True.
func runBounded(timeout time.Duration, name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", false
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return "", false
		}
	}
	return buf.String(), true
}
