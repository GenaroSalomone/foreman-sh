#!/usr/bin/env bash
# Mutate copies only under $HW_ARTIFACTS, then exercise the real hook APIs.
#
# BEFORE 2026-09-06 every arm below named one of four Python or four JavaScript
# lane copies, because that is what existed. There is one of each now, plus
# eight shims, so every arm names a shared file — and that is a strictly better
# mutation target: a surviving mutant is now a hole in ALL FOUR LANES at once,
# which is exactly the blast radius the unification created and the thing worth
# proving the suite can see.
#
# Two arms still name a lane file on purpose (`shim-lane-name`, `shim-import`):
# the shims are the only per-lane code left, and an unexercised shim is how a
# lane silently stops being guarded while the shared module stays perfect.
set -euo pipefail

ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ART="${HW_ARTIFACTS:-$(mktemp -d "${TMPDIR:-/tmp}/deny-repo-writes-mutants.XXXXXX")}"
BASE="$ART/mutants"
OWN_ART=0
[ -n "${HW_ARTIFACTS:-}" ] || OWN_ART=1
rm -rf "$BASE"
mkdir -p "$BASE"
trap '[ "$OWN_ART" -eq 0 ] || rm -rf "$ART"' EXIT

copy_tree() {
  local dst="$1" lane
  mkdir -p "$dst/brain/setup/guards"
  cp "$ROOT/setup/guards/deny_repo_writes.py" "$dst/brain/setup/guards/"
  cp "$ROOT/setup/guards/deny-repo-writes.js" "$dst/brain/setup/guards/"
  cp "$ROOT/setup/guards/deny-repo-writes-vectors.json" "$dst/brain/setup/guards/"
  # THE POLICY THE MODULES READ, from `<root>/guards.json` beside `setup/`. A
  # copy without it would not load at all, and every arm would read VACUOUS.
  cp "$ROOT/guards.json" "$dst/brain/"
  # DERIVED FROM THE GUARD'S OWN LANE TABLE, NOT A SECOND HARDCODED LIST. This
  # used to be a fixed list of four lanes, and it drifted: a fifth lane was
  # registered in deny-repo-writes.js's LANES table (7a014e1) without this
  # list ever being touched, so every mutation arm crashed on
  # `Cannot find module '.../<lane>/.opencode/plugin/deny-repo-writes.js'` before
  # reaching the mutated line — every arm reported VACUOUS, not killed. Measured
  # 2026-09-09, chasing a mandatory pre-push gate that could never turn green.
  # `brain` (the root) is excluded because it is handled as the fifth lane
  # below, at a different path shape.
  for lane in $(node -e "import('$ROOT/setup/guards/deny-repo-writes.js').then(m => console.log(Object.keys(m.LANES).filter(l => l !== 'brain').join(' ')))"); do
    mkdir -p "$dst/brain/$lane/.claude/hooks" "$dst/brain/$lane/.opencode/plugin"
    cp "$ROOT/$lane/.claude/hooks/deny-repo-writes.py" "$dst/brain/$lane/.claude/hooks/"
    cp "$ROOT/$lane/.opencode/plugin/deny-repo-writes.js" "$dst/brain/$lane/.opencode/plugin/"
  done
  # THE ROOT IS THE FIFTH LANE and its shims live one directory up — see the
  # lane table's `brain` entry. Omitting them here does not make the mutation
  # run weaker, it makes it VACUOUS: the decision driver iterates every lane in
  # the table, so a missing shim crashes node before any mutation is exercised
  # and every arm after it reports "the mutated line may never have run".
  mkdir -p "$dst/brain/.claude/hooks" "$dst/brain/.opencode/plugin"
  cp "$ROOT/.claude/hooks/deny-repo-writes.py" "$dst/brain/.claude/hooks/"
  cp "$ROOT/.opencode/plugin/deny-repo-writes.js" "$dst/brain/.opencode/plugin/"
}

