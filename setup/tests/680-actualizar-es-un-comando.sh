#!/usr/bin/env bash
# Upgrading foreman-sh is one command, and it keeps the install's flags
#
# WHAT THIS HOLDS. Before: an upgrade was the new release plus install.sh run
# again with the flags of the first install, from memory; a forgotten flag made
# a different install and nothing said so. Now install.sh records its flags in
# <brain>/.foreman/install.json, and `upgrade` replays them from the release it
# fetched. Claims, over a fixture brain, two fake releases served as file://
# tarballs (the network is never touched), and an empty HOME:
#
#   1. an install writes the record (version, date, its flags), and a second
#      identical run leaves it byte for byte;
#   2. a brain with no record (what the old installer made): `upgrade` refuses,
#      names the flags to pass once, and writes nothing;
#   3. --dry-run names the target and every recorded flag, and changes nothing;
#   4. `upgrade` installs the newest release with the SAME flags (custom bin
#      dir, permissions, a lane that was deleted and comes back with its
#      operator, floor and request rule), runs --check, prints the release's
#      "In short", and records the new version;
#   5. `upgrade --to 1.0.0` is the way back;
#   6. release-check, which `hw status` prints, says a newer release exists in
#      one line, asks once a day, and is silent offline, with no record, when
#      current, and when turned off;
#   7. mutants: dropped flags, a skipped --check, no "In short", no record,
#      an equal version announced as newer, a cache nobody reads, and a
#      status that never asks.
#
# Point it at another tree to watch it fail there (the base has no `upgrade`):
#     SUBJECT_ROOT=/path/to/old/tree bash setup/tests/680-actualizar-es-un-comando.sh
#
# Run alone:  bash setup/tests/680-actualizar-es-un-comando.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR OPENCODE_CONFIG_DIR OPENCODE_EXECUTOR_CONFIG \
      FOREMAN_SH_VERSION FOREMAN_SH_INSTALL_CMD FOREMAN_SH_INSTALLED_FROM FOREMAN_SH_UPGRADE_TARGET \
      FOREMAN_SH_NO_UPDATE_CHECK FOREMAN_SH_LATEST_URL FOREMAN_SH_TARBALL_BASE
SUBJECT="${SUBJECT_ROOT:-$ROOT}"
export ENGRAM_PORT=9 ENGRAM_DATA_DIR="$TMP/engram"   # never 7437
export TMPDIR="$TMP"

# ── a HOME with the prerequisites met (572's stubs) ─────────────────────────
STUBS="$TMP/prereq-bin"; mkdir -p "$STUBS"
cat > "$STUBS/herdr" <<'STUB'
#!/usr/bin/env bash
[ "$1 $2" = "integration status" ] && { echo "claude: current (x)"; exit 0; }
case "${1:-} ${2:-}" in
  "workspace list") echo '{"result":{"workspaces":[]}}' ;;
  "pane list")      echo '{"result":{"panes":[]}}' ;;
  "agent list")     echo '{"result":{"type":"agent_list","agents":[]}}' ;;
esac
exit 0
STUB
cat > "$STUBS/claude" <<'STUB'
#!/usr/bin/env bash
[ "$1 $2" = "auth status" ] && echo '{"loggedIn":true}'
exit 0
STUB
chmod +x "$STUBS/herdr" "$STUBS/claude"
export PATH="$STUBS:$PATH"
printf '{"hasCompletedOnboarding":true}' > "$HOME/.claude.json"
mkdir -p "$HOME/.claude"; printf '{"skipDangerousModePermissionPrompt":true}' > "$HOME/.claude/settings.json"
git config --global user.email t@t; git config --global user.name t
REPO="$HOME/code/myapp"; mkdir -p "$REPO"
TMPR="$(cd -P "$TMP" && pwd)"; RREPO="$(cd -P "$REPO" && pwd)"   # the installer records real paths
( cd "$REPO" && git init -q -b main && echo hi > README.md && git add . && git commit -q -m base )

