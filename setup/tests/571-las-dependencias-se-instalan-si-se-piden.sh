#!/usr/bin/env bash
# install.sh --with-recommended installs what is missing, with Homebrew, only when asked
#
# WHAT THIS HOLDS. Before: install.sh named every missing dependency and its
# install command, and the person ran each by hand (measured 2026-10-01 on a bare
# macOS PATH: herdr and Claude Code by URL only; rg, fd and sd, which hw calls,
# never named on macOS). Claims, with `brew` a stub that logs each call and
# "installs" by creating the command, on a PATH built here from single links:
#
#   1. without the flag nothing calls brew: neither --check nor a real run;
#   2. --with-recommended --check prints every command it would run, runs none
#      and writes nothing under HOME;
#   3. --with-recommended installs each missing package with its own
#      `brew install`, in order, required before recommended, and prints the
#      command BEFORE brew runs; Claude Code is the cask; what is present is not
#      reinstalled; exit 0 when nothing is left;
#   4. one failing install does not stop the rest, and is named; exit 1;
#   5. a jq older than 1.7 counts as missing;
#   6. no brew on PATH: it says so, names https://brew.sh, installs nothing, exit 1;
#   7. --check, with brew present and tools missing, names --with-recommended;
#   8. a brew that reads its stdin does not eat the packages still to come.
#
# Point it at another tree to watch it fail there:
#     SUBJECT_ROOT=/path/to/old/tree bash setup/tests/571-las-dependencias-se-instalan-si-se-piden.sh
#
# Run alone:  bash setup/tests/571-las-dependencias-se-instalan-si-se-piden.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR OPENCODE_CONFIG_DIR OPENCODE_EXECUTOR_CONFIG
SUBJECT="${SUBJECT_ROOT:-$ROOT}"
export ENGRAM_PORT=9 ENGRAM_DATA_DIR="$TMP/engram"   # never 7437

# ── a PATH of single links: the base tools, and nothing foreman-sh depends on ──
SYS="$TMP/sys"; mkdir -p "$SYS"
for t in bash sh env sed awk cat dirname basename mkdir rm chmod tr head tail wc \
         mktemp readlink grep sort cut ls printf id touch cp mv ln date rsync md5 md5sum; do
  p="$(command -v "$t" 2>/dev/null || true)"; case "$p" in /*) ln -sf "$p" "$SYS/$t" ;; esac
done
# The claims are the macOS list (Claude Code as the cask); on a Linux runner too.
printf '#!/bin/sh\n{ [ "$1" = -s ] || [ -z "$1" ]; } && echo Darwin || exec %s "$@"\n' "$(command -v uname)" > "$SYS/uname"; chmod +x "$SYS/uname"
link() { local p; p="$(command -v "$1")" || fail "this test needs $1 on PATH"; ln -sf "$p" "$2/$1"; }
link git "$SYS"; link python3 "$SYS"   # what Homebrew does not install here; present, so "ok"

# brew: logs every call and "installs" a command into $GOT, which is on PATH.
# BREW_FAIL names a package whose install fails.
GOT="$TMP/got"; BREWBIN="$TMP/brewbin"; mkdir -p "$GOT" "$BREWBIN"
cat > "$BREWBIN/brew" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$TMP/brew.log"
[ "\$1" = --prefix ] && { echo "$TMP/prefix"; exit 0; }
[ "\$1" = install ] || exit 0
shift; [ "\$1" = --cask ] && shift
pkg="\$1"; echo "BREW-RAN \$pkg"
[ -z "\${BREW_EATS:-}" ] || cat > /dev/null
[ "\$pkg" != "\${BREW_FAIL:-}" ] || { echo "Error: \$pkg failed" >&2; exit 1; }
case "\$pkg" in ripgrep) cmd=rg ;; claude-code) cmd=claude ;; */engram) cmd=engram ;; *) cmd="\$pkg" ;; esac
if [ "\$cmd" = jq ]; then printf '#!/bin/sh\necho jq-1.7.1\n' > "$GOT/jq"
else printf '#!/bin/sh\nexit 0\n' > "$GOT/\$cmd"; fi
chmod +x "$GOT/\$cmd"
STUB
chmod +x "$BREWBIN/brew"
reset() { rm -rf "$GOT" "$TMP/brew.log"; mkdir -p "$GOT"; }
# old jq: a PATH entry before $GOT, so brew's jq (in $GOT) has to win by being newer — see claim 5
OLDJQ="$TMP/oldjq"; mkdir -p "$OLDJQ"; printf '#!/bin/sh\necho jq-1.6\n' > "$OLDJQ/jq"; chmod +x "$OLDJQ/jq"
NEWJQ="$TMP/newjq"; mkdir -p "$NEWJQ"; printf '#!/bin/sh\necho jq-1.7.1\n' > "$NEWJQ/jq"; chmod +x "$NEWJQ/jq"

run() {  # <PATH> <args…> — install.sh from SUBJECT, stdout+stderr in $out, exit in $rc
  local p="$1"; shift
  set +e
  out="$(cd "$TMP" && PATH="$p" "$SYS/bash" "$SUBJECT/install.sh" "$@" < /dev/null 2>&1)"; rc=$?
  set -e
}
brew_calls() { [ -f "$TMP/brew.log" ] && grep -c '^install ' "$TMP/brew.log" || echo 0; }
snapshot() { (cd "$HOME" && find . | sort | cksum); }

