#!/usr/bin/env bash
# pi's write guard (setup/briefs/pi-vendor-v1.md, piece 4): `tool_call` blocks
# bash/edit/write into a protected tree with the SAME detection as the opencode
# half (setup/guards/deny-repo-writes.js: `decide`, `insideProtected`), driven
# with pi's payload shapes and the real guards.json. And in an executor the raw
# engram server is refused in favour of the label proxy (hw_engram).
# OLD: no adapter — pi runs `echo x > <product repo>/…` unasked (live, in
# docs/pi-vendor.md: the redirect reached the shell).
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

ADAPTER="${PI_ADAPTER_SOURCE:-$ROOT/setup/pi/hw-executor.ts}"
DRIVER="$ROOT/setup/pi/test-hw-executor.mjs"
command -v node >/dev/null 2>&1 || fail "node is required to drive the pi adapter"
[ -r "$ADAPTER" ] || fail "no pi adapter at $ADAPTER — a pi executor's writes are unguarded"

for env_set in "plain" "executor"; do
  if [ "$env_set" = executor ]; then set -- HW_TASK=t HW_RUN=r HW_WORKDIR="$TMP" ENGRAM_PROJECT=brain; else set --; fi
  out="$(env "$@" HW_PROJECT=setup HW_BRAIN_ROOT="$ROOT" \
    node --experimental-strip-types --no-warnings "$DRIVER" "$ADAPTER" guard 2>&1)" \
    || fail "the pi write guard did not behave ($env_set): $out"
  printf '%s\n' "$out" | while IFS= read -r l; do case "$l" in "ok - "*) pass "$env_set: ${l#ok - }" ;; esac; done
done

# A guard that cannot load its module refuses every bash/edit/write.
out="$(HW_PROJECT=setup HW_BRAIN_ROOT="$TMP/nowhere" node --experimental-strip-types --no-warnings --input-type=module -e '
  const m = await import(process.argv[1]);
  console.log(await m.judge("bash", { command: "ls" }, "/tmp"));' "$ADAPTER" 2>&1)"
case "$out" in *"could not load"*) pass "a guard module that does not load blocks even a harmless bash call" ;; *) fail "a broken guard module let bash through: $out" ;; esac

# A PRODUCT EXECUTOR WORKS INSIDE ITS LANE'S PROTECTED TREE (Judgment Day, both
# judges). Its own worktree is writable; the lane's main checkout is not.
# OLD (6ee01860): the adapter refused every write of a product-lane executor.
pw="$(eval echo "~$(id -un)")"
# The lane is a FIXTURE this test builds, not a row read from the real guards.json:
# the exported guards.json is sanitised and carries no lane whose worktrees live
# inside its checkout, so reading the real lanes made this case pass here and fail
# in the export. The fixture is the real policy plus one lane of that shape, with
# the real guard module copied beside it (the module finds guards.json relative to
# its own path).
FX="$TMP/fxbrain"; lane=fxlane
mkdir -p "$FX/setup/guards"
cp "$ROOT/setup/guards/deny-repo-writes.js" "$FX/setup/guards/"
jq '.lanes.fxlane = (.lanes.setup | .repo = "~/fx-lane/app" | .worktrees = "~/fx-lane/app/.worktrees")
    | .product_repos += ["~/fx-lane/app"]' "$ROOT/guards.json" > "$FX/guards.json" \
  || fail "could not build the fixture lane from $ROOT/guards.json"
repo="$pw/fx-lane/app"; wts="$pw/fx-lane/app/.worktrees"
out="$(env HW_TASK=t HW_RUN=r HW_WORKDIR="$wts/pi-guard-test-wt" HW_PROJECT="$lane" OTHER_PROTECTED="$repo" \
  ENGRAM_PROJECT="$lane" HW_BRAIN_ROOT="$FX" \
  node --experimental-strip-types --no-warnings "$DRIVER" "$ADAPTER" product-executor 2>&1)" \
  || fail "a product-lane pi executor is mis-guarded ($lane): $out"
printf '%s\n' "$out" | while IFS= read -r l; do case "$l" in "ok - "*) pass "product lane: ${l#ok - }" ;; esac; done
