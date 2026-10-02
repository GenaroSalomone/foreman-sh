#!/usr/bin/env bash
# The Homebrew keg builds a brain: no .git, its own command name, nothing pointing back
#
# WHAT THIS HOLDS. The formula (setup/packaging/homebrew/foreman-sh.rb.in) puts the
# release tree in libexec/, with no .git, and links bin/foreman-sh as Homebrew's
# write_env_script: export FOREMAN_SH_VERSION and FOREMAN_SH_INSTALL_CMD, then
# exec libexec/install.sh. Before, install.sh had no way to know either: run from
# a keg it said "install.sh unknown" and told the person to run ./install.sh,
# which a brew user does not have. Claims, over a keg laid out the same way:
#
#   1. `foreman-sh --version` prints the packaged version;
#   2. `foreman-sh --brain … --lane … --repo …` builds the brain, and its marker
#      carries the packaged version;
#   3. every hint names `foreman-sh`, never `./install.sh`;
#   4. the keg is not written to, and nothing in the brain or the bin dir links
#      into it (`brew upgrade` deletes the old keg).
#
# setup/packaging/homebrew/verify does the same against a real `brew install`.
#
# Point it at another tree to watch it fail there:
#     SUBJECT_ROOT=/path/to/old/tree bash setup/tests/572-la-formula-instala-un-cerebro.sh
#
# Run alone:  bash setup/tests/572-la-formula-instala-un-cerebro.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR OPENCODE_CONFIG_DIR OPENCODE_EXECUTOR_CONFIG
SUBJECT="${SUBJECT_ROOT:-$ROOT}"
export ENGRAM_PORT=9 ENGRAM_DATA_DIR="$TMP/engram"   # never 7437
export TMPDIR="$TMP"

# ── the keg: the tree without .git in libexec/, and the env script Homebrew writes ──
KEG="$TMP/Cellar/foreman-sh/9.9.9"; mkdir -p "$KEG/libexec" "$KEG/bin"
( cd "$SUBJECT" && tar --exclude=./.git -cf - . ) | tar -xf - -C "$KEG/libexec"
cat > "$KEG/bin/foreman-sh" <<EOF
#!/bin/bash
export FOREMAN_SH_VERSION="v9.9.9"
export FOREMAN_SH_INSTALL_CMD="foreman-sh"
exec "$KEG/libexec/install.sh" "\$@"
EOF
chmod +x "$KEG/bin/foreman-sh"
keg_sum() { (cd "$KEG" && find . -type f -exec cksum {} + | sort); }
keg_before="$(keg_sum)"

# ── a HOME with the prerequisites met (480's stubs) ─────────────────────────
STUBS="$TMP/prereq-bin"; mkdir -p "$STUBS"
cat > "$STUBS/herdr" <<'STUB'
#!/usr/bin/env bash
[ "$1 $2" = "integration status" ] && { echo "claude: current (x)"; exit 0; }
exit 0
STUB
cat > "$STUBS/claude" <<'STUB'
#!/usr/bin/env bash
[ "$1 $2" = "auth status" ] && echo '{"loggedIn":true}'
exit 0
STUB
chmod +x "$STUBS/herdr" "$STUBS/claude"
export PATH="$KEG/bin:$STUBS:$PATH"
printf '{"hasCompletedOnboarding":true}' > "$HOME/.claude.json"
mkdir -p "$HOME/.claude"; printf '{"skipDangerousModePermissionPrompt":true}' > "$HOME/.claude/settings.json"
git config --global user.email t@t; git config --global user.name t
REPO="$HOME/code/myapp"; mkdir -p "$REPO"
( cd "$REPO" && git init -q -b main && echo hi > README.md && git add . && git commit -q -m base )

fs() { set +e; out="$(cd "$TMP" && foreman-sh "$@" < /dev/null 2>&1)"; rc=$?; set -e; }

# ── 1. the version ──────────────────────────────────────────────────────────
fs --version
[ "$rc" = 0 ] && [ "$out" = "install.sh v9.9.9" ] || fail "foreman-sh --version printed '$out' (rc=$rc), not 'install.sh v9.9.9'"
pass "foreman-sh --version prints the packaged version, with no .git in the keg"

# ── 3. the hints name foreman-sh ───────────────────────────────────────────
fs --brain "$HOME/brain" --check
case "$out" in *"foreman-sh --brain $HOME/brain --lane <name> --repo <path>"*) ;; *) fail "--check from the keg does not name foreman-sh as the next command: $out" ;; esac
case "$out" in *"./install.sh"*) fail "--check from the keg tells a brew user to run ./install.sh: $out" ;; esac

# ── 2. the brain ────────────────────────────────────────────────────────────
fs --brain "$HOME/brain" --lane myapp --repo "$REPO" --operator me --min-model none --requested-by none
[ "$rc" = 0 ] || fail "foreman-sh did not install (rc=$rc): $(printf '%s' "$out" | tail -8 | tr '\n' ' ')"
[ -x "$HOME/brain/bin/hw" ] && [ -d "$HOME/brain/myapp" ] || fail "no brain, or no lane, after foreman-sh"
[ "$(jq -r .version "$HOME/brain/.brain-install.json")" = v9.9.9 ] || fail "the marker says $(jq -r .version "$HOME/brain/.brain-install.json"), not v9.9.9"
pass "foreman-sh builds the brain and a lane, and the marker carries v9.9.9"
case "$out" in *"./install.sh"*) fail "the install from the keg tells a brew user to run ./install.sh: $out" ;; esac
pass "every hint names foreman-sh, never ./install.sh"

# ── 4. nothing points back into the keg, and the keg is untouched ──────────
[ "$keg_before" = "$(keg_sum)" ] || fail "the install wrote into the keg"
back="$(find "$HOME/brain" "$HOME/.local/bin" -type l -exec readlink {} \; | grep -F "$KEG" || true)"
[ -z "$back" ] || fail "links point into the keg, which brew upgrade deletes: $back"
pass "the keg is not written to, and no link in the brain or ~/.local/bin points into it"
printf 'coverage - 572: 4 live claims, 0 mutants — run red against the base install.sh instead (commit message)\n'
