#!/usr/bin/env bash
# THE INSTALLER BUILDS hw-core WHEN IT CAN, MAKES THE PORTED VERBS RUN IN IT, AND SAYS WHAT IT DID NOT DO.
#
# 0.4.0-rc.1 ships the Go half of hw (core/, built by setup/build-core). A user who installs it gets the ported verbs
# in Go by default and the bash implementation as the way back; the installer is what makes both true:
#   - with a Go toolchain of 1.22 or newer it builds hw-core into <brain>/core/dist/<os>-<arch>/ (the file bin/hw looks
#     in), byte for byte what setup/build-core makes, and writes the switch file `go` — only when the user has none;
#   - without Go, or with an older Go, it builds nothing, says so in one line, exits 0, writes no switch file, and hw
#     runs bash (the same answers);
#   - `--no-core` (and FOREMAN_SH_NO_CORE=1) builds nothing even where Go is;
#   - with the switch file and a core, a ported verb runs in Go, and HW_IMPL=bash or HW_IMPL_<VERB>=bash goes back;
#   - a second identical run changes no file, and a switch file the user wrote is never touched;
#   - a run that leaves no fresh hw-core (--no-core, no Go, an old Go, a failed build, a Go for another architecture)
#     removes the one an earlier install left: an installed brain has no core/ sources, so bin/hw could never tell it
#     is old and would run a Go of another release against this release's bin/hw;
#   - a new hw-core replaces the old FILE (a new inode), never a write into it: macOS keeps a binary's code signature
#     per file and kills an executable rewritten in place;
#   - a Go that builds for an architecture other than the one bin/hw looks in installs nothing and writes no switch file;
#   - the mutants 993-M01 (an existing switch file is overwritten), 993-M02 (an old Go is accepted), 993-M03 (no
#     switch file is written), 993-M04 (a stale hw-core is kept), 993-M05 (hw-core written in place) and 993-M06
#     (another architecture is installed) and 993-M07 (a go that fails `go version` stops the install) are each caught.
#
# Run by ../test-hw. Alone:  bash setup/tests/993-el-instalador-construye-hw-core.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR FOREMAN_SH_NO_CORE
command -v go >/dev/null 2>&1 || { pass "993: no Go toolchain on this machine — the build arms need one (the no-Go arm is held by 993's stub in a machine that has it)"; exit 0; }
[ -f "$ROOT/setup/build-core" ] && [ -d "$ROOT/core" ] || { pass "993: this tree carries no core/ to build"; exit 0; }
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/hw-go-cache}"

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

# ── 1. with Go: the binary is built where hw looks, equal to the builder's, and the switch file says go ──────────────
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the install with Go failed: $(printf '%s' "$out" | tail -6)"
BIN="$HOME/b/core/dist/$os-$arch/hw-core$ext"
[ -x "$BIN" ] || fail "993: no hw-core at $BIN after an install with Go: $(printf '%s' "$out" | tail -8)"
"$ROOT/setup/build-core" --native --out "$TMP/ref" >/dev/null 2>&1 || fail "993: build-core --native failed"
cmp -s "$BIN" "$TMP/ref/$os-$arch/hw-core$ext" || fail "993: the installed hw-core is not the builder's binary"
[ "$(grep -v '^#' "$IMPL" | tr -d '[:space:]')" = go ] || fail "993: the switch file does not say go: $(cat "$IMPL" 2>&1)"
case "$out" in *"$IMPL"*"run in Go"*) ;; *) fail "993: the install did not say it moved the ported verbs to Go: $out" ;; esac
pass "993: with Go the installer builds the builder's hw-core into the brain's core/dist and writes the switch file"

# ── 2. the switch file makes a ported verb run in Go, and the two switches go back ───────────────────────────────────
printf '#!/bin/sh\necho GO-RAN "$@"\n' > "$TMP/stubcore"; chmod +x "$TMP/stubcore"
g="$(HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1)"
case "$g" in *"GO-RAN handle zzz-993"*) ;; *) fail "993: with the switch file at go a ported verb did not run in hw-core: $g" ;; esac
b="$(HW_IMPL=bash HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1 || true)"
case "$b" in *GO-RAN*) fail "993: HW_IMPL=bash still ran hw-core: $b" ;; esac
b="$(HW_IMPL_HANDLE=bash HW_IMPL_FILE="$IMPL" HW_CORE="$TMP/stubcore" "$HOME/b/bin/hw" handle zzz-993 2>&1 || true)"
case "$b" in *GO-RAN*) fail "993: HW_IMPL_HANDLE=bash still ran hw-core: $b" ;; esac
pass "993: the switch file runs a ported verb in Go; HW_IMPL=bash and HW_IMPL_<VERB>=bash go back"