# ── two fake releases, as the tarballs GitHub serves for a tag ──────────────
REL="$TMP/releases"; mkdir -p "$REL"
copy_tree() {  # <dst>
  mkdir -p "$1"; ( cd "$SUBJECT" && tar --exclude=./.git -cf - . ) | tar -xf - -C "$1"
}
notes() {  # <version> <in short text> — a RELEASE-NOTES.md with an "In short" section
  printf '# foreman-sh %s\n\n## In short\n\n%s\n\n## Added\n\n- a thing\n' "$1" "$2"
}
package() {  # <tree> <tag> — <tree> becomes <REL>/<tag>.tar.gz, one top directory like GitHub's
  local top="$TMP/pkg/$2/foreman-sh-${2#v}"
  rm -rf "$TMP/pkg/$2"; mkdir -p "$TMP/pkg/$2"; cp -R "$1" "$top"
  tar -czf "$REL/$2.tar.gz" -C "$TMP/pkg/$2" "foreman-sh-${2#v}"
}
build_releases() {  # <mutate fn, or ":"> — v1.0.0 (what is installed) and v2.0.0 (the newest), both mutated alike
  rm -rf "$TMP/rel-v1" "$TMP/rel-v2"
  copy_tree "$TMP/rel-v1"; notes v1.0.0 "THE-OLD-RELEASE-IN-SHORT" > "$TMP/rel-v1/RELEASE-NOTES.md"
  copy_tree "$TMP/rel-v2"; notes v2.0.0 "THE-NEW-RELEASE-IN-SHORT: it is faster" > "$TMP/rel-v2/RELEASE-NOTES.md"
  : > "$TMP/rel-v2/bin/probe-new-release"
  "$1" "$TMP/rel-v1"; "$1" "$TMP/rel-v2"
  package "$TMP/rel-v1" v1.0.0; package "$TMP/rel-v2" v2.0.0
  printf '{"tag_name":"v2.0.0"}\n' > "$REL/latest.json"
}
replace_in() {  # <file> <old> <new> — a mutation that did not apply is a failure, never a pass
  python3 - "$@" <<'PY' || fail "the mutation did not apply: $2 (in $1)"
import sys
p, old, new = sys.argv[1:4]
s = open(p).read()
if s.count(old) != 1: sys.exit(1)
open(p, "w").write(s.replace(old, new))
PY
}
build_releases :

export FOREMAN_SH_LATEST_URL="file://$REL/latest.json" FOREMAN_SH_TARBALL_BASE="file://$REL"

