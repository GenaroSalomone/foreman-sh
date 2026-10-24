#!/usr/bin/env bash
# A NON-ZERO EXIT MUST NOT LEAVE SILENTLY WHEN STDOUT IS PIPED.
#
# THE INCIDENT, twice, same tool, same shape. A brainer ran
#
#     channel-send … | tail -4
#
# read `exit 0`, and reported the send as delivered. Nothing had been delivered.
# The `0` was TAIL's: after a pipeline the shell's `$?` is the LAST command's
# status, and tail exits 0 whenever it reads a byte. It had happened once before
# with the same tool and the same pipe, and it cost a report both times.
#
# AND THE TOOL INVITES IT. channel-send's output is long and explanatory on
# purpose, which is exactly what makes `| tail -4` the obvious thing to do. So
# the rule "remember PIPESTATUS" is one an agent has to hold in mind on the busy
# day, which is the only day it matters. The fix is that the tool says so itself.
#
# WHAT THIS SUBJECT PROVES, and it is deliberately the caller's view rather than
# the implementation's: run the tool in the shape that fooled the brainer, on a
# call that provably delivered nothing, and assert that a reader of that output
# CANNOT conclude what the brainer concluded. `shell $?` staying 0 is not the
# bug being fixed — that is how pipelines work and it cannot be changed. The bug
# is that nothing said so.
#
# Run it alone while working on this subject:
#
#     bash setup/tests/66-exit-code-survives-a-pipe.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
need_invokers   # C09/C10 run done-invoker and ask-invoker from a copy of bin/, which has no core/dist beside it: they are Go (go-060 S2)

CLAIMS=0
claim() { CLAIMS=$((CLAIMS + 1)); pass "$1"; }

# A HERMETIC HARNESS, DRIVEN DIRECTLY — never the live channel. Exercising
# ask-invoker or done-invoker against the real brainer pane spends an ask, holds
# this pane until a human answers, and is the one thing a task that touches
# these binaries must not do. Same construction as 05-channel-send.sh: copy the
# binary and its siblings into $TMP, stub herdr and herdr-rpc beside it, and
# call it with arguments that cannot reach anything real.
H="$TMP/h"; mkdir -p "$H/bin"
cp "$ROOT/bin/channel-send" "$ROOT/bin/done-invoker" "$ROOT/bin/ask-invoker" \
   "$ROOT/bin/invoker-common.sh" "$ROOT/bin/state-witness.sh" "$H/bin/" 2>/dev/null || true
cp "$TMP/bin/herdr" "$H/bin/herdr"
cat > "$H/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
exit "${STUB_RPC_RC:-4}"
STUB
chmod +x "$H/bin/herdr-rpc"

# THE SHAPE THAT FOOLED THE BRAINER, reproduced exactly: stdout through `tail
# -4`, stderr left alone because `cmd | tail` redirects only stdout. What the
# caller SEES is stderr plus the tail of stdout, and what the shell REPORTS is
# tail's status. Both are captured, separately, because the whole defect is that
# the two disagree.
#
# `nosuch:pane` against a stubbed herdr-rpc that exits 4 is a call that provably
# delivers nothing, and it returns in milliseconds.
# PIPEFAIL IS OFF INSIDE THE FIXTURE, AND THAT IS THE POINT. This suite runs
# under `set -o pipefail`, which makes `$?` after a pipeline report the first
# failing stage — so under pipefail the brainer's bug does not reproduce at all.
# A brainer's shell is an interactive bash or zsh, where pipefail is OFF by
# default and `$?` is the LAST stage's status. Reproducing the incident therefore
# means reproducing the caller's shell options, not this file's.
piped() { # <binary> [args...]  → prints "RC=<what the caller's shell reports>" then its view
  local bin="$1"; shift
  { PATH="$H/bin:$PATH" HW_INVOKER_WAIT_MS=1000 bash -c '
      set +o pipefail
      "$0" "$@" | tail -4
      printf "RC=%s\n" "$?"' "$H/bin/$bin" "$@"; } 2>&1
}
# The same call with stdout NOT piped, so the shell really does report the
# tool's own status. This is the control: it is what an interactive caller gets.
direct() { # <binary> [args...]  → prints "RC=<the tool's own status>"
  local bin="$1" rc=0; shift
  PATH="$H/bin:$PATH" HW_INVOKER_WAIT_MS=1000 "$H/bin/$bin" "$@" >/dev/null 2>&1 || rc=$?
  printf 'RC=%s\n' "$rc"
}

# ── C01. the shell reports 0 for a call that delivered nothing ───────────────
#
# Established first because every claim below depends on it. If this ever stops
# being true the rest of this file is testing a shape that no longer exists.
OUT="$(piped channel-send herdr nosuch:pane - probe)"
case "$OUT" in
  *'RC=0'*) claim "C01 a piped channel-send that delivered nothing still reports 0 to the shell" ;;
  *) fail "C01: the pipeline did not report 0 — this subject's premise is gone: $(printf '%s' "$OUT" | tail -3)" ;;
