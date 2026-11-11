package main

// Is the engram serve on the port this machine's store? — what bin/engram-serve.sh answered for hw's launch, in Go.
//
// WHY AN ADDRESS IS NOT AN IDENTITY (measured 2026-09-28): a serve started under a sandbox HOME took the default port and
// every `POST /sessions` hw made landed in the sandbox store and answered "created", while the executor's `engram mcp`
// opened ~/.engram and answered `unknown_session`. So the port is checked for WHO IT IS: `engram serve` reports `instance_id`
// on GET /health, the `.instance-id` file of the data dir it opened. The data dir an `engram mcp` opens is ENGRAM_DATA_DIR,
// else $HOME/.engram. If the two ids differ, the serve is someone else's store and nothing is written to it. Nothing here
// starts a serve: one started from here would carry this process's HOME, the very defect above.
//
// The brain's launch (brain_helpers.go), the turn end (t3_turnend.go) and the invokers (core/invokers/engram.go) read the same
// serve the same way; the first two share this, and the invokers are a package of their own.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"hwcore/internal/engramhealth"
)

// engramServe is an environment that can ask: the variables the serve's address and store come from, and HOME.
type engramServe struct {
	getenv func(string) string
	home   string
}

// url is engram_serve_url: HW_ENGRAM_URL, else the port ENGRAM_PORT (default 7437) on this machine.
func (e engramServe) url() string {
	if u := e.getenv("HW_ENGRAM_URL"); u != "" {
		return u
	}
	port := e.getenv("ENGRAM_PORT")
	if port == "" {
		port = "7437"
	}
	return "http://127.0.0.1:" + port
}

// storeDir is engram_store_dir: the store an `engram mcp` started from this environment opens (a blank value keeps the default).
func (e engramServe) storeDir() string {
	if d := e.getenv("ENGRAM_DATA_DIR"); strings.TrimSpace(d) != "" {
		return d
	}
	return e.home + "/.engram"
}

// verdict is engram_serve_verdict: one line, and 0 the serve at u is this environment's store (the instance ids match), 2 a
// serve answers and it is ANOTHER store (never write to it), 3 unverifiable (nothing answers, or no instance id on either side).
func (e engramServe) verdict(u string) (string, int) {
	dir := e.storeDir()
	want := ""
	if data, err := os.ReadFile(dir + "/.instance-id"); err == nil {
		want = strings.Join(strings.Fields(string(data)), "")
	}
	probe := engramhealth.Get(u+"/health", engramhealth.Budget(e.getenv("HW_ENGRAM_HEALTH_TIMEOUT")))
	if !probe.OK {
		return engramhealth.Refusal(u, probe), 3
	}
	got := bjRawOr(probe.Body, "instance_id")
	switch {
	case want == "":
		return "cannot prove the serve at " + u + " is this store: " + dir + "/.instance-id is missing", 3 // # MUTATION-ANCHOR: bpr3-want
	case got == "":
		return "cannot prove the serve at " + u + " is this store: its /health names no instance_id", 3
	case got != want:
		return "FOREIGN engram serve at " + u + ": instance " + got + ", but this store (" + dir + ") is instance " + want + " — a serve started under another HOME or ENGRAM_DATA_DIR holds the port", 2 // # MUTATION-ANCHOR: bpr3-foreign
	}
	return "engram serve at " + u + " is this store (" + dir + ", instance " + got + ")", 0
}

// sessionPost is engram_session_post: create or renew (a renewal rewrites the 30-minute runtime lease) the session on a serve
// the caller has already verified. ok only when engram echoes the id; out is what came back when it did not echo it, and ""
// when nothing did (`curl -sf` prints nothing for an HTTP error).
func (e engramServe) sessionPost(id, project, dir, u string) (out string, ok bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// the keys sorted, as the turn end's renewal always sent them (the bash registration sent id first): engram reads a JSON object
	// by key, and one body for the launch, `brain` and the renewal is one thing to pin
	enc.Encode(struct {
		Directory string `json:"directory"`
		ID        string `json:"id"`
		Project   string `json:"project"`
	}{dir, id, project})
	req, err := http.NewRequest("POST", u+"/sessions", strings.NewReader(strings.TrimRight(buf.String(), "\n")))
	if err != nil {
		return "", false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	body.ReadFrom(resp.Body)
	if resp.StatusCode >= 400 {
		return "", false
	}
	out = strings.TrimRight(body.String(), "\n") // `out="$(curl …)"` dropped trailing newlines; the receipt line is that text
	return out, bjTestEq(body.String(), id, "id") // # MUTATION-ANCHOR: bpr3-echo
}