# ── an install, then what upgrading it does, as one function the mutants reuse ──
# <name> <v1 tree> — installs v1.0.0 with flags a default would not give, in $TMP/<name>
FLAGS_OF=()
install_v1() {
  local n="$1" src="$2"
  BR="$TMPR/$n/brain"; BINX="$TMPR/$n/bin-x"; mkdir -p "$TMP/$n"
  rm -f "$HOME/.config/hw/permissions"
  printf '{"skipDangerousModePermissionPrompt":true}' > "$HOME/.claude/settings.json"   # one user, one brain: each scenario starts clean
  FLAGS_OF=(--brain "$BR" --lane myapp --repo "$REPO" --base main --operator "Ada L" --min-model sonnet \
            --requested-by required --bin-dir "$BINX" --permissions ask)
  set +e; v1out="$(cd "$TMP" && FOREMAN_SH_VERSION=v1.0.0 bash "$src/install.sh" "${FLAGS_OF[@]}" < /dev/null 2>&1)"; v1rc=$?; set -e
}
upgrade() {  # [args...] — from the installed release's own copy, as a person would
  set +e; uout="$(cd "$TMP" && FOREMAN_SH_VERSION=v1.0.0 bash "$TMP/rel-v1/install.sh" upgrade --brain "$BR" "$@" < /dev/null 2>&1)"; urc=$?; set -e
}
tree_sum() { ( cd "$TMP" && find "$HOME" "$BR" "$BINX" -path "$HOME/code" -prune -o -path "$HOME/.local" -prune -o -type f -print0 2>/dev/null | sort -z | xargs -0 shasum 2>/dev/null ) | shasum; }
# what is true of the brain after an upgrade, one line per fact, for the mutants to be read by
facts() {
  local m c
  m="$(jq -r .version "$BR/.brain-install.json" 2>/dev/null || echo none)"
  printf 'marker version: %s\n' "$m"
  printf 'record: %s\n' "$([ -f "$BR/.foreman/install.json" ] && jq -r .version "$BR/.foreman/install.json" || echo absent)"
  printf 'bin-dir links after upgrade: %s\n' "$([ -L "$BINX/hw" ] && echo present || echo missing)"
  printf 'permissions after upgrade: %s\n' "$(cat "$HOME/.config/hw/permissions" 2>/dev/null || echo none)"
  printf 'lane operator: %s\n' "$(jq -r '.operator // "none"' "$BR/projects.json" 2>/dev/null)"
  printf 'lane floor: %s\n' "$(jq -r '.lanes.myapp.model_floor.tier // "none"' "$BR/projects.json" 2>/dev/null)"
  printf 'lane request rule: %s\n' "$(jq -r '.lanes.myapp.requested_by // "none"' "$BR/projects.json" 2>/dev/null)"
  printf 'lane dir after upgrade: %s\n' "$([ -d "$BR/myapp" ] && echo present || echo missing)"
  printf 'new release in the brain: %s\n' "$([ -e "$BR/bin/probe-new-release" ] && echo yes || echo no)"
  case "$uout" in *"--check: nothing written"*) c=yes ;; *) c=no ;; esac
  printf 'check ran: %s\n' "$c"
  case "$uout" in *"THE-NEW-RELEASE-IN-SHORT: it is faster"*) c=yes ;; *) c=no ;; esac
  printf 'in short printed: %s\n' "$c"
}
# the install is deleted down to what the upgrade has to bring back; the lane's
# row in projects.json stays: it is where operator, floor and request rule live
forget_lane() {
  rm -rf "$BR/myapp" "$BINX" "$HOME/.config/hw/permissions"
}

# ── 1. an install records its flags, and a second run leaves the record alone ──
install_v1 main "$TMP/rel-v1"
[ "$v1rc" = 0 ] || fail "the install did not run (rc=$v1rc): $(printf '%s' "$v1out" | tail -8 | tr '\n' ' ')"
REC="$BR/.foreman/install.json"
[ -f "$REC" ] || fail "the install wrote no record at .foreman/install.json"
[ "$(jq -r .version "$REC")" = v1.0.0 ] || fail "the record says $(jq -r .version "$REC"), not v1.0.0"
[ "$(jq -r '.date | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]{8}Z$")' "$REC")" = true ] || fail "the record carries no date: $(cat "$REC")"
want="$(jq -c '[.bin_dir, .permissions, .lanes[0].lane, .lanes[0].repo, .lanes[0].base]' "$REC")"
[ "$want" = "$(printf '["%s","ask","myapp","%s","main"]' "$BINX" "$RREPO")" ] || fail "the record does not carry the install's own flags: $(cat "$REC")"
[ "$(jq -c '.lanes[0] | keys' "$REC")" = '["base","lane","repo"]' ] || fail "the record keeps what the lane's row in projects.json already says (vendor, model, operator, floor, request rule), which goes stale when the row is edited: $(cat "$REC")"
[ "$(jq -r 'keys | join(",")' "$REC")" = "bin_dir,date,lanes,latest_url,permissions,version" ] || fail "the record carries something besides version, date, flags and where to look for the newest release: $(jq -c keys "$REC")"
before="$(shasum "$REC")"; sleep 1
set +e; again="$(cd "$TMP" && FOREMAN_SH_VERSION=v1.0.0 bash "$TMP/rel-v1/install.sh" "${FLAGS_OF[@]}" < /dev/null 2>&1)"; set -e
[ "$before" = "$(shasum "$REC")" ] || fail "a second identical install rewrote the record"
pass "an install writes .foreman/install.json with its version, date and flags, and a second identical run leaves it byte for byte"

