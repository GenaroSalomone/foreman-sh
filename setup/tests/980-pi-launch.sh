#!/usr/bin/env bash
# pi is an hw vendor: `--agent pi` launches with the opencode-go roster by kind,
# none-mode only, its extension set explicit, and the account NOT APPLIED.
# setup/briefs/pi-vendor-v1.md, piece 1. OLD (main before this branch) dies
# with "--agent must be claude, opencode or codex"; NEW reaches the manifest.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SOURCE="${HW_SOURCE:-$ROOT/bin/hw}"
launch() {  # <task> <extra args...>
  local t="$1"; shift
  env -u HW_INVOKER_PANE -u HW_CHAINING_ENABLED "$SOURCE" setup "$t" --agent pi "$@" --no-report --dry-run 2>&1 || true
}
brief() {  # <kind> -> path of a brief declaring it
  local f="$TMP/pi-$1.md"
  printf -- '---\nkind: %s\nbase: main\nrequested_by: test\nasked: build\n---\n\n# pi %s\n\nbody\n' "$1" "$1" > "$f"
  printf '%s' "$f"
}

out="$(launch pi-none --sdd none --no-brief --model opencode-go/deepseek-v4.1-flash)"
case "$out" in *"dry run — nothing created"*) pass "--agent pi --sdd none reaches the dry-run manifest" ;; *) fail "--agent pi was not accepted: $out" ;; esac
case "$out" in *"--agent must be"*) fail "--agent pi refused as an unknown vendor: $out" ;; esac
case "$out" in *"--model opencode-go/deepseek-v4.1-flash"*) pass "the pi model rides as --model provider/id" ;; *) fail "no --model provider/id in the agent args: $out" ;; esac
case "$out" in *"-ne -na -e builtin:mcp"*"setup/pi/hw-executor.ts"*) pass "pi launches with an explicit extension set ending in hw's adapter" ;; *) fail "pi's extension set is missing: $out" ;; esac
case "$out" in *"setup/pi/hw-executor.ts --exclude-tools mcp__engram__* -ns"*) pass "the raw engram server's tools are excluded at launch (--exclude-tools mcp__engram__*), after the extension set and before the skills" ;; *) fail "no --exclude-tools mcp__engram__* in pi's launch: $out" ;; esac
case "$out" in *"hw_engram"*) pass "the manifest names the engram proxy as the pi extension's hw_engram" ;; *) fail "no hw_engram proxy line: $out" ;; esac
case "$out" in *"pi has no permission prompts"*) pass "the permissions line says pi has none" ;; *) fail "permissions line wrong for pi: $out" ;; esac

out="$(launch pi-speckit --sdd speckit --no-brief --model opencode-go/deepseek-v4.1-flash)"
case "$out" in *"pi framework dispatch REFUSED BEFORE BUILDING ANYTHING"*) pass "pi with --sdd speckit is refused before building" ;; *) fail "pi speckit was not refused: $out" ;; esac
case "$out" in *"agent args:"*) fail "pi speckit reached argument construction: $out" ;; esac

out="$(launch pi-bare --sdd none --no-brief --model deepseek-v4-pro)"
case "$out" in *"pi needs provider/model"*) pass "a bare pi model id is refused with the provider/model form" ;; *) fail "bare pi model not refused: $out" ;; esac

out="$(launch pi-ask --sdd none --no-brief --model opencode-go/glm-5.3 --permissions ask)"
case "$out" in *"--permissions ask cannot be honoured on pi"*) pass "--permissions ask is refused on pi" ;; *) fail "pi ask not refused: $out" ;; esac

for row in "build opencode-go/deepseek-v4-pro" "design opencode-go/deepseek-v4-pro" "audit opencode-go/deepseek-v4-pro" \
           "review opencode-go/glm-5.3-flash" "explore opencode-go/deepseek-v4.1-flash"; do
  k="${row%% *}"; m="${row#* }"
  out="$(launch "pi-roster-$k" --sdd none --brief "$(brief "$k")")"
  case "$out" in
    *"--model $m "*|*"--model $m"$'\n'*) pass "kind $k with no --model defaults to $m" ;;
    *) fail "kind $k did not default to $m: $out" ;;
  esac
done

out="$(launch pi-account --sdd none --no-brief --model opencode-go/deepseek-v4.1-flash)"
case "$out" in *"NOT APPLIED"*) pass "the account reaches Claude Code only: pi's manifest says NOT APPLIED" ;; *) fail "no NOT APPLIED for pi's account: $out" ;; esac

# THINKING BY KIND (the operator, 2026-10-11; the brainer's roster ruling): explore off, every other kind low, never high by default;
# else medium; --effort overrides; the manifest says it is a default.
for row in "build low" "design low" "audit low" "review low" "explore off"; do
  k="${row%% *}"; e="${row#* }"
  out="$(launch "pi-think-$k" --sdd none --brief "$(brief "$k")")"
  case "$out" in *"--thinking $e"*) pass "kind $k thinks at $e by default" ;; *) fail "kind $k did not default to --thinking $e: $out" ;; esac
  case "$out" in *"effort=$e  (default; pi roster, kind $k)"*) pass "the manifest shows kind $k's thinking as a pi-roster default" ;; *) fail "manifest effort line for kind $k: $out" ;; esac
done
out="$(launch pi-think-flag --sdd none --brief "$(brief explore)" --effort minimal)"
case "$out" in *"--thinking minimal"*) pass "--effort overrides the roster's thinking" ;; *) fail "--effort minimal did not reach pi: $out" ;; esac
out="$(launch pi-think-bad --sdd none --no-brief --model opencode-go/glm-5.3 --effort ultra)"
case "$out" in *"pi --effort must be one of: off minimal"*) pass "an effort pi does not know is refused" ;; *) fail "pi accepted --effort ultra: $out" ;; esac