# ── 3. it installs, one by one, each command printed before it runs ─────────
reset
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --with-recommended
[ "$rc" = 0 ] || fail "--with-recommended did not finish clean (rc=$rc): $out"
want="install herdr
install --cask claude-code
install ripgrep
install fd
install sd
install gentleman-programming/tap/engram
install fzf"
got="$(grep '^install ' "$TMP/brew.log")"
[ "$got" = "$want" ] || fail "brew was not called once per missing package, in order: got [$got]"
pass "--with-recommended runs one brew install per missing package, required first, Claude Code as the cask, jq (present) left alone"
order="$(printf '%s\n' "$out" | grep -E '^  \+ brew install |^BREW-RAN ' | sed -E 's/^  \+ brew install (--cask )?([^ ]+).*/CMD \2/; s/^BREW-RAN /RAN /')"
expect="$(printf '%s\n' herdr claude-code ripgrep fd sd gentleman-programming/tap/engram fzf | while read -r p; do printf 'CMD %s\nRAN %s\n' "$p" "$p"; done)"
[ "$order" = "$expect" ] || fail "a command was not printed before brew ran it: $order"
pass "each command is printed before brew runs it"
rm -f "$TMP/brew.log"
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --with-recommended
[ "$rc" = 0 ] && [ "$(brew_calls)" = 0 ] || fail "with everything present it still called brew ($(brew_calls)) or exited $rc"
pass "with everything present it installs nothing and exits 0"

# ── 1. without the flag, nobody calls brew ──────────────────────────────────
reset
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --brain "$HOME/brain" --check
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --brain "$HOME/brain"
[ "$rc" != 0 ] || fail "a real run with herdr missing did not stop"
[ ! -f "$TMP/brew.log" ] || fail "install.sh called brew without --with-recommended: $(cat "$TMP/brew.log")"
pass "without --with-recommended neither --check nor a real run calls brew"

# ── 2. --with-recommended --check runs nothing ─────────────────────────────
reset; before="$(snapshot)"
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --with-recommended --check
[ ! -f "$TMP/brew.log" ] || fail "--with-recommended --check called brew: $(cat "$TMP/brew.log")"
[ "$before" = "$(snapshot)" ] || fail "--with-recommended --check wrote under HOME"
for p in herdr "--cask claude-code" ripgrep fd sd gentleman-programming/tap/engram fzf; do
  case "$out" in *"+ brew install $p "*) ;; *) fail "--with-recommended --check does not print 'brew install $p': $out" ;; esac
done
[ "$rc" = 1 ] || fail "--with-recommended --check with packages to install exited $rc, not 1"
pass "--with-recommended --check prints every command, runs none, writes nothing (exit 1: something to do)"

# ── 4. one failure does not stop the rest ───────────────────────────────────
reset
BREW_FAIL=fd run "$GOT:$BREWBIN:$NEWJQ:$SYS" --with-recommended
[ "$rc" = 1 ] || fail "a failed install exited $rc, not 1"
grep -q '^install fzf$' "$TMP/brew.log" || fail "after fd failed, the next packages were not tried"
case "$out" in *"FAILED brew install fd"*"still missing: fd"*) ;; *) fail "the failed package is not named: $out" ;; esac
pass "a failing brew install is named, the rest are still tried, exit 1"

# ── 5. an old jq is missing ─────────────────────────────────────────────────
reset
run "$GOT:$BREWBIN:$OLDJQ:$SYS" --with-recommended
grep -q '^install jq$' "$TMP/brew.log" || fail "jq 1.6 was not replaced: $(cat "$TMP/brew.log")"
pass "a jq older than 1.7 is installed again"

# ── 6. no brew ──────────────────────────────────────────────────────────────
reset
run "$GOT:$NEWJQ:$SYS" --with-recommended
[ "$rc" = 1 ] || fail "with no brew it exited $rc, not 1"
case "$out" in *"Homebrew is not on PATH"*"https://brew.sh"*) ;; *) fail "with no brew it does not say so: $out" ;; esac
[ -z "$(ls -A "$GOT")" ] || fail "with no brew something was installed"
pass "with no brew it names https://brew.sh, installs nothing, exit 1"

# ── 7. --check points at it ─────────────────────────────────────────────────
reset
run "$GOT:$BREWBIN:$NEWJQ:$SYS" --brain "$HOME/brain" --check
case "$out" in *"--with-recommended"*) ;; *) fail "--check with brew present and tools missing does not name --with-recommended: $out" ;; esac
run "$GOT:$NEWJQ:$SYS" --brain "$HOME/brain" --check
case "$out" in *"--with-recommended"*) fail "--check names --with-recommended where there is no brew" ;; esac
pass "--check names --with-recommended only when brew could do it"
# ── 8. brew reading stdin ───────────────────────────────────────────────────
reset
BREW_EATS=1 run "$GOT:$BREWBIN:$NEWJQ:$SYS" --with-recommended
[ "$(brew_calls)" = 7 ] && [ "$rc" = 0 ] || fail "a brew that reads stdin left $(brew_calls) of 7 installs run (rc=$rc): $out"
pass "a brew that reads its stdin still gets every package"
printf 'coverage - 571: 8 live claims, 0 mutants — run red against the base install.sh instead (commit message)\n'