_mutate_one() {
  local name="$1" path="$2" old="$3" new="$4" expected="${5:-decision}" dir decision_rc filesystem_rc
  dir="$BASE/$name"
  copy_tree "$dir"
  python3 - "$dir/brain/$path" "$old" "$new" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()
old, new = sys.argv[2:]
if s.count(old) != 1:
    raise SystemExit(f"expected exactly one replacement for {old!r}, got {s.count(old)}")
p.write_text(s.replace(old, new, 1))
PY
  decision_rc=0
  filesystem_rc=0
  DENY_GUARD_ROOT="$dir/brain" node "$ROOT/setup/guards/test-deny-repo-writes.mjs" >"$dir/decision.txt" 2>&1 || decision_rc=$?
  TMPDIR="$dir" DENY_GUARD_ROOT="$dir/brain" node "$ROOT/setup/guards/test-deny-repo-writes-filesystem.mjs" >"$dir/filesystem.txt" 2>&1 || filesystem_rc=$?
  # A NON-ZERO EXIT IS NOT EVIDENCE OF A KILL, and this is the same rule
  # `saw_mutant` enforces in tests/_common.sh (see setup/decisions.md). A driver
  # that crashes on startup — a broken import, a missing fixture, a syntax error
  # the mutation happened to introduce — also exits non-zero, and certifying on
  # that alone passes every mutant that never reached the mutated line. Raised
  # by a Judgment Day judge against this exact function.
  #
  # So the kill must be a `not ok -` line the driver PRINTED: a named assertion
  # that flipped. That is text only a mutant which actually ran can produce.
  local out="$dir/$expected.txt" evidence
  case "$expected" in
    decision)   [ "$decision_rc" -ne 0 ] || { echo "not ok - mutant $name survived"; return 1; } ;;
    filesystem) [ "$filesystem_rc" -ne 0 ] || { echo "not ok - mutant $name survived"; return 1; } ;;
  esac
  evidence="$(grep -m1 '^not ok - ' "$out" 2>/dev/null || true)"
  if [ -z "$evidence" ]; then
    echo "not ok - mutant $name VACUOUS: the $expected driver exited non-zero but printed no failing assertion, so the mutated line may never have run — tail: $(tail -3 "$out" | tr '\n' ' ')"
    return 1
  fi
  echo "ok - mutant $name killed by $expected test ($evidence)"
}

# THE MUTANTS RUN FOUR AT A TIME AND ARE REPORTED IN ORDER. Each one owns
# $BASE/<name>, and both drivers make their own mkdtemp sandbox, so no mutant
# can see another's tree. Run one after another they were this suite's longest
# job: ~380s of wall clock on 2026-09-23 under the runner, most of it node
# start-up, and the guards lane runs nothing else meanwhile. What each mutant
# must show is unchanged; `mutate` only queues it, and `_mutate_report` below
# prints every result in declaration order and stops at the first failure,
# exactly as the serial version did.
MUTATE_JOBS="${MUTATE_JOBS:-4}"
MUTANTS=()
mutate() {
  local name="$1"
  while [ "$(jobs -rp | wc -l | tr -d ' ')" -ge "$MUTATE_JOBS" ]; do sleep 0.1; done
  mkdir -p "$BASE/$name"
  ( _rc=0; _mutate_one "$@" > "$BASE/$name.result" 2>&1 || _rc=$?
    printf '%s' "$_rc" > "$BASE/$name.rc" ) &
  MUTANTS+=("$name")
}
_mutate_report() {
  local name
  wait
  for name in "${MUTANTS[@]}"; do
    cat "$BASE/$name.result"
    [ "$(cat "$BASE/$name.rc" 2>/dev/null || echo missing)" = 0 ] || return 1
  done
}

GUARD_PY="setup/guards/deny_repo_writes.py"
GUARD_JS="setup/guards/deny-repo-writes.js"

# ── The write-detection tables, one runtime each ────────────────────────────
mutate copy-write-detection "$GUARD_JS" "rm|mv|cp|rsync|tee" "rm|mv|rsync|tee" filesystem
mutate copy-destination-selection "$GUARD_JS" 'const dest = operands[operands.length - 1];' 'const dest = operands[0];' filesystem
mutate mv-both-ways "$GUARD_JS" "rm|mv|cp|rsync|tee" "rm|cp|rsync|tee" filesystem
mutate install-destination "$GUARD_PY" "install|patch|sponge" "patch|sponge"
mutate rsync-destination "$GUARD_PY" "rm|mv|cp|rsync|tee" "rm|mv|cp|tee"

# ── The cwd axis ───────────────────────────────────────────────────────────
mutate bash-workdir "$GUARD_JS" "workdir ?? sessionDir" "sessionDir"
mutate python-cwd-boundary "$GUARD_PY" \
  "if cand == root or cand.startswith(root + \"/\"):" "if False:"
# The trigger text became "the command names a protected root (<path>)" on
# 2026-09-07: a lane now protects the other lanes' product repos, so "the repo
# or worktree path" no longer says WHICH tree, and a cross-lane denial that
# cannot name its tree reads to its reader as a misconfiguration.
mutate fired-reason "$GUARD_PY" \
  "the command names a protected root (%s)" "the command path"

# ── Code-vs-data, the escape hatches that keep masking safe ────────────────
mutate shell-c-code "$GUARD_PY" "(?:bash|sh|zsh|dash|ksh|ash)" "(?:sh|zsh|dash|ksh|ash)"
mutate heredoc-code "$GUARD_JS" '"(?:bash|sh|zsh|dash|ksh|ash)"' '"(?:zsh|dash|ksh|ash)"'

