package main

// A failed `bw unlock` is a rejection only when bw said the master password is wrong. Both real alerts of 2026-11-04 and
// 2026-11-05 were false: a node broken by a brew upgrade (dyld) and a CLI/server migration error (HTTP 404). The stubs here
// say exactly what those bw's said; no real vault, no Keychain, every value a fixture.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	hsDyldErr      = "dyld[4242]: Library not loaded: /opt/homebrew/opt/simdjson/lib/libsimdjson.30.dylib\n  Referenced from: <ABCD> /opt/homebrew/Cellar/node/26.13.0_1/bin/node\n  Reason: tried: '/opt/homebrew/opt/simdjson/lib/libsimdjson.30.dylib' (no such file)"
	hsMigrationErr = "key_id_backfill: Failed to post the user key id: ErrorResponse { statusCode: 404, message: 'Not found', status: 404 } [Encrypted Migrator] Error running migrations"
)

// wrapBw puts shell text in front of the bw stub: what it prints or exits with before the stub gets a say.
func (h *hsT) wrapBw(body string) {
	real := filepath.Join(h.stubBin, "bw")
	os.Rename(real, real+"-real")
	os.WriteFile(real, []byte("#!/bin/bash\n"+body+"\nexec "+real+"-real \"$@\"\n"), 0o755)
}

func (h *hsT) autoStatus() string {
	b, _ := os.ReadFile(filepath.Join(h.home, ".cache", "hw", "bw-unlock-status"))
	return string(b)
}

// renewable: a live session that --auto would renew (under half of 12 h left), the Keychain item stored.
func (h *hsT) renewable(left int64) {
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	os.Remove(filepath.Join(h.state, "locked"))
	h.session(left, "TOK-STUB-123")
}

func TestHwSecretClassifyUnlock(t *testing.T) {
	for _, c := range []struct {
		name, text string
		rc         int
		timedOut   bool
		kind       string
	}{
		{"bw's own wrong-password answer", "Invalid master password.", 1, false, hsFailRejected},
		{"the same, with login's longer wording", "Invalid master password. Confirm your email is correct and your account was created on bitwarden.com.", 1, false, hsFailRejected},
		{"the dyld crash of 2026-11-04", hsDyldErr, 134, false, hsFailTool},
		{"a bw that does not start at all", "", -1, false, hsFailTool},
		{"the migration 404 of 2026-11-05", hsMigrationErr, 1, false, hsFailMigration},
		{"a connection error", "FetchError: request to https://api.bitwarden.com failed, reason: getaddrinfo ENOTFOUND api.bitwarden.com", 1, false, hsFailNetwork},
		{"a server error", "ErrorResponse: status: 503", 1, false, hsFailNetwork},
		{"a hang", "", -1, true, hsFailTimeout},
		{"bw failed and said nothing useful", "something unexpected", 1, false, hsFailUnknown},
		{"exit 0 and no token", "", 0, false, hsFailUnknown},
	} {
		kind, reason := hsClassifyUnlock(c.rc, c.text, c.timedOut, 120)
		if kind != c.kind {
			t.Errorf("%s: kind %q, want %q (%s)", c.name, kind, c.kind, reason)
		}
		if kind != hsFailRejected && (strings.Contains(reason, "rejected") || !strings.Contains(reason, "not the password")) {
			t.Errorf("%s: a failure that is not a rejection must say so and never say rejected: %q", c.name, reason)
		}
	}
}

