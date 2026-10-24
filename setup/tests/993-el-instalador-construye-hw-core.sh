#!/usr/bin/env bash
# THE INSTALLER BUILDS hw-core, AND REFUSES WHERE IT CANNOT (go-060 S2: hw-core is required).
#
# From 0.5.0 Go is the default of every tool and from 0.6.0 hw-core is not optional: hw, brain, channel-send and the invokers
# die without one, so an install that leaves none would leave a brain that cannot run. The installer is what makes both true:
#   - with a Go toolchain of 1.22 or newer it builds hw-core into <brain>/core/dist/<os>-<arch>/ (the file bin/hw looks
#     in), byte for byte what setup/build-core makes, and writes no switch file (Go needs none);
#   - without Go, with an older Go, or with a go that does not answer `go version`, it REFUSES before writing anything to the
#     brain — it exits 1, names the missing go >= 1.22 and https://go.dev/dl, and creates nothing (`--check` lists it among the
#     missing prerequisites and, like every `--check`, writes nothing);
#   - `--no-core` is refused, "removed in 0.6.0", before anything is written; FOREMAN_SH_NO_CORE=1 is no option of a user's but the
#     seam the suite's own installs use (_common.sh): it builds nothing, says so, and writes nothing;
#   - a build that FAILS refuses (exit 1, the builder's last lines), and so does a Go that builds for another architecture than
#     the one bin/hw looks in: both remove the hw-core an earlier install left, which would run against this release's bin/hw;
#   - with a core and no switch file, a ported verb runs in Go (the default), and HW_IMPL=bash or HW_IMPL_<VERB>=bash no longer goes back:
#     it says once that bash was removed in 0.6.0 and Go still runs (go-060 S3);
#   - an install that builds a core removes the `default=bash` file an old `--no-core` install wrote, byte for byte, and no
#     other: a switch file the user wrote is never touched;
#   - a second identical run changes no file;
#   - a new hw-core replaces the old FILE (a new inode), never a write into it: macOS keeps a binary's code signature
#     per file and kills an executable rewritten in place;
#   - the mutants 993-M01 (a switch file of the user's is removed, or the one --no-core wrote is kept), 993-M02 (an
#     old Go is accepted), 993-M03 (--no-core is accepted), 993-M04 (a stale hw-core is kept), 993-M05 (hw-core written in place) and 993-M06
#     (another architecture is installed) 993-M07 (a go that fails `go version` stops the install with no word) and 993-M08 (a retired switch line is not said) are each caught.
#
# Run by ../test-hw. Alone:  bash setup/tests/993-el-instalador-construye-hw-core.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR FOREMAN_SH_NO_CORE
# The claim is what the INSTALLED hw does with nothing set, so the runner's choice is not an input here.
unset HW_IMPL
command -v go >/dev/null 2>&1 || { pass "993: no Go toolchain on this machine — the build arms need one (the no-Go arm is held by 993's stub in a machine that has it)"; exit 0; }
[ -f "$ROOT/setup/build-core" ] && [ -d "$ROOT/core" ] || { pass "993: this tree carries no core/ to build"; exit 0; }
export GOCACHE="${GOCACHE:-$HW_GOCACHE_DEFAULT}"

REPO="$HOME/code/app"
( mkdir -p "$REPO" && cd "$REPO" && git init -q -b trunk && git config user.email t@t \
  && git config user.name t && echo hi > README.md && git add . && git commit -q -m base )
STUBS="$TMP/prereq-bin"; mkdir -p "$STUBS"
printf '#!/usr/bin/env bash\n[ "$1 $2" = "integration status" ] && { echo "claude: current (x)"; exit 0; }\nexit 0\n' > "$STUBS/herdr"
printf '#!/bin/sh\nexit 0\n' > "$STUBS/claude"
chmod +x "$STUBS/herdr" "$STUBS/claude"
REALPATH="$PATH"
export PATH="$HOME/.local/bin:$STUBS:$PATH"
os="$(uname -s | tr 'A-Z' 'a-z')"; arch="$(uname -m)"
case "$os" in mingw*|msys*|cygwin*) os=windows ;; esac
case "$arch" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
ext=""; [ "$os" != windows ] || ext=.exe
IMPL="$HOME/.config/hw/impl"
inst() { "$1/install.sh" --brain "$2" --lane app --repo "$REPO" "${@:3}" 2>&1; }
# one user, one brain: every arm reinstalls the same brain, from a state with no core and no switch file of the arm before
fresh() { rm -rf "$HOME/b/core" "$HOME/.config/hw"; }