# ── 3. a second run changes nothing; a switch file of the user's stays ───────────────────────────────────────────────
out="$(inst "$ROOT" "$HOME/b")" || fail "993: the second install failed: $out"
case "$out" in *"  +     "*) fail "993: a second identical run changed something: $(printf '%s\n' "$out" | grep '^  +')" ;; esac
printf 'bash\n' > "$IMPL"
out="$(inst "$ROOT" "$HOME/b")" || fail "993: an install over the user's switch file failed: $out"
[ "$(cat "$IMPL")" = bash ] || fail "993: the install rewrote a switch file the user had: $(cat "$IMPL")"
case "$out" in *"stays as it is"*) ;; *) fail "993: the install did not say it left the user's switch file alone: $out" ;; esac
pass "993: a second run writes nothing, and a switch file that exists is never rewritten"

# the brain now holds a hw-core of this install: a run that builds none removes it, and says so
[ -x "$BIN" ] || fail "993: the brain holds no hw-core to be found stale"
out="$(inst "$ROOT" "$HOME/b" --no-core)" || fail "993: --no-core over a brain with a core failed: $out"
[ ! -e "$HOME/b/core/dist" ] || fail "993: --no-core left the hw-core of the earlier install: it would keep running in Go"
case "$out" in *"hw-core of an earlier install is removed"*) ;; *) fail "993: the removal of the old hw-core was not said: $out" ;; esac
pass "993: a run that builds no hw-core removes the one an earlier install left"