# ── 2. no record: upgrade refuses, says which flags, writes nothing ─────────
cp -R "$BR" "$TMP/main-with-record"
rm -rf "$BR/.foreman"
sum0="$(tree_sum)"
upgrade
[ "$urc" = 1 ] || fail "upgrade over a brain with no record exited $urc, not 1: $uout"
case "$uout" in *"no install record"*"--brain $BR --lane myapp --repo $RREPO"*) ;; *) fail "the refusal does not name the flags to pass: $uout" ;; esac
[ "$sum0" = "$(tree_sum)" ] || fail "a refused upgrade wrote something"
pass "a brain with no record is refused, the lane and repo to pass are named, and nothing is written"
rm -rf "$BR"; mv "$TMP/main-with-record" "$BR"

# ── 3. --dry-run says it all and changes nothing ────────────────────────────
sum0="$(tree_sum)"; ls0="$(ls "$TMP" | sort)"
upgrade --dry-run
[ "$urc" = 0 ] || fail "upgrade --dry-run exited $urc: $uout"
for needle in "dry run" "target     v2.0.0" "recorded   v1.0.0" "--bin-dir $BINX" "--permissions ask" "--lane myapp" "--base main" "$REL/v2.0.0.tar.gz"; do
  case "$uout" in *"$needle"*) ;; *) fail "--dry-run does not say '$needle': $uout" ;; esac
done
[ "$sum0" = "$(tree_sum)" ] && [ "$ls0" = "$(ls "$TMP" | sort)" ] || fail "--dry-run changed something"
case "$uout" in *"--operator"*|*"--min-model"*|*"--requested-by"*|*"--vendor"*|*"--model "*) fail "--dry-run replays a rule the lane's row owns: $uout" ;; esac
pass "--dry-run names the target, the download and every recorded flag, and changes nothing"

# ── 4. upgrade: newest release, same flags, --check, In short, a new record ──
forget_lane
upgrade
[ "$urc" = 0 ] || fail "upgrade exited $urc: $(printf '%s' "$uout" | tail -12)"
survivors="$(facts)"
expected='marker version: v2.0.0
record: v2.0.0
bin-dir links after upgrade: present
permissions after upgrade: ask
lane operator: Ada L
lane floor: sonnet
lane request rule: required
lane dir after upgrade: present
new release in the brain: yes
check ran: yes
in short printed: yes'
[ "$survivors" = "$expected" ] || fail "after upgrade the brain is not the install again on v2.0.0: $(diff <(printf '%s\n' "$expected") <(printf '%s\n' "$survivors") | tr '\n' ' ')"
case "$uout" in *"upgraded v1.0.0 -> v2.0.0"*) ;; *) fail "upgrade does not say what it did: $uout" ;; esac
[ "$(jq -r '.lanes | length' "$REC")" = 1 ] || fail "the upgrade changed what the record holds: $(cat "$REC")"
pass "upgrade installs the newest release with the recorded flags, runs --check, prints its In short and records v2.0.0"
SURVIVAL_FACTS="$survivors"

# ── 4b. a row edited by hand after the install is not overwritten by the record ──
# The record keeps the install's own flags; what the lane declares is read from
# projects.json when the upgrade runs, so a hand edit is neither replayed back
# to its old value nor refused as a conflict.
install_v1 stale "$TMP/rel-v1"
[ "$v1rc" = 0 ] || fail "the stale-scenario install did not run (rc=$v1rc)"
jq '.operator = "Grace H" | .lanes.myapp.model_floor.tier = "opus" | del(.lanes.myapp.requested_by)' "$BR/projects.json" > "$BR/projects.json.t" && mv "$BR/projects.json.t" "$BR/projects.json"
upgrade
[ "$urc" = 0 ] || fail "upgrade refused a lane whose row was edited by hand (rc=$urc): $(printf '%s' "$uout" | tail -6 | tr '\n' ' ')"
[ "$(jq -r '[.operator, .lanes.myapp.model_floor.tier, (.lanes.myapp.requested_by // "none")] | join(",")' "$BR/projects.json")" = "Grace H,opus,none" ] \
  || fail "upgrade put the recorded values back over the hand edit: $(jq -c '[.operator, .lanes.myapp]' "$BR/projects.json")"