# ── Resolve-then-compare: the gate, both runtimes ──────────────────────────
# These are the arms for the holes measured on 2026-09-06 (a symlink or a `..`
# traversal into a protected tree was ALLOWED by all eight pre-unification
# copies). Turning the resolution off must be visible.
mutate python-resolution-gate "$GUARD_PY" \
  "resolved_token = _resolves_protected(cfg, no_heredoc)" "resolved_token = None"
mutate js-resolution-gate "$GUARD_JS" \
  "const resolvedToken = namesProtected ? null : resolvesProtected(cfg, noHeredoc);" \
  "const resolvedToken = null;"
mutate python-realpath "$GUARD_PY" \
  "        real = _real(path)" "        real = path"
mutate js-realpath "$GUARD_JS" \
  "  const real = realPath(p);" "  const real = normPath(p);"
# 2026-09-29: a DANGLING link into a protected root (`<link> -> <repo>/absent`)
# was walked past by the JS realpath and resolved to the link's own name, so a
# write that CREATES the target in the repo was allowed. Not following it again
# must be visible.
mutate js-dangling-link "$GUARD_JS" \
  "    if (!lstatSync(head).isSymbolicLink()) return null;" "    return null;"
# And a RELATIVE dangling target counts its `..` from the link's real parent:
# counting from the lexical one reopens the hole through a symlinked directory.
mutate js-dangling-real-parent "$GUARD_JS" \
  'realPath(dirname(head), n) + "/" + target' 'dirname(head) + "/" + target'

# ── Redirect targets go through the fail-closed predicate ──────────────────
mutate python-redirect-target "$GUARD_PY" \
  "        if not _lands_outside_protected(cfg, t, cwd):" "        if False:"
mutate js-redirect-target "$GUARD_JS" \
  "    if (!landsOutsideProtected(cfg, t, cwd)) {" "    if (false) {"

# ── The masker must honour bash's escapes ──────────────────────────────────
# Found by a Judgment Day judge and MEASURED: without the top-level backslash
# escape, `echo \' && rm -rf <REPO>/x \'` blanks the real `rm` out of the text
# every deny regex reads, and three of four lanes went DENY -> ALLOW.
mutate python-escaped-quote "$GUARD_PY" \
  "        if c == \"\\\\\" and i + 1 < n:" "        if False:"
mutate js-escaped-quote "$GUARD_JS" \
  "    if (c === \"\\\\\" && i + 1 < n) {" "    if (false) {"

# An unterminated quote must refuse the masking, not blank the rest of the
# command out of the text every deny regex reads.
mutate python-unbalanced-quote "$GUARD_PY" "    if not balanced:" "    if False:"
mutate js-unbalanced-quote "$GUARD_JS" \
  "quoted.balanced ? quoted.masked : noHeredoc" "quoted.masked"

# ── The lane-scoped exemption stays scoped ─────────────────────────────────
# Granting the spent-worktree teardown to every lane is the levelling-down this
# unification exists to refuse, so it must not pass unnoticed.
# The anchor moved on 2026-09-07: `roots` is now assembled from the lane's own
# pair PLUS `also_protect`, so the assignment is `cfg["roots"] = tuple(roots)`.
# The mutation itself is unchanged — grant every lane the teardown exemption and
# see whether anything notices.
mutate teardown-stays-lane-scoped "$GUARD_PY" \
  "    cfg[\"roots\"] = tuple(roots)" \
  "    cfg[\"roots\"] = tuple(roots); cfg[\"worktree_roots\"] = cfg[\"worktree_roots\"] or cfg[\"roots\"]"

# ── Destination, not payload: heredoc bodies must not open the gate ────────
# The gate that decides whether to even look at a command must ignore a
# protected root cited only inside a heredoc BODY that is not fed to a shell —
# see the module docstring's 2026-09-09 measurement. Reverting either half to
# scan the raw, unstripped command must turn the new heredoc-citation vectors
# from allow back to deny.
# The gate reads the text in two spellings since 53eacc5 (literal, and with
# backslash escapes removed), BOTH derived from the heredoc-stripped text. The
# mutant moves BOTH to the raw `probe`, so it is still exactly "the gate scans
# heredoc bodies" and nothing else; moving only one would leave the other half
# of the gate stripped and prove less than the pre-53eacc5 arm did.
mutate python-heredoc-gate "$GUARD_PY" \
  'if root in no_heredoc or root in unescaped
                       or root in no_heredoc_c or root in unescaped_c), None)' \
  'if root in probe or root in re.sub(r"\\(.)", r"\1", probe)), None)'