// OLD: the dyld crash and the 404 both said "bw rejected the stored master password" and paged a person. NEW: only the
// wrong password does; the other two keep the live session and report their cause.
func TestHwSecretAutoOnlyAWrongPasswordIsRejected(t *testing.T) {
	// the wrong password, with a live session: a rejection, whatever session is there
	h, _ := alertWorld(t, "", "0")
	h.renewable(3 * 3600)
	h.w.env = append(h.w.env, "BW_STUB_MASTER=CHANGED")
	if rc := h.run("unlock", "--auto"); rc != 1 || !strings.Contains(h.err.String(), "bw rejected the stored master password") || !strings.HasPrefix(h.autoStatus(), "failed ") {
		t.Fatalf("a wrong password: rc=%d err=%q status=%q", rc, h.err, h.autoStatus())
	}

	for _, c := range []struct{ name, body, cause, fix string }{
		{"node broken by a brew upgrade", `case "$1" in status|unlock|--version) printf '%s\n' "` + hsDyldErr + `" >&2; exit 134;; esac`, "bw or node cannot start", "brew reinstall node"},
		{"the migration 404", `[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`, "migration error", "npm i -g @bitwarden/cli"},
	} {
		h, _ := alertWorld(t, "", "0")
		h.renewable(3 * 3600)
		h.wrapBw(c.body)
		if rc := h.run("unlock", "--auto"); rc != 0 {
			t.Fatalf("%s: with a live session the run is a retry, not a failure: rc=%d err=%q", c.name, rc, h.err)
		}
		st := h.autoStatus()
		if !strings.HasPrefix(st, "degraded ") || !strings.Contains(st, c.cause) || !strings.Contains(st, c.fix) || strings.Contains(st, "rejected") {
			t.Fatalf("%s: the status must name the cause and the machine's fix: %q", c.name, st)
		}
		if strings.Contains(h.err.String(), "rejected") || strings.Contains(h.err.String(), "--setup") {
			t.Fatalf("%s: stderr sent a person to the password: %q", c.name, h.err)
		}
		if b, err := os.ReadFile(h.sessionPath()); err != nil || !strings.Contains(string(b), "TOK-STUB-123") {
			t.Fatalf("%s: the live session was not kept: %v", c.name, err)
		}
		if log := h.autoLog(); !strings.Contains(log, "degraded: ") || !strings.Contains(log, "session kept") || strings.Contains(log, "rejected") {
			t.Fatalf("%s: the log must say what happened, as a retry: %s", c.name, log)
		}
		if _, err := os.Stat(filepath.Join(h.home, ".cache", "hw", "vault-alert-state")); err == nil {
			t.Fatalf("%s: a failure that is not the password, with 3 h left, paged: %s", c.name, h.alertLog())
		}
	}
}

func TestHwSecretAutoToolFailureWithNoSessionIsBrokenNotRejected(t *testing.T) {
	h, _ := alertWorld(t, "", "0")
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsDyldErr + `" >&2; exit 134; }`)
	if rc := h.run("unlock", "--auto"); rc != 1 {
		t.Fatalf("nothing to keep: rc=%d", rc)
	}
	st := h.autoStatus()
	if !strings.HasPrefix(st, "broken ") || !strings.Contains(st, "bw or node cannot start") || strings.Contains(st, "rejected") || strings.Contains(st, "--setup") {
		t.Fatalf("status: %q", st)
	}
	// ensure, which a dispatch drives, does not send a person to the password either
	if rc := h.run("ensure"); rc != 1 || strings.Contains(h.err.String(), "--setup") || !strings.Contains(h.err.String(), "Not the master password") {
		t.Fatalf("ensure: rc=%d %q", rc, h.err)
	}
}

func TestHwSecretAutoUnlockTimeoutIsATimeoutNotARejection(t *testing.T) {
	h, _ := alertWorld(t, "", "0")
	h.renewable(3 * 3600)
	h.wrapBw(`[ "$1" = unlock ] && exec sleep 30`)
	h.w.env = append(h.w.env, "HW_BW_UNLOCK_TIMEOUT=1")
	if rc := h.run("unlock", "--auto"); rc != 0 {
		t.Fatalf("a hung bw with a live session: rc=%d %q", rc, h.err)
	}
	if st := h.autoStatus(); !strings.HasPrefix(st, "degraded ") || !strings.Contains(st, "did not answer within 1 s") || strings.Contains(st, "rejected") {
		t.Fatalf("status: %q", st)
	}
}

// The alert goes to the setup brainer only for a real rejection, or for a session that is nearly over and keeps failing to renew.
func TestHwSecretAlertOnlyForARejectionOrANearlyOverSession(t *testing.T) {
	pane := func(h *hsT) string {
		return "HERDR_PANES=" + panesJSON(h, `{"pane_id":"w7G:p1","label":"brain","cwd":"`+filepath.Join(h.home, "brain", "setup")+`"}`)
	}
	// a tool failure with 3 h left: no page
	h, rec := alertWorld(t, "", "0")
	h.w.env = append(h.w.env, pane(h))
	h.renewable(3 * 3600)
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`)
	h.run("unlock", "--auto")
	if _, err := os.Stat(rec); err == nil {
		t.Fatalf("a migration error with 3 h of session left paged the brainer")
	}
	// the same failure with 30 min left: a page, naming the machine's fix and not asking for the password
	h, rec = alertWorld(t, "", "0")
	h.w.env = append(h.w.env, pane(h))
	h.renewable(30 * 60)
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`)
	h.run("unlock", "--auto")
	sent, _ := os.ReadFile(rec)
	if !strings.Contains(string(sent), "the renewal keeps failing") || !strings.Contains(string(sent), "migration error") || !strings.Contains(string(sent), "machine's fix") || strings.Contains(string(sent), "secret-unlock") {
		t.Fatalf("the near-expiry alert for a tool failure: %q", sent)
	}
	// a tool failure with 90 min left (inside the old 2 h window): still no page, the retry is the answer
	h, rec = alertWorld(t, "", "0")
	h.w.env = append(h.w.env, pane(h))
	h.renewable(90 * 60)
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`)
	h.run("unlock", "--auto")
	if _, err := os.Stat(rec); err == nil {
		t.Fatalf("a tool failure with 90 min left paged")
	}
	// no session and a broken bw: nothing is usable, so it pages, with the machine's fix
	h, rec = alertWorld(t, "", "0")
	h.w.env = append(h.w.env, pane(h))
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsDyldErr + `" >&2; exit 134; }`)
	h.run("unlock", "--auto")
	sent, _ = os.ReadFile(rec)
	if !strings.Contains(string(sent), "no live session") || !strings.Contains(string(sent), "brew reinstall node") || strings.Contains(string(sent), "secret-unlock") {
		t.Fatalf("the alert for a broken bw with no session: %q", sent)
	}
	// a real rejection still pages a person
	h, rec = alertWorld(t, "", "0")
	h.w.env = append(h.w.env, pane(h), "BW_STUB_MASTER=CHANGED")
	h.run("unlock", "--auto")
	sent, _ = os.ReadFile(rec)
	if !strings.Contains(string(sent), "bw rejected the stored master password") || !strings.Contains(string(sent), "A person, once") {
		t.Fatalf("a rejection must still page a person: %q", sent)
	}
}

