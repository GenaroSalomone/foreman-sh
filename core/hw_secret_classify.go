package main

// What a failed `bw unlock` means. Only bw's own answer to a wrong master password is a rejection; everything else (a node that
// will not start, a server or network error, a CLI migration error, a hang) says nothing about the password, and must never
// send a person to retype it.
//
// The rejection text is MEASURED, not guessed: @bitwarden/cli 2026.10.1, build/bw.js, UnlockCommand answers
// `Response.error(e.message)`, and the message of a failed master-password check is the locale string `invalidMasterPassword`
// = "Invalid master password." (build/locales/en/messages.json:77; thrown at build/bw.js:66584 and :66599).
// `bw login` words the same rejection "Invalid master password. Confirm your email is correct …" (invalidMasterPasswordConfirmEmailAndHost),
// which starts with the same words, so one match covers both.

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	hsFailRejected  = "rejected"  // bw said the password is wrong: a person's problem
	hsFailTool      = "tool"      // bw or node cannot start: the machine's
	hsFailMigration = "migration" // bw hit a CLI/server migration error
	hsFailNetwork   = "network"   // the server or the network
	hsFailTimeout   = "timeout"   // bw did not answer in time
	hsFailUnknown   = "unknown"   // bw failed and did not say the password was wrong
)

// hsToolFix is the machine's repair, for a message that names a tool failure.
const hsToolFix = "brew reinstall node, then npm i -g @bitwarden/cli if bw still will not start"

var (
	hsRejectedRe  = regexp.MustCompile(`(?i)invalid master password`)
	hsToolRe      = regexp.MustCompile(`(?i)dyld|library not loaded|image not found|bad cpu type|illegal instruction|symbol not found|segmentation fault|command not found|env: node|cannot find module|no such file or directory|node:internal|syntaxerror|node: bad option`)
	hsMigrationRe = regexp.MustCompile(`(?i)migrat|key_id_backfill`)
	hsNetworkRe   = regexp.MustCompile(`(?i)enotfound|econnrefused|econnreset|etimedout|eai_again|ehostunreach|enetunreach|fetch failed|getaddrinfo|network|socket hang up|certificate|status:?\s*[45][0-9][0-9]|bad gateway|service unavailable|too many requests|rate limit`)
)

// hsClassifyUnlock: rc is bw's exit status (-1: it did not start or was killed), text its stderr and stdout (not yet redacted;
// the classifier only reads it), timedOut that this process killed it at the deadline. The reason is what status, log and
// alert say; for every kind but rejected it names the cause and says the password was not at fault.
func hsClassifyUnlock(rc int, text string, timedOut bool, timeoutSecs int) (kind, reason string) {
	switch {
	case hsRejectedRe.MatchString(text):
		return hsFailRejected, "bw rejected the stored master password"
	case timedOut:
		return hsFailTimeout, "bw unlock did not answer within " + strconv.Itoa(timeoutSecs) + " s: not the password, retried on the next run"
	case hsMigrationRe.MatchString(text):
		return hsFailMigration, "bw hit a CLI/server migration error: not the password; update bw (npm i -g @bitwarden/cli), it retries on the next run"
	case hsToolRe.MatchString(text) || rc == -1 || rc == 126 || rc == 127:
		return hsFailTool, "bw or node cannot start: not the password; the machine's fix is " + hsToolFix
	case hsNetworkRe.MatchString(text):
		return hsFailNetwork, "bw could not reach the Bitwarden server: not the password, retried on the next run"
	}
	return hsFailUnknown, "bw unlock failed without saying the password was wrong: not the password, retried on the next run"
}

// hsToolReason is the one reason `bw --version` failing gets: a bw that cannot print its version cannot unlock anything.
func hsToolReason(detail string) string {
	r := "bw cannot start: not the master password; the machine's fix is " + hsToolFix
	if strings.TrimSpace(detail) != "" {
		r += " (bw said: " + detail + ")"
	}
	return r
}