pass "a lane edited by hand in projects.json upgrades, and keeps the edit"

# a record written by 0.3.0 still carries those values: projects.json wins, in one line
jq '.lanes[0] += {"min_model":"sonnet","requested_by":"required","operator":"Ada L"}' "$BR/.foreman/install.json" > "$BR/.foreman/install.json.t" && mv "$BR/.foreman/install.json.t" "$BR/.foreman/install.json"
upgrade
[ "$urc" = 0 ] || fail "upgrade refused an old-format record over an edited row (rc=$urc): $(printf '%s' "$uout" | tail -6 | tr '\n' ' ')"
[ "$(printf '%s\n' "$uout" | rg -c 'projects.json wins')" = 1 ] || fail "upgrade did not say in one line that projects.json wins over the record: $uout"
pass "an old-format record that disagrees with projects.json: projects.json wins, said in one line"
install_v1 main "$TMP/rel-v1"; forget_lane; upgrade   # back to the main brain, as claim 4 left it

# ── 5. --to is the way back ─────────────────────────────────────────────────
sum0="$(tree_sum)"
set +e; uout="$(cd "$TMP" && FOREMAN_SH_VERSION=v2.0.0 bash "$TMP/rel-v2/install.sh" upgrade --brain "$BR" --to 1.0.0 < /dev/null 2>&1)"; urc=$?; set -e
[ "$urc" = 0 ] || fail "upgrade --to 1.0.0 exited $urc: $(printf '%s' "$uout" | tail -8)"
[ "$(jq -r .version "$BR/.brain-install.json")" = v1.0.0 ] && [ ! -e "$BR/bin/probe-new-release" ] || fail "--to 1.0.0 did not put v1.0.0 back"
case "$uout" in *"THE-OLD-RELEASE-IN-SHORT"*"") ;; *) fail "--to does not print the In short of the version it installed: $uout" ;; esac
set +e; bad="$(cd "$TMP" && FOREMAN_SH_VERSION=v2.0.0 bash "$TMP/rel-v2/install.sh" upgrade --brain "$BR" --to 7.7.7 < /dev/null 2>&1)"; badrc=$?; set -e
[ "$badrc" != 0 ] && [ "$(jq -r .version "$BR/.brain-install.json")" = v1.0.0 ] || fail "--to a release that does not exist changed the brain or succeeded: $bad"
pass "--to 1.0.0 puts the old release back, and a release that does not exist changes nothing"

# ── 6. release-check, the line hw status prints ─────────────────────────────
CHECKER="$SUBJECT/bin/release-check"
[ -f "$CHECKER" ] || fail "no bin/release-check in $SUBJECT: hw status has nothing to ask"
RB="$TMP/rc-brain"; mkdir -p "$RB/.foreman"
printf '{"version":"v1.0.0","date":"2026-01-01T00:00:00Z"}\n' > "$RB/.foreman/install.json"
ask() { rc_out="$(python3 "$CHECKER" "$RB" 2>&1)"; rc_rc=$?; }
set +e
ask
[ "$rc_rc" = 0 ] && [ "$(printf '%s\n' "$rc_out" | wc -l | tr -d ' ')" = 1 ] && [[ "$rc_out" == "foreman-sh v2.0.0 is out"*"has v1.0.0"* ]] \
  || fail "a newer release was not announced in one line (rc=$rc_rc): $rc_out"
