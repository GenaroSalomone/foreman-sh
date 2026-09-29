#!/usr/bin/env bash
# Proves both halves of the reinstall defense: exact v12 repair works on a
# disposable copy, and --check rejects an unpatched copy.
#
# THE LIVE INSTALLED PLUGIN IS NO LONGER READ HERE (since 2026-09-22). It used
# to be the fixture's source and the last assertion's subject, so a
# `herdr integration install opencode` turned the suite red — a machine event,
# not a code change. The subject is now setup/fixtures/herdr-agent-state.js, a
# copy of the patched plugin; the live file is checked by
# setup/check-machine, which runs the same `--check` against it.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

installer="$ROOT/setup/install-opencode-background-state.sh"
patched="$FIXTURES/herdr-agent-state.js"
[ -x "$installer" ] || fail "OpenCode background-state drift detector is executable"
[ -f "$patched" ] || fail "OpenCode background-state drift detector found its patched-plugin fixture"
# Installed where the installer looks by default, so its no-override path is
# the one exercised below — the same path setup/check-machine takes live.
# That WRITES under $HOME, so refuse unless $HOME is this run's own: a copy of
# this file sourcing an older _common.sh must not overwrite the real plugin.
case "$HOME" in "$TMP"/*) ;; *) fail "refusing to install a fixture into a HOME this run does not own: $HOME" ;; esac
mkdir -p "$XDG_CONFIG_HOME/opencode/plugins"
cp "$patched" "$HOME/.config/opencode/plugins/herdr-agent-state.js"
live="$HOME/.config/opencode/plugins/herdr-agent-state.js"

# The unpatched subject is herdr's v12 plugin as upstream ships it, frozen beside the
# patched copy (also v12), so the repair is proved against the shape it targets and
# must reproduce the patched fixture byte for byte.
fixture="$TMP/herdr-agent-state.js"
cp "$FIXTURES/herdr-agent-state-unpatched.js" "$fixture"

if OPENCODE_HERDR_STATE_PLUGIN="$fixture" bash "$installer" --check >/dev/null 2>&1; then
  fail "OpenCode background-state drift detector rejects a reinstall-shaped plugin"
else
  pass "OpenCode background-state drift detector rejects a reinstall-shaped plugin"
fi
OPENCODE_HERDR_STATE_PLUGIN="$fixture" bash "$installer" --apply >/dev/null
OPENCODE_HERDR_STATE_PLUGIN="$fixture" bash "$installer" --check >/dev/null \
  && pass "OpenCode background-state repair produces a detectable patched plugin" \
  || fail "OpenCode background-state repair did not produce a detectable patched plugin"
cmp -s "$fixture" "$patched" \
  && pass "OpenCode background-state repair of the v12 plugin reproduces the patched fixture" \
  || fail "OpenCode background-state repair of the v12 plugin differs from the patched fixture"
bash "$installer" --check >/dev/null \
  && pass "OpenCode background-state drift detector verifies a patched plugin at its default install path" \
  || fail "OpenCode background-state drift detector rejects the patched fixture at its default install path"