esac
case "$(direct channel-send herdr nosuch:pane - probe)" in
  RC=1) claim "C02 unpiped, the same call reports its own exit 1" ;;
  *) fail "C02: unpiped exit was not 1: $(direct channel-send herdr nosuch:pane - probe)" ;;
esac

# ── C03. the tool says the number is not its own, and says what its own is ──
case "$OUT" in
  *'MY EXIT CODE IS 1'*) claim "C03 the piped call states its own exit code" ;;
  *) fail "C03: no exit code in the caller's view: $(printf '%s' "$OUT" | tail -3)" ;;
esac
case "$OUT" in
  *'NOT DELIVERED'*) claim "C04 it states the delivery fact, not only the number" ;;
  *) fail "C04: the caller's view never says nothing was delivered: $(printf '%s' "$OUT" | tail -3)" ;;
esac
case "$OUT" in
  *"NOT mine"*) claim "C05 it says the number the shell will report is not its own" ;;
  *) fail "C05: nothing warned that the shell's status is someone else's: $(printf '%s' "$OUT" | tail -3)" ;;
esac
case "$OUT" in
  *'PIPESTATUS'*) claim "C06 it names how to read the real status" ;;
  *) fail "C06: no PIPESTATUS in the caller's view: $(printf '%s' "$OUT" | tail -3)" ;;
esac

# ── C0A. the guard covers exits that happen BEFORE the delivery machinery ───
#
# A usage error exits from the argument loop at the top of the file, long before
# the delivery lock and its own EXIT trap exist. Two separate guard installations
# are what cover both halves (66-G01 and 66-G02 hold them in Go, see MUTANTS below).
USAGE="$(piped channel-send)"
case "$USAGE" in
  *'MY EXIT CODE IS 1'*) claim "C0A a usage error through a pipe is disowned too, not only a failed delivery" ;;
  *) fail "C0A: a pre-lock exit went silent through a pipe: $(printf '%s' "$USAGE" | tail -3)" ;;
esac

# ── C07. it fits inside a `tail -4` window ──────────────────────────────────
#
# NOT COSMETIC. `2>&1 | tail -4` is the other common shape, and there the
# warning competes with everything else for four lines. A banner longer than the
# window pushes itself out of it.
banner_lines="$(PATH="$H/bin:$PATH" HW_INVOKER_WAIT_MS=1000 "$H/bin/channel-send" herdr nosuch:pane - probe 2>&1 >/dev/null | grep -c 'MY EXIT CODE IS\|NOT mine' || true)"
[ "$banner_lines" = 2 ] || fail "C07: the banner is $banner_lines lines, not 2 — it must fit a tail -4 window beside the diagnosis"
claim "C07 the banner is two lines, so it survives a 4-line tail window"

# ── C08. an interactive caller is not warned ────────────────────────────────
#
# The guard's whole premise is that stdout is not a terminal. On a real tty the
# shell reports the tool's own status, the warning would be false, and a warning
# that fires when it does not apply is a warning that gets muted. Driven through
# a pty because `[ -t 1 ]` cannot be faked any other way.
# NOT ON NATIVE WINDOWS: Windows Python has no pty (nor termios, which pty
# needs), and Git Bash ships no `script`, so there is no terminal to hand the
# control. Named and skipped there; everywhere else it runs.
case "${OSTYPE:-}" in msys*|cygwin*)
  claim "C08 SKIPPED on native Windows — no pty to drive a real terminal (Windows Python has no pty/termios, Git Bash no script)"
  tty_out=SKIPPED ;;
*)
tty_out="$(python3 - "$H/bin" <<'PY' 2>&1 || true
import os, pty, sys
bindir = sys.argv[1]
env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"])
chunks = []
def read(fd):
    data = os.read(fd, 1024)
    chunks.append(data)
    return b""
pid, fd = pty.fork()
if pid == 0:
    os.execvpe("bash", ["bash", "-c",
        # NO REDIRECTION OF STDOUT. `>/dev/null` would make `[ -t 1 ]` false
        # and turn this control into a second copy of the piped case — which is
        # exactly the mistake this comment exists to stop from coming back.
        "%s/channel-send; printf 'RC=%%s\\n' \"$?\"" % bindir], env)