printf '{"tag_name":"v3.0.0"}\n' > "$REL/latest.json"; ask
[[ "$rc_out" == "foreman-sh v2.0.0 is out"* ]] || fail "asked twice in one day: the cached answer was not used: $rc_out"
python3 - "$RB/.foreman/latest-release.json" <<'PY'
import json, sys
p = sys.argv[1]; d = json.load(open(p)); d["checked_at"] -= 2 * 86400; json.dump(d, open(p, "w"))
PY
ask
[[ "$rc_out" == "foreman-sh v3.0.0 is out"* ]] || fail "a day later it did not ask again: $rc_out"
printf '{"version":"v3.0.0"}\n' > "$RB/.foreman/install.json"; rm -f "$RB/.foreman/latest-release.json"; ask
[ "$rc_rc" = 0 ] && [ -z "$rc_out" ] || fail "an install already on the newest release was told about one: $rc_out"
printf '{"version":"v1.0.0"}\n' > "$RB/.foreman/install.json"; rm -f "$RB/.foreman/latest-release.json"
# with no override, it asks where the install recorded (bin/release-check names no repository itself)
printf '{"version":"v1.0.0","latest_url":"file://%s/latest.json"}\n' "$REL" > "$RB/.foreman/install.json"
printf '{"tag_name":"v2.0.0"}\n' > "$REL/latest.json"
rc_out="$(env -u FOREMAN_SH_LATEST_URL python3 "$CHECKER" "$RB" 2>&1)"
[[ "$rc_out" == "foreman-sh v2.0.0 is out"* ]] || fail "release-check ignored the latest_url the install recorded: $rc_out"
rm -f "$RB/.foreman/latest-release.json"; printf '{"version":"v1.0.0"}\n' > "$RB/.foreman/install.json"
rc_out="$(env -u FOREMAN_SH_LATEST_URL python3 "$CHECKER" "$RB" 2>&1)"
[ -z "$rc_out" ] || fail "release-check guessed a place to look when none was recorded: $rc_out"
rm -f "$RB/.foreman/latest-release.json"
FOREMAN_SH_LATEST_URL="file://$REL/nowhere.json" ask
[ "$rc_rc" = 0 ] && [ -z "$rc_out" ] || fail "offline was not silent (rc=$rc_rc): $rc_out"
FOREMAN_SH_NO_UPDATE_CHECK=1 ask
[ -z "$rc_out" ] || fail "FOREMAN_SH_NO_UPDATE_CHECK=1 did not silence it: $rc_out"
rm -rf "$RB/.foreman/latest-release.json" "$RB/.foreman/install.json"; ask
[ -z "$rc_out" ] && [ ! -e "$RB/.foreman/latest-release.json" ] || fail "a brain with no record was asked about: $rc_out"
set -e
pass "release-check: one line when newer, once a day, silent when current, offline, off, or with no record"

# hw status carries that line (the brain's own hw, from its own bin/)
printf '{"tag_name":"v2.0.0"}\n' > "$REL/latest.json"
set +e; st="$(cd "$TMP" && env -u HW_TASK -u HW_RUN HOME="$HOME" "$BR/bin/hw" status 2>&1 < /dev/null)"; strc=$?; set -e
rm -f "$BR/.foreman/latest-release.json"
set +e; st="$(cd "$TMP" && env -u HW_TASK -u HW_RUN HOME="$HOME" "$BR/bin/hw" status 2>&1 < /dev/null)"; strc=$?; set -e
case "$st" in *"foreman-sh v2.0.0 is out (this brain has v1.0.0)"*) ;; *) fail "hw status (rc=$strc) does not say a newer release exists: $(printf '%s' "$st" | tail -6 | tr '\n' ' ')" ;; esac
case "$st" in *"install.sh upgrade --brain $BR"*) ;; *) fail "hw status names a command this install does not have (no foreman-sh on PATH): $(printf '%s' "$st" | rg 'is out')" ;; esac
case "$st" in *"foreman-sh upgrade"*) fail "hw status says foreman-sh upgrade on a non-Homebrew install" ;; esac
pass "hw status prints the line for the brain it runs in, naming install.sh upgrade where there is no Homebrew foreman-sh"