mutate js-heredoc-gate "$GUARD_JS" \
  'const namedRoot = roots.find((root) => noHeredoc.includes(root) || unescaped.includes(root)
    || noHeredocC.includes(root) || unescapedC.includes(root)) ?? null;' \
  'const namedRoot = roots.find((root) => probe.includes(root) || probe.replace(/\\(.)/g, "$1").includes(root)) ?? null;'

# ── A content-only match must say so ────────────────────────────────────────
# When the only signal is a protected path sitting inside a quoted argument,
# the deny message must admit it could not confirm a destination instead of
# asserting one. Forcing `content_only` permanently false must turn that
# admission back into the ordinary confident wording.
mutate python-content-only-honesty "$GUARD_PY" \
  "        and ((named_root is not None and named_root not in masked" \
  "        and False and ((named_root is not None and named_root not in masked"
mutate js-content-only-honesty "$GUARD_JS" \
  "const contentOnly = !standsIn && !cdsInto && (" \
  "const contentOnly = false && !standsIn && !cdsInto && ("

# ── Not every heredoc body is data: xargs/patch/ed/ex/interpreters ─────────
# Found adversarially while reviewing the heredoc-gate fix, before it shipped:
# treating EVERY non-shell heredoc target as data would let `xargs -I{} rm {}
# <<EOF` and a `patch <<EOF` whose diff header names a protected path straight
# past the gate. Narrowing this back to shells-only must turn both new vectors
# from deny back to allow.
mutate python-heredoc-body-is-live "$GUARD_PY" \
  '    r"(?:bash|sh|zsh|dash|ksh|ash|xargs|patch|ed|ex|"' \
  '    r"(?:bash|sh|zsh|dash|ksh|ash|"'
mutate js-heredoc-body-is-live "$GUARD_JS" \
  '  "(?:bash|sh|zsh|dash|ksh|ash|xargs|patch|ed|ex|" +' \
  '  "(?:bash|sh|zsh|dash|ksh|ash|" +'

# ── The message must name the destination it detected, not just the trigger ─
# Flagged by the brainer's own probe: a $HOME-resolved redirect target was
# correctly still denied, but the message described a literal-string test
# this branch does not run. Losing the resolved-destination statement must be
# visible.
mutate python-redirect-states-destination "$GUARD_PY" \
  '                    "redirect'"'"'s destination resolves to %s (written as "' \
  '                    "redirect target is protected: %s (spelled as "'
mutate js-redirect-states-destination "$GUARD_JS" \
  '        `redirect'"'"'s destination resolves to ${redirectHit.resolved} ` +' \
  '        `redirect target is protected ` +'

# ── The content-only fallback must point at the mechanism that works ───────
# A preservation copy through an interpreter cannot be told apart from a real
# write and stays denied (the safe default), but the message must not leave
# the operator without a way forward: `cp -R`/`rsync -a`/`install` already
# parse source from destination correctly. Losing that pointer must be visible.
mutate python-content-only-preservation-pointer "$GUARD_PY" \
  '                    "as this trigger. If you are instead trying to PRESERVE "' \
  '                    "as this trigger. If you are instead trying to sleep "'
mutate js-content-only-preservation-pointer "$GUARD_JS" \
  '        `this trigger. If you are instead trying to PRESERVE data by ` +' \
  '        `this trigger. If you are instead trying to sleep ` +'

# ── The shims, the only per-lane code left ─────────────────────────────────
# A shim that names the wrong lane guards the wrong trees; a shim that cannot
# reach the shared module guards nothing at all. Both must be loud.
# The mutated shim is a lane with a directory of its own (not the root), and
# the name it is swapped to is the next such lane — or, where the table has only
# one besides the root, the root itself. So the mutant names no lane of its own
# and still swaps one real shim for another. The fallback is not hypothetical:
# the exported table carries only `brain` and `setup`, and until 2026-09-29 this
# demanded two lanes besides both, so the export's suite aborted here before
# any mutant ran. `setup` is no longer excluded — the shim-import arm below
# mutates its JS shim in a separate copy, so the two arms do not collide.
read -r SHIM_LANE SHIM_OTHER <<<"$(node -e "import('$ROOT/setup/guards/deny-repo-writes.js').then(m => { const own = Object.keys(m.LANES).filter(l => l !== 'brain'); console.log(own[0] ?? '', own[1] ?? (own[0] ? 'brain' : '')); })")"
[ -n "$SHIM_OTHER" ] || { echo "shim-lane-name: the lane table has no lane besides brain" >&2; exit 1; }
mutate shim-lane-name "$SHIM_LANE/.claude/hooks/deny-repo-writes.py" \
  "LANE = \"$SHIM_LANE\"" "LANE = \"$SHIM_OTHER\""
mutate shim-import "setup/.opencode/plugin/deny-repo-writes.js" \
  '../../../setup/guards/deny-repo-writes.js' '../../../setup/guards/absent.js'

_mutate_report