# ── 1. with Go: the binary is built where hw looks, equal to the builder's, and no switch file is written ───────────
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the install with Go failed: $(printf '%s' "$out" | tail -6)"
BIN="$HOME/b/core/dist/$os-$arch/hw-core$ext"
[ -x "$BIN" ] || fail "993: no hw-core at $BIN after an install with Go: $(printf '%s' "$out" | tail -8)"
"$ROOT/setup/build-core" --native --out "$TMP/ref" >/dev/null 2>&1 || fail "993: build-core --native failed"
cmp -s "$BIN" "$TMP/ref/$os-$arch/hw-core$ext" || fail "993: the installed hw-core is not the builder's binary"
[ ! -e "$IMPL" ] || fail "993: the install wrote a switch file although Go is the default: $(cat "$IMPL" 2>&1)"
pass "993: with Go the installer builds the builder's hw-core into the brain's core/dist and writes no switch file"

# ── 2. with no switch file a ported verb runs in Go (the default), and the two retired switches do not go back ────────────────────
printf '#!/bin/sh\necho GO-RAN "$@"\n' > "$TMP/stubcore"; chmod +x "$TMP/stubcore"
g="$(HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1)"
case "$g" in *"GO-RAN handle zzz-993"*) ;; *) fail "993: with no switch file a ported verb did not run in hw-core: $g" ;; esac
b="$(HW_IMPL=bash HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1 || true)"
case "$b" in *"GO-RAN handle zzz-993"*) ;; *) fail "993: HW_IMPL=bash (retired) did not leave hw-core running: $b" ;; esac
case "$b" in *"removed in 0.6.0"*) ;; *) fail "993: HW_IMPL=bash (retired) was not said to be removed: $b" ;; esac
b="$(HW_IMPL_HANDLE=bash HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1 || true)"
case "$b" in *"GO-RAN handle zzz-993"*) ;; *) fail "993: HW_IMPL_HANDLE=bash (retired) did not leave hw-core running: $b" ;; esac
case "$b" in *"removed in 0.6.0"*) ;; *) fail "993: HW_IMPL_HANDLE=bash (retired) was not said to be removed: $b" ;; esac
pass "993: with no switch file a ported verb runs in Go; HW_IMPL=bash and HW_IMPL_<VERB>=bash say once that bash was removed and Go still runs"

# ── 3. a second run changes nothing; a switch file of the user's stays ───────────────────────────────────────────────
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the second install failed: $out"
case "$out" in *"  +     "*) fail "993: a second identical run changed something: $(printf '%s\n' "$out" | grep '^  +')" ;; esac
mkdir -p "$(dirname "$IMPL")"; printf 'bash\n' > "$IMPL"
out="$(inst "$ROOT" "$HOME/b")" || fail "993: an install over the user's switch file failed: $out"
[ "$(cat "$IMPL")" = bash ] || fail "993: the install rewrote a switch file the user had: $(cat "$IMPL")"
case "$out" in *"stays as it is"*) ;; *) fail "993: the install did not say it left the user's switch file alone: $out" ;; esac
pass "993: a second run writes nothing, and a switch file that exists is never rewritten"

# `--no-core` is refused, whatever the brain holds: nothing is written, the hw-core of the earlier install and the user's switch file stay
[ -x "$BIN" ] || fail "993: the brain holds no hw-core to be left alone"
rc=0; out="$(inst "$ROOT" "$HOME/b" --no-core)" || rc=$?
[ "$rc" = 1 ] || fail "993: --no-core exited $rc, want 1: $out"
case "$out" in *"--no-core was removed in 0.6.0"*"https://go.dev/dl"*) ;; *) fail "993: --no-core was not refused as removed in 0.6.0, naming where Go comes from: $out" ;; esac
[ -x "$BIN" ] || fail "993: a refused --no-core removed the hw-core of the earlier install"
[ "$(cat "$IMPL")" = bash ] || fail "993: a refused --no-core rewrote a switch file the user had: $(cat "$IMPL")"
pass "993: --no-core is refused (removed in 0.6.0), names https://go.dev/dl, and touches nothing"