# ── 7. mutants: each one runs the same scenario over a broken copy ──────────
# survival control above: the healthy copy gave every fact in $SURVIVAL_FACTS.
run_mutant() {  # <name> <mutation fn over a release tree>; prints the facts after upgrade
  local n="$1" fn="$2"
  build_releases "$fn"
  install_v1 "$n" "$TMP/rel-v1"
  [ "$v1rc" = 0 ] || { printf 'the mutant did not install (rc=%s): %s\n' "$v1rc" "$(printf '%s' "$v1out" | tail -3 | tr '\n' ' ')"; return 0; }
  forget_lane
  upgrade
  printf 'upgrade exit: %s\n' "$urc"
  facts
}
m_drop_flags()  { replace_in "$1/install.sh" 'if r.get("bin_dir"): g += ["--bin-dir", r["bin_dir"]]' 'pass'
                  replace_in "$1/install.sh" 'if r.get("permissions"): g += ["--permissions", r["permissions"]]' 'pass'; }
m_skip_check()  { replace_in "$1/install.sh" 'env -u FOREMAN_SH_UPGRADE_TARGET bash "$SRC/install.sh" --brain "$BR" --check || rc=$?' ':'; }
m_no_in_short() { replace_in "$1/install.sh" 'if not m or not m.group(1).strip(): raise SystemExit(1)' 'raise SystemExit(1)'; }
m_no_record()   { replace_in "$1/install.sh" 'os.replace(path + ".tmp", path)' 'os.remove(path + ".tmp")'; }
m_drop_lane()   { replace_in "$1/install.sh" 'a += ["--lane", l["lane"], "--repo", l["repo"]]' 'pass'; }