# ── 4. no Go, an older Go, and --no-core: nothing built, one line, exit 0, no switch file ────────────────────────────
fresh
GODIRS="$(for d in $(printf '%s' "$REALPATH" | tr ':' ' '); do [ ! -x "$d/go" ] || printf '%s ' "$d"; done)"
NOGO="$TMP/nogo-bin"; mkdir -p "$NOGO"
for d in $GODIRS; do for f in "$d"/*; do n="$(basename "$f")"; case "$n" in go|gofmt) ;; *) [ -e "$NOGO/$n" ] || ln -s "$f" "$NOGO/$n" ;; esac; done; done
PATH_NOGO="$HOME/.local/bin:$STUBS:$NOGO"
for d in $(printf '%s' "$REALPATH" | tr ':' ' '); do
  case " $GODIRS " in *" $d "*) continue ;; esac
  PATH_NOGO="$PATH_NOGO:$d"
done
out="$(PATH="$PATH_NOGO" inst "$ROOT" "$HOME/b")" || fail "993: the install without Go failed: $(printf '%s' "$out" | tail -6)"
case "$out" in *"no Go toolchain on PATH: hw-core is not built and hw runs its bash implementation"*) ;; *) fail "993: no line said hw-core was not built for lack of Go: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: an install without Go left a core or a switch file behind"
"$HOME/b/bin/hw" --version >/dev/null 2>&1 || fail "993: hw does not run in a brain installed without Go"
pass "993: without Go the installer builds nothing, says so in one line, writes no switch file, and hw runs"

fresh; OLDGO="$TMP/oldgo-bin"; mkdir -p "$OLDGO"
printf '#!/bin/sh\necho "$@" >> "%s/go.calls"\n[ "$1" = version ] && { echo "go version go1.21.5 linux/amd64"; exit 0; }\nexit 1\n' "$TMP" > "$OLDGO/go"; chmod +x "$OLDGO/go"
out="$(PATH="$OLDGO:$PATH" inst "$ROOT" "$HOME/b")" || fail "993: the install with an old Go failed: $out"
case "$out" in *"Go 1.21 is older than 1.22"*) ;; *) fail "993: an old Go was not named: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: an old Go built something or wrote the switch file"
[ "$(grep -vc '^version' "$TMP/go.calls")" = 0 ] || fail "993: the installer ran an old go to build: $(cat "$TMP/go.calls")"
fresh; BADGO="$TMP/badgo-bin"; mkdir -p "$BADGO"
printf '#!/bin/sh\nexit 3\n' > "$BADGO/go"; chmod +x "$BADGO/go"
out="$(PATH="$BADGO:$PATH" inst "$ROOT" "$HOME/b")" || fail "993: a go that does not answer 'go version' stopped the install (it is optional): $(printf '%s' "$out" | tail -4)"
case "$out" in *"of an unreadable version is older than 1.22"*) ;; *) fail "993: a go with no version was not named: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: a go with no version built something or wrote the switch file"
fresh; out="$(inst "$ROOT" "$HOME/b" --no-core)" || fail "993: --no-core failed: $out"
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: --no-core built or wrote the switch file"
pass "993: an older Go is named and builds nothing, and --no-core builds nothing even where Go is"

# ── mutants of install.sh, in a source tree of its own (it installs what lies beside it), with a builder that is a stub ──
MS="$TMP/mutant-src"; mkdir -p "$MS/lanes" "$MS/setup" "$MS/core"
cp -R "$ROOT/bin" "$ROOT/layouts" "$ROOT/lib" "$MS/"; cp "$ROOT/lanes/git-worktree.sh" "$MS/lanes/"
cp -R "$ROOT/setup/guards" "$MS/setup/"; cp "$ROOT/setup/brain-guard-programs.txt" "$MS/setup/"
cat > "$MS/setup/build-core" <<EOF
#!/usr/bin/env bash
out=""; while [ \$# -gt 0 ]; do case "\$1" in --out) out="\$2"; shift ;; esac; shift; done
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
fresh
out="$(STUB_DIR=plan9-mips inst "$MS" "$HOME/b")" || fail "993: an install with a Go for another architecture failed: $out"
case "$out" in *"builds for plan9-mips"*"where hw looks for hw-core"*) ;; *) fail "993: a Go for another architecture was not named: $out" ;; esac
[ ! -e "$HOME/b/core/dist" ] && [ ! -e "$IMPL" ] || fail "993: a hw-core for another architecture was installed or the switch file written"
pass "993: a Go that builds for another architecture installs nothing and says where hw looks"

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

# M01: the user's switch file is overwritten
fresh; mkdir -p "$HOME/.config/hw"; printf 'bash\n' > "$IMPL"
fresh_src; mutate_anchor 993-M01 "$MS/install.sh" 'if false; then :'; parses 993-M01
inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M01 a switch file of the user's overwritten" "switch file: $(grep -v '^#' "$IMPL" | tr -d '[:space:]')" "switch file: go"

# M02: an old Go is accepted
fresh; : > "$TMP/go.calls"
fresh_src; mutate_anchor 993-M02 "$MS/install.sh" 'if [ -n "$_gov" ]; then ok "go $_gov (builds hw-core)"; CORE_BUILD=1'; parses 993-M02
PATH="$OLDGO:$PATH" inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M02 an old Go accepted" "core built: $([ -e "$HOME/b/core/dist/$os-$arch/hw-core$ext" ] && echo yes || echo no)" "core built: yes"

# M03: no switch file is written
fresh
fresh_src; mutate_anchor 993-M03 "$MS/install.sh" ':'; parses 993-M03
inst "$MS" "$HOME/b" >/dev/null || true
saw_mutant "M03 no switch file written" "switch file: $([ -e "$IMPL" ] && echo yes || echo no)" "switch file: no"

# M04: a stale hw-core is kept
fresh; fresh_src; STUB_BODY=one inst "$MS" "$HOME/b" >/dev/null || true
mutate_anchor 993-M04 "$MS/install.sh" ':'; parses 993-M04
inst "$MS" "$HOME/b" --no-core >/dev/null || true
saw_mutant "M04 a stale hw-core kept" "core after --no-core: $([ -e "$HOME/b/core/dist" ] && echo kept || echo removed)" "core after --no-core: kept"

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

# M07: a go that does not answer `go version` stops the install
fresh; fresh_src; mutate_anchor 993-M07 "$MS/install.sh" '  _gov="$(go version 2>/dev/null | sed -n '"'"'s/^go version go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p'"'"')"'; parses 993-M07
m7rc=0; PATH="$BADGO:$PATH" inst "$MS" "$HOME/b" >/dev/null 2>&1 || m7rc=$?
saw_mutant "M07 an optional go that fails stops the install" "install exit: $([ "$m7rc" = 0 ] && echo 0 || echo nonzero)" "install exit: nonzero"