# an old `--no-core` install wrote `default=bash`, which kept every tool on bash until 0.6.0 (now it only says once that bash was removed and
# Go runs); an install that builds a core removes exactly that file
# (it would keep every tool on bash for a user who has since installed Go), never another
OLDTEXT='# written by install.sh --no-core: every tool runs its bash implementation. Delete this file to run Go (the default)
default=bash'
printf '%s\n' "$OLDTEXT" > "$IMPL"
g="$(HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1 || true)"
case "$g" in *"GO-RAN handle zzz-993"*) ;; *) fail "993: the default=bash file of an old --no-core install stopped hw-core from running (it is retired: Go runs): $g" ;; esac
case "$g" in *"removed in 0.6.0"*) ;; *) fail "993: the default=bash file of an old --no-core install was not said to be retired: $g" ;; esac
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the install with Go over an old --no-core file failed: $out"
[ -x "$BIN" ] && [ ! -e "$IMPL" ] || fail "993: an install that builds a core left the default=bash file an old --no-core install wrote (core: $([ -x "$BIN" ] && echo built || echo none), file: $([ -e "$IMPL" ] && echo there || echo gone))"
case "$out" in *"the one --no-core wrote is removed"*) ;; *) fail "993: the removal of the old --no-core file was not said: $out" ;; esac
printf 'default=bash # mine\n' > "$IMPL"
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the install over a hand-written default=bash failed: $out"
[ "$(cat "$IMPL")" = 'default=bash # mine' ] || fail "993: an install that builds a core removed or rewrote a switch file that is not the one --no-core wrote: $(cat "$IMPL" 2>&1)"
pass "993: an install that builds a core removes the default=bash file an old --no-core install wrote, byte for byte, and no other"

# a file of the user's that still has lines of the retired switch is SAID at install time, line by line, and left as it is: a default or one of the
# five tools means nothing in 0.6.0, a `<tool>=bash` of the eight still does, and a `go` line is quiet
printf 'default=bash\nhw-secret=bash\nstop-hook=zig\nbrain=bash\nhw=go\n' > "$IMPL"; IMPLWAS="$(cat "$IMPL")"
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the install over a switch file with retired lines failed: $out"
case "$out" in *"line 1 (default=bash) means nothing since 0.6.0"*) ;; *) fail "993: a default=bash line was not said to mean nothing: $out" ;; esac
case "$out" in *"line 4 (brain=bash) means nothing since 0.6.0"*) ;; *) fail "993: a brain=bash line was not said to mean nothing: $out" ;; esac
case "$out" in *"line 2 (hw-secret=bash) still runs that tool on its bash"*) ;; *) fail "993: a hw-secret=bash line was not said to still mean something: $out" ;; esac
case "$out" in *"line 3 (stop-hook=zig) means nothing: only the word bash"*) ;; *) fail "993: a stop-hook=zig line was not said to mean nothing: $out" ;; esac
case "$out" in *"line 5"*) fail "993: a hw=go line was said something about: $out" ;; esac
[ "$(cat "$IMPL")" = "$IMPLWAS" ] || fail "993: the install rewrote a switch file that has retired lines: $(cat "$IMPL" 2>&1)"
pass "993: an install over a switch file says each retired line, and each tool=bash line that still means something, and leaves the file"