run_stale_mutant() {  # <name> <mutation fn> — the 4b scenario over a broken copy
  local n="$1" fn="$2"
  build_releases "$fn"
  install_v1 "$n" "$TMP/rel-v1"
  [ "$v1rc" = 0 ] || { printf 'the mutant did not install (rc=%s)\n' "$v1rc"; return 0; }
  jq '.operator = "Grace H" | .lanes.myapp.model_floor.tier = "opus" | del(.lanes.myapp.requested_by)' "$BR/projects.json" > "$BR/projects.json.t" && mv "$BR/projects.json.t" "$BR/projects.json"
  upgrade
  printf 'stale upgrade exit: %s\n' "$urc"
}
# Mutants that break independent facts share one run (an install and an upgrade
# cost seconds each); every one is still killed by its own fact.
# the stale replay: the record keeps, and the replay hands back, what the row owns
m_stale_replay() { local all='OWN = ("base", "vendor", "model", "operator", "min_model", "requested_by")'
                   replace_in "$1/install.sh" 'OWN = ("base",)
try:' "$all
try:"
                   replace_in "$1/install.sh" 'OWN = ("base",)
keys' "$all
keys"; }
m_drop_flags_and_lane() { m_drop_flags "$1"; m_drop_lane "$1"; }
m_skip_check_and_in_short() { m_skip_check "$1"; m_no_in_short "$1"; }
out="$(run_mutant M01 m_drop_flags_and_lane)"
saw_mutant "M01 the replay forgets the bin dir and the permissions" "$out" "bin-dir links after upgrade: missing" "permissions after upgrade: none"
saw_mutant "M02 the replay forgets the lane, so it does not come back" "$out" "lane dir after upgrade: missing"
out="$(run_mutant M03 m_skip_check_and_in_short)"
saw_mutant "M03 upgrade never runs --check" "$out" "check ran: no"
saw_mutant "M04 upgrade prints no In short" "$out" "in short printed: no"
out="$(run_mutant M05 m_no_record)"
saw_mutant "M05 the install writes no record" "$out" "record: absent"

out="$(run_stale_mutant M09 m_stale_replay)"
saw_mutant "M09 the record keeps what the row owns and the upgrade replays it over a hand edit" "$out" "stale upgrade exit: 1"

# release-check and hw status: the same fixture, one line changed
mutate_checker() {  # <old> <new> — a copy of the subject's bin/ with release-check mutated, in $TMP/mut-bin
  rm -rf "$TMP/mut-bin"; mkdir -p "$TMP/mut-bin"; cp "$CHECKER" "$TMP/mut-bin/release-check"
  replace_in "$TMP/mut-bin/release-check" "$1" "$2"
}
mut_ask() {  # prints what the mutated checker says for: current install, then a cached day
  local b="$TMP/mut-brain"; rm -rf "$b"; mkdir -p "$b/.foreman"
  printf '{"version":"v2.0.0"}\n' > "$b/.foreman/install.json"; printf '{"tag_name":"v2.0.0"}\n' > "$REL/latest.json"
  printf 'when current: [%s]\n' "$(python3 "$TMP/mut-bin/release-check" "$b" 2>&1)"
  printf '{"version":"v1.0.0"}\n' > "$b/.foreman/install.json"; rm -f "$b/.foreman/latest-release.json"
  python3 "$TMP/mut-bin/release-check" "$b" > /dev/null 2>&1
  printf '{"tag_name":"v9.0.0"}\n' > "$REL/latest.json"
  printf 'second ask in a day: [%s]\n' "$(python3 "$TMP/mut-bin/release-check" "$b" 2>&1)"
  printf '{"tag_name":"v2.0.0"}\n' > "$REL/latest.json"
}
# survival controls: the healthy checker is silent when current, and answers from its cache
mkdir -p "$TMP/mut-bin"; cp "$CHECKER" "$TMP/mut-bin/release-check"
healthy="$(mut_ask)"
[ "$healthy" = "when current: []
second ask in a day: [foreman-sh v2.0.0 is out (this brain has v1.0.0): install.sh upgrade --brain $TMP/mut-brain]" ] \
  || fail "the healthy release-check does not behave as the mutants assume: $healthy"
mutate_checker 'if tag and key(tag) > key(have):' 'if tag and key(tag) >= key(have):'
out="$(mut_ask)"
saw_mutant "M06 an equal version is announced as newer" "$out" "when current: [foreman-sh v2.0.0 is out"
mutate_checker 'if isinstance(checked, (int, float)) and 0 <= now - checked < DAY:' 'if False:'
out="$(mut_ask)"
saw_mutant "M07 the cache is written and never read" "$out" "second ask in a day: [foreman-sh v9.0.0 is out"

mutate_checker 'how(argv[1]), argv[1]))' '"foreman-sh", argv[1]))'
out="$(mut_ask)"
saw_mutant "M10 status names foreman-sh upgrade on an install that has no foreman-sh" "$out" "second ask in a day: [foreman-sh v2.0.0 is out (this brain has v1.0.0): foreman-sh upgrade"

mutate_hw() {  # a copy of the brain, its hw never asking
  rm -rf "$TMP/mut-brain-hw"; cp -R "$BR" "$TMP/mut-brain-hw"
  replace_in "$TMP/mut-brain-hw/bin/hw" '  _status_par_start release _release_quiet
' ''
}
mutate_hw
rm -f "$TMP/mut-brain-hw/.foreman/latest-release.json"
set +e; out="$(cd "$TMP" && env -u HW_TASK -u HW_RUN HOME="$HOME" "$TMP/mut-brain-hw/bin/hw" status 2>&1 < /dev/null)"; set -e
case "$out" in *"is out (this brain has v1.0.0)"*) said=yes ;; *) said=no ;; esac
saw_mutant "M08 hw status never asks" "status printed the newer-release line: $said" "status printed the newer-release line: no"
printf 'coverage - 680: 8 live claims, 10 mutants (10 must be named)\n'