// An open rejection alert is not "recovered" by a later run that merely retries under a live session.
func TestHwSecretAlertStaysOpenThroughAToolFailure(t *testing.T) {
	h, _ := alertWorld(t, "", "0")
	h.w.env = append(h.w.env, "HERDR_PANES="+panesJSON(h, `{"pane_id":"w7G:p1","label":"brain","cwd":"`+filepath.Join(h.home, "brain", "setup")+`"}`), "BW_STUB_MASTER=CHANGED")
	h.renewable(3 * 3600)
	h.run("unlock", "--auto") // a real rejection: the alert opens
	state := filepath.Join(h.home, ".cache", "hw", "vault-alert-state")
	if st, _ := os.ReadFile(state); !strings.HasPrefix(string(st), "failed ") {
		t.Fatalf("no rejection alert to start from: %q", st)
	}
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`)
	h.run("unlock", "--auto") // the next run: a migration error, the 3 h session kept
	if _, err := os.Stat(state); err != nil || strings.Contains(h.alertLog(), "recovered") {
		t.Fatalf("a retry under a live session closed the rejection alert as recovered: %v\n%s", err, h.alertLog())
	}
}

// `hw secret status` runs a no-vault `bw --version` and reports a bw that cannot start.
func TestHwSecretStatusReportsABrokenBw(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	h.wrapBw(`printf '%s\n' "$*" >> "$BW_STUB_STATE/calls"; case "$1" in status|unlock|--version) printf '%s\n' "` + hsDyldErr + `" >&2; exit 134;; esac`)
	if rc := h.run("status"); rc != 1 || !strings.Contains(h.out.String(), "bw cannot start") || !strings.Contains(h.out.String(), "brew reinstall node") || strings.Contains(h.out.String(), "unlocked until") {
		t.Fatalf("a broken bw: rc=%d out=%q", rc, h.out)
	}
	if !strings.Contains(h.out.String(), "the session is kept") {
		t.Fatalf("status must say the session survives: %q", h.out)
	}
	if b, err := os.ReadFile(h.sessionPath()); err != nil || !strings.Contains(string(b), "TOK-STUB-123") {
		t.Fatalf("status deleted the live session of a bw that merely cannot start: %v", err)
	}
	if strings.Contains(h.bwCalls(), "status") || !strings.Contains(h.bwCalls(), "--version") {
		t.Fatalf("status must probe with --version and open no vault: %q", h.bwCalls())
	}
}

// --setup must not delete a just-stored password because bw could not run: bw never judged it.
func TestHwSecretSetupKeepsThePasswordWhenBwCannotRun(t *testing.T) {
	h := newHsT(t)
	h.wrapBw(`[ "$1" = unlock ] && { printf '%s\n' "` + hsMigrationErr + `" >&2; exit 1; }`)
	if rc := h.setup("MASTER-STUB-PW", "MASTER-STUB-PW"); rc != 1 {
		t.Fatalf("rc=%d %q", rc, h.err)
	}
	if _, ok := h.item(); !ok {
		t.Fatalf("a migration error deleted the Keychain item: bw never said the password was wrong")
	}
	if !strings.Contains(h.err.String(), "NOT verified") || strings.Contains(h.err.String(), "did not accept that password") {
		t.Fatalf("stderr: %q", h.err)
	}
}