try:
    while True:
        data = os.read(fd, 1024)
        if not data:
            break
        chunks.append(data)
except OSError:
    pass
os.waitpid(pid, 0)
sys.stdout.write(b"".join(chunks).decode("utf-8", "replace"))
PY
)"
;;
esac
case "$tty_out" in
  SKIPPED) ;;
  *'MY EXIT CODE IS'*) fail "C08: the banner fired on a real terminal, where the shell already reports the tool's own status: $tty_out" ;;
  *'RC=1'*) claim "C08 on a real terminal it stays silent, and the shell reports 1 anyway" ;;
  *) fail "C08: the pty control did not run — got: $(printf '%s' "$tty_out" | tr '\n' ' ' | tail -c 200)" ;;
esac

# ── C09/C10. both invokers share the defect, so both carry the fix ──────────
#
# Driven with no HW_INVOKER_PANE and a $PWD that has no .hw/<run>/env to walk up
# to, so each refuses before touching anything: exit 1, before any delivery is
# attempted. `env -u` rather than `VAR=` because these read the value's presence.
inv_piped() { # <binary> <arg>
  ( cd "$H" && env -u HW_INVOKER_PANE -u HW_WORKDIR -u HW_RUN -u HW_PROJECT -u HW_TASK \
      HOME="$H" PATH="$H/bin:$PATH" bash -c '
        set +o pipefail
        "$0" "$1" | tail -4
        printf "RC=%s\n" "$?"' "$H/bin/$1" "$2" 2>&1 )
}
for pair in "done-invoker:report" "ask-invoker:question"; do
  bin="${pair%%:*}"; noun="${pair##*:}"
  out="$(inv_piped "$bin" "a $noun with nowhere to go" || true)"
  case "$out" in
    *'RC=0'*)
      # MATCHED WITH THE BINARY'S OWN PREFIX. Both invokers call channel-send,
      # which carries the same guard, so an unqualified match would let the
      # child's banner stand in for the parent's.
      case "$out" in
        *"$bin: ══ MY EXIT CODE IS 1"*)
          case "$out" in
            *"$bin: my stdout is NOT a terminal"*) claim "C0x $bin also states its own exit code and disowns the shell's" ;;
            *) fail "C0x: $bin printed a code but did not disown the shell's: $(printf '%s' "$out" | tail -2)" ;;
          esac ;;
        *) fail "C0x: $bin exited non-zero through a pipe in silence: $(printf '%s' "$out" | tail -3)" ;;
      esac ;;
    *) fail "C0x: $bin's premise is gone — the pipeline did not report 0: $(printf '%s' "$out" | tail -3)" ;;
  esac
done

# ── MUTANTS ─────────────────────────────────────────────────────────────────
#
# THE SIX BASH MUTANTS WENT WITH THE BASH (go-060 S5: bin/channel-send, bin/done-invoker and bin/ask-invoker are shims). Each one
# edited a line of a bash body and was killed by text only the mutant printed. What each held is held in Go, by anchors killed in process:
#   M01 the early guard removed (a usage error goes silent)           66-G01  core/channel_send.go  TestChannelSendExitGuard
#   M02 a later trap replaces the guard                               66-G02  core/channel_send.go  TestChannelSendLock
#   M03 `$?` read after cleanup, so the guard sees 0                  66-G03  core/channel_send.go  TestChannelSendSignalOutcomes
#   M04 the tty test inverted                                         66-G04  core/channel_send.go  TestChannelSendExitGuard
#   M05 the disowning sentence dropped                                66-G05  core/channel_send.go  TestChannelSendExitGuard
#   M06 done-invoker's guard re-arm skipped                           66-M06  core/invokers/ctx.go  TestDiffAsk/delivered_herdr (core/invokers/mutants_test.go)
# 66-G01..G05 are rows of setup/tests/contract-hw/mutants-cs, killed by setup/tests/1016; 66-M06 is a row of core/invokers/mutants_test.go.
# C01-C0A and C07-C10 above still run the caller's view of all three binaries through their shims.

[ "$CLAIMS" -ge 1 ] || fail "no behaviour claims were made"
printf 'coverage - %s behaviour claims, 0 dedicated production mutants (the 6 bash mutants are retired; 66-G01..G05 are killed by 1016, 66-M06 by core/invokers)\n' "$CLAIMS"