# ── 4. no Go, an older Go, a go that says nothing, --no-core: the install REFUSES before writing anything ───────────
# refuses <label> <expected text> <env…> -- the install is run against a brain that does not exist: it exits 1, says why, and creates nothing
refuses() {
  local label="$1" want="$2" path="$3" rc=0 out; shift 3
  rm -rf "$HOME/nb" "$HOME/.config/hw"
  out="$(PATH="$path" inst "$ROOT" "$HOME/nb" "$@")" || rc=$?
  [ "$rc" = 1 ] || fail "993: $label exited $rc, want 1 (a refusal): $(printf '%s' "$out" | tail -6)"
  case "$out" in *"$want"*) ;; *) fail "993: $label did not say «$want»: $out" ;; esac
  case "$out" in *"nothing was written"*|*"--no-core was removed"*) ;; *) fail "993: $label did not say nothing was written: $out" ;; esac
  [ ! -e "$HOME/nb" ] && [ ! -e "$IMPL" ] || fail "993: $label wrote something before refusing: $(fd . "$HOME/nb" 2>/dev/null | head -5)"
}
GODIRS="$(for d in $(printf '%s' "$REALPATH" | tr ':' ' '); do [ ! -x "$d/go" ] || printf '%s ' "$d"; done)"
NOGO="$TMP/nogo-bin"; mkdir -p "$NOGO"
for d in $GODIRS; do for f in "$d"/*; do n="$(basename "$f")"; case "$n" in go|gofmt) ;; *) [ -e "$NOGO/$n" ] || ln -s "$f" "$NOGO/$n" ;; esac; done; done
PATH_NOGO="$HOME/.local/bin:$STUBS:$NOGO"
for d in $(printf '%s' "$REALPATH" | tr ':' ' '); do
  case " $GODIRS " in *" $d "*) continue ;; esac
  PATH_NOGO="$PATH_NOGO:$d"
done
refuses "an install with no go on PATH" "MISSING go >= 1.22 — no Go toolchain on PATH" "$PATH_NOGO"
refuses "an install with no go on PATH" "https://go.dev/dl" "$PATH_NOGO"
# --check lists it, in the way of every missing prerequisite, and writes nothing
rm -rf "$HOME/nb"; out="$(PATH="$PATH_NOGO" inst "$ROOT" "$HOME/nb" --check)" && rc=0 || rc=$?
case "$out" in *"MISSING go >= 1.22"*"https://go.dev/dl"*) ;; *) fail "993: --check with no go did not list it among the missing prerequisites: $out" ;; esac
[ ! -e "$HOME/nb" ] || fail "993: --check wrote a brain"
OLDGO="$TMP/oldgo-bin"; mkdir -p "$OLDGO"
printf '#!/bin/sh\necho "$@" >> "%s/go.calls"\n[ "$1" = version ] && { echo "go version go1.21.5 linux/amd64"; exit 0; }\nexit 1\n' "$TMP" > "$OLDGO/go"; chmod +x "$OLDGO/go"
: > "$TMP/go.calls"
refuses "an install with an old Go" "Go 1.21 is older than 1.22" "$OLDGO:$PATH"
[ "$(grep -vc '^version' "$TMP/go.calls")" = 0 ] || fail "993: the installer ran an old go to build: $(cat "$TMP/go.calls")"
BADGO="$TMP/badgo-bin"; mkdir -p "$BADGO"
printf '#!/bin/sh\nexit 3\n' > "$BADGO/go"; chmod +x "$BADGO/go"
refuses "an install with a go that does not answer 'go version'" "Go of an unreadable version is older than 1.22" "$BADGO:$PATH"
refuses "--no-core" "--no-core was removed in 0.6.0" "$PATH" --no-core
# a refusal leaves the hw-core an earlier install left, as it leaves everything: nothing was written
fresh; inst "$ROOT" "$HOME/b" >/dev/null || fail "993: the install with Go failed"
[ -x "$BIN" ] || fail "993: no hw-core to be left alone"
out="$(PATH="$PATH_NOGO" inst "$ROOT" "$HOME/b")" && fail "993: an install with no go over a brain with a core was accepted"
[ -x "$BIN" ] || fail "993: a refused install removed the hw-core of the earlier one"
pass "993: no Go, an older Go, a go that says nothing and --no-core each refuse with status 1 before writing anything (a brain that does not exist is not created, one that does is left as it was), and --check lists the missing go"

# FOREMAN_SH_NO_CORE=1, the suite's seam: builds nothing, says so, writes no switch file, exits 0
fresh; out="$(FOREMAN_SH_NO_CORE=1 inst "$ROOT" "$HOME/b")" || fail "993: the seam install failed: $out"
case "$out" in *"hw-core is not built (FOREMAN_SH_NO_CORE=1"*"setup/build-core --native"*) ;; *) fail "993: the seam did not say hw-core is not built, and how to build it: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: the seam built a hw-core or wrote a switch file"
pass "993: FOREMAN_SH_NO_CORE=1, the suite's seam, builds nothing, says so, and writes no switch file"

# ── mutants of install.sh, in a source tree of its own (it installs what lies beside it), with a builder that is a stub ──
MS="$TMP/mutant-src"; mkdir -p "$MS/lanes" "$MS/setup" "$MS/core"
cp -R "$ROOT/bin" "$ROOT/layouts" "$ROOT/lib" "$MS/"; cp "$ROOT/lanes/git-worktree.sh" "$MS/lanes/"
cp -R "$ROOT/setup/guards" "$MS/setup/"; cp "$ROOT/setup/brain-guard-programs.txt" "$MS/setup/"
cat > "$MS/setup/build-core" <<EOF
#!/usr/bin/env bash
out=""; while [ \$# -gt 0 ]; do case "\$1" in --out) out="\$2"; shift ;; esac; shift; done
[ -z "\${STUB_FAIL:-}" ] || { echo "stub: the build broke" >&2; exit 1; }
d="\${STUB_DIR:-$os-$arch}"
mkdir -p "\$out/\$d"; printf '%s\\n' "\${STUB_BODY:-stub hw-core}" > "\$out/\$d/hw-core$ext"; chmod +x "\$out/\$d/hw-core$ext"
EOF
chmod +x "$MS/setup/build-core"
fresh_src() { cp "$ROOT/install.sh" "$MS/install.sh"; chmod +x "$MS/install.sh"; }
parses() { bash -n "$MS/install.sh" || fail "993: mutant $1 does not parse"; }

# the stub builder, as real arms: a new hw-core is a new file, and another architecture installs nothing
fresh; fresh_src
STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || fail "993: the stub install failed"
CORE="$HOME/b/core/dist/$os-$arch/hw-core$ext"; [ -x "$CORE" ] || fail "993: the stub install left no hw-core"
i1="$(ls -i "$CORE" | awk '{print $1}')"
STUB_BODY=two inst "$MS" "$HOME/b" >/dev/null || fail "993: the stub reinstall failed"
i2="$(ls -i "$CORE" | awk '{print $1}')"
[ "$(cat "$CORE")" = two ] && [ "$i1" != "$i2" ] || fail "993: a new hw-core was written into the old file (inode $i1 -> $i2, content $(cat "$CORE"))"
pass "993: a new hw-core replaces the old file instead of being written into it"
fresh; fresh_src; STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || fail "993: the stub install failed"
rc=0; out="$(STUB_DIR=plan9-mips inst "$MS" "$HOME/b")" || rc=$?
[ "$rc" = 1 ] || fail "993: an install with a Go for another architecture exited $rc, want 1: $out"
case "$out" in *"builds for plan9-mips"*"where hw looks for hw-core"*"hw-core is required and your Go builds for another architecture"*) ;; *) fail "993: a Go for another architecture was not refused, naming it: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: a hw-core for another architecture was installed, the earlier one kept, or the switch file written"
case "$out" in *"hw-core of an earlier install is removed"*) ;; *) fail "993: the removal of the earlier hw-core was not said: $out" ;; esac
pass "993: a Go that builds for another architecture refuses (status 1), installs nothing, says where hw looks, and removes the hw-core of the earlier install"
# a build that FAILS refuses too, with the builder's words, and leaves no old hw-core to run against this release
fresh; fresh_src; STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || fail "993: the stub install failed"
rc=0; out="$(STUB_FAIL=1 inst "$MS" "$HOME/b")" || rc=$?
[ "$rc" = 1 ] || fail "993: an install whose build failed exited $rc, want 1: $out"
case "$out" in *"hw-core did not build"*"the build broke"*"hw-core is required and did not build"*) ;; *) fail "993: a failed build was not refused with the builder's words: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: after a failed build the earlier hw-core is still there, or a switch file was written"
case "$out" in *"hw-core of an earlier install is removed"*) ;; *) fail "993: the removal of the earlier hw-core was not said: $out" ;; esac
pass "993: a build that fails refuses (status 1) with the builder's words and removes the hw-core of the earlier install"

# a copy that fails stops the install: it is not reported as written, and no switch file follows
fresh; fresh_src; STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || fail "993: the stub install failed"
chmod 555 "$HOME/b/core/dist/$os-$arch"; rm -f "$IMPL"
rcp=0; STUB_BODY=two inst "$MS" "$HOME/b" >"$TMP/cpfail.out" 2>&1 || rcp=$?
chmod 755 "$HOME/b/core/dist/$os-$arch"
if [ "$(id -u)" = 0 ]; then pass "993: a failed copy is not testable as root (a read-only directory does not stop root)"
else
  [ "$rcp" != 0 ] || fail "993: a hw-core that could not be copied was reported as installed: $(tail -3 "$TMP/cpfail.out")"
  [ ! -e "$IMPL" ] || fail "993: the switch file was written after a copy that failed"
  pass "993: a hw-core that cannot be copied stops the install, and no switch file follows"
fi

# M01: a switch file of the user's is removed by an install that builds a core ...
fresh; mkdir -p "$HOME/.config/hw"; printf 'bash\n' > "$IMPL"
fresh_src; mutate_anchor 993-M01 "$MS/install.sh" 'if [ -e "$_implf" ]; then rm -f "$_implf"; chg "$_implf — removed"'; parses 993-M01
inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M01 a switch file of the user's removed" "switch file: $([ -e "$IMPL" ] && echo kept || echo gone)" "switch file: gone"
# ... and the one an old --no-core install wrote is kept by it
fresh; fresh_src; mkdir -p "$HOME/.config/hw"; printf '%s\n' "$OLDTEXT" > "$IMPL"
mutate_anchor 993-M01 "$MS/install.sh" 'if false; then :'; parses 993-M01
inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M01 the --no-core switch file kept by an install with a core" "switch file: $([ -e "$IMPL" ] && echo kept || echo gone)" "switch file: kept"

# M08: a line of the retired switch is not said at install time
fresh; fresh_src; mkdir -p "$HOME/.config/hw"; printf 'default=bash\n' > "$IMPL"
mutate_anchor 993-M08 "$MS/install.sh" '          *) :;;'; parses 993-M08
m8out="$(inst "$MS" "$HOME/b" 2>&1)" || true
m8said=silent; case "$m8out" in *"means nothing since 0.6.0"*) m8said=said ;; esac
saw_mutant "M08 a retired switch line not said at install" "message: $m8said" "message: silent"

# M02: an old Go is accepted
fresh; : > "$TMP/go.calls"
fresh_src; mutate_anchor 993-M02 "$MS/install.sh" 'if [ -n "$_gov" ]; then ok "go $_gov (builds hw-core)"; CORE_BUILD=1'; parses 993-M02
PATH="$OLDGO:$PATH" inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M02 an old Go accepted" "core built: $([ -e "$HOME/b/core/dist/$os-$arch/hw-core$ext" ] && echo yes || echo no)" "core built: yes"

# M03: --no-core is accepted
fresh
fresh_src; mutate_anchor 993-M03 "$MS/install.sh" '--no-core) NO_CORE=1; shift ;;'; parses 993-M03
m3rc=0; inst "$MS" "$HOME/b" --no-core >/dev/null 2>&1 || m3rc=$?
saw_mutant "M03 --no-core accepted" "install exit: $([ "$m3rc" = 0 ] && echo 0 || echo nonzero)" "install exit: 0"

# M04: a stale hw-core is kept (a run that builds none — the suite's seam here — must remove the one an earlier install left)
fresh; fresh_src; STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || true
mutate_anchor 993-M04 "$MS/install.sh" ':'; parses 993-M04
FOREMAN_SH_NO_CORE=1 inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M04 a stale hw-core kept" "core after a run that builds none: $([ -e "$HOME/b/core/dist" ] && echo kept || echo removed)" "core after a run that builds none: kept"

# M05: hw-core written in place
fresh; fresh_src
mutate_anchor 993-M05 "$MS/install.sh" 'else cp -p "$1" "$2"; chg "${2#"$BRAIN"/}"; fi'; parses 993-M05
STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || true
j1="$(ls -i "$HOME/b/core/dist/$os-$arch/hw-core$ext" | awk '{print $1}')"
STUB_BODY=two inst "$MS" "$HOME/b" >/dev/null || true
j2="$(ls -i "$HOME/b/core/dist/$os-$arch/hw-core$ext" | awk '{print $1}')"
saw_mutant "M05 hw-core written in place" "inode: $([ "$j1" = "$j2" ] && echo same || echo new)" "inode: same"

# M06: another architecture is installed
fresh; fresh_src; mutate_anchor 993-M06 "$MS/install.sh" 'elif false; then'; parses 993-M06
STUB_DIR=plan9-mips inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M06 a hw-core for another architecture installed" "plan9: $([ -e "$HOME/b/core/dist/plan9-mips" ] && echo installed || echo none)" "plan9: installed"

# M07: a go that does not answer `go version` stops the install with no word (the refusal names it instead)
fresh; fresh_src; mutate_anchor 993-M07 "$MS/install.sh" '  _gov="$(go version 2>/dev/null | sed -n '"'"'s/^go version go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p'"'"')"'; parses 993-M07
rm -rf "$HOME/nb"; m7out="$(PATH="$BADGO:$PATH" inst "$MS" "$HOME/nb" 2>&1)" || true
m7said=silent; case "$m7out" in *"unreadable version"*) m7said=said ;; esac
saw_mutant "M07 a go that fails stops the install with no word" "message: $m7said" "message: silent"
