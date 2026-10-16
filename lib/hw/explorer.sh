# shellcheck shell=bash
# lib/hw/explorer.sh — the cheap explorer every executor is handed, and the tier table the prompt states.
#
# SOURCED BY bin/hw, never executed. Reads the globals bin/hw sets (AGENT, PROJ, _brain_root) and nothing
# else; every function prints or sets a documented global, so the dry run and the launch ask the same code.
#
# WHY IT EXISTS (setup/briefs/delegar-por-modelo.md, 2026-10-14): a product lane's `road-pace-build`
# (kind: build) ran on opus and read the code itself, file by file; no lane had a subagent on a cheap model.
# The rule "haiku searches, sonnet implements, opus decides" lived in a CLAUDE.md nothing enforced. hw now
# hands every executor ONE read-only explorer pinned to its vendor's cheapest tier, NAMED THE SAME on every
# vendor (`cheap-explorer`), and the prompt it prepends says which job goes to which tier.
#
# WHERE IT COMES FROM: the brain, never the product repo. The role is `_agents/cheap-explorer.md`; each vendor
# gets it through its OWN per-launch mechanism, so nothing is written into a product repo:
#   claude    `--agents <json>`, NOT typed: the JSON goes to .hw/<run>/explorer-agents.json and a shell function
#             typed into the pane (claude_session_env_prep) adds `--agents "$(cat <file>)"` to the one `claude` herdr
#             types. The JSON used to ride on the typed line (2310 bytes): a pane whose shell was still starting read
#             it in the tty's canonical mode, which holds 1024, and claude never started (2026-10-15, opus).
#   opencode  an `agent` entry in the OPENCODE_CONFIG_CONTENT overlay (merged last, this session only)
#   codex     `-c agents.cheap-explorer.*` plus setup/codex/agents/cheap-explorer.toml (its per-agent config file)
#   pi        setup/pi/agents/ added to PI_SUBAGENT_EXTRA_AGENT_DIRS (pi-subagents' own read-only user-scope hook)
# setup/tests/1003-cada-ejecutor-delega-por-modelo.sh holds that the four copies carry the same role text.

HW_EXPLORER_NAME="cheap-explorer"
# THE CHEAPEST TIER OF EACH VENDOR, as that vendor's own machine config already names it: claude's haiku;
# opencode's and codex's `luna` (~/.config/opencode/opencode.json `explore-luna`, ~/.codex/config.toml
# `[agents.luna]`: "bounded lookups"); pi's roster `explore` model (PI_MODEL_CHEAP in bin/hw). No pricing is
# on disk, so the order luna < terra < sol is the vendor's own description, not a measurement of cost.
HW_EXPLORER_MODEL_CLAUDE="haiku"
HW_EXPLORER_MODEL_OPENCODE="openai/gpt-5.6-luna"
HW_EXPLORER_MODEL_CODEX="gpt-5.6-luna"
HW_EXPLORER_MODEL_PI="opencode-go/deepseek-v4.1-flash"
# `explore-luna` ships at reasoningEffort max, which spends the cheap model's saving; the explorer asks for low.
HW_EXPLORER_EFFORT_OPENCODE="low"

_explorer_role_file() { printf '%s/_agents/%s.md' "${_brain_root:-}" "$HW_EXPLORER_NAME"; }
_explorer_codex_file() { printf '%s/setup/codex/agents/%s.toml' "${_brain_root:-}" "$HW_EXPLORER_NAME"; }
_explorer_pi_dir() { printf '%s/setup/pi/agents' "${_brain_root:-}"; }

# The model the explorer runs on for <vendor> (default: this dispatch's AGENT).
_explorer_model() {
  case "${1:-${AGENT:-}}" in
    claude)   local p; p="$(_model_pin_for "${PROJ:-}" "$HW_EXPLORER_MODEL_CLAUDE")"; printf '%s' "${p:-$HW_EXPLORER_MODEL_CLAUDE}" ;;
    opencode) printf '%s' "$HW_EXPLORER_MODEL_OPENCODE" ;;
    codex)    printf '%s' "$HW_EXPLORER_MODEL_CODEX" ;;
    pi)       printf '%s' "$HW_EXPLORER_MODEL_PI" ;;
  esac
}

# Reads the role file's frontmatter and body: prints `name|description|tools|body` as JSON for the vendor
# renderers below. Folded (`>`) descriptions are joined into one line. Dies (exit 1, nothing printed) on a
# role file that is missing or malformed, so a caller never builds a half-agent.
_explorer_role_json() {
  local f head name desc tools body; f="$(_explorer_role_file)"
  [ -r "$f" ] || return 1
  # awk + jq, not python: this runs several times per dispatch and python's start-up was the measurable cost
  # (setup/tests/08 went 1.2s -> 2.7s of CPU). First output line: name, description, tools (tab-separated); the rest is the body.
  local parsed; parsed="$(awk '
    NR == 1 { if ($0 != "---") bad = 1; infm = 1; next }
    infm && $0 == "---" { infm = 0; done = 1; printf "%s\t%s\t%s\n", vals["name"], vals["description"], vals["tools"]; next }
    infm {
      if (match($0, /^[A-Za-z_-]+:/) && substr($0, 1, 1) != " ") {
        key = substr($0, 1, RLENGTH - 1); v = substr($0, RLENGTH + 1); sub(/^[ \t]+/, "", v); sub(/[ \t]+$/, "", v)
        vals[key] = (v == sprintf("%c", 62) || v == "|") ? "" : v
      } else if (key != "" && $0 ~ /[^ \t]/) {
        c = $0; sub(/^[ \t]+/, "", c); sub(/[ \t]+$/, "", c); vals[key] = vals[key] " " c; sub(/^ /, "", vals[key])
      }
      next
    }
    done { print }
    END { if (bad || !done) exit 1 }
  ' "$f")" || return 1
  head="${parsed%%$'\n'*}"
  case "$parsed" in *$'\n'*) body="${parsed#*$'\n'}" ;; *) body="" ;; esac
  IFS=$'\t' read -r name desc tools <<<"$head"
  [ -n "$name" ] && [ -n "$desc" ] && [ -n "$tools" ] || return 1
  body="${body#"${body%%[![:space:]]*}"}"; body="${body%"${body##*[![:space:]]}"}"
  jq -cn --arg n "$name" --arg d "$desc" --arg t "$tools" --arg p "$body" \
    '{name: $n, description: $d, tools: ($t | split(",") | map(gsub("^\\s+|\\s+$"; "") | select(. != ""))), prompt: $p}'
}

# claude: the `--agents` JSON for the cheap explorer, or nothing (exit 1) when the role cannot be read. Spaces are
# spelt   as in the other renderers (valid JSON, same string), so the copies stay byte-for-byte alike.
_explorer_claude_json() {
  local role model json
  role="$(_explorer_role_json)" || return 1
  model="$(_explorer_model claude)"
  json="$(printf '%s' "$role" | jq -ca --arg model "$model" \
    '{(.name): {description: .description, prompt: .prompt, tools: .tools, model: $model}}')" || return 1
  printf '%s' "${json// /\\u0020}"
}

# THE LAUNCH LINE STAYS SHORT. herdr single-quotes every word it types, so `--agents "$(cat f)"` handed to it arrives
# literal; a function in the pane's shell is the one place the substitution is still the shell's. `command claude`
# skips the function, so the sandbox's shim (first on PATH) is still what runs. This module only PRINTS: bin/hw's
# _explorer_claude_arm writes the file, inside the run directory (`.hw/` is git-excluded), never into a repo.
_explorer_claude_file() { printf '%s/explorer-agents.json' "$1"; }  # $1 = the run directory
_explorer_claude_function() {  # $1 = the file; the shell text that defines the function
  # The path is single-quoted by hand, not by %q: %q is locale-dependent for non-ASCII bytes, and the Go twin must print
  # the same bytes for ANY path (a hostile worktree name is a contract case).
  local q; q="'$(printf '%s' "$1" | sed "s/'/'\\\\''/g")'"
  # shellcheck disable=SC2016  # the $(cat ...) and "$@" belong to the PANE's shell
  printf 'claude() { command claude --agents "$(cat %s)" "$@"; }' "$q"
}

# opencode: the OPENCODE_CONFIG_CONTENT overlay that defines the subagent (deep-merged last with the effort and
# engram overlays by _opencode_config_content). `edit` is denied: the explorer reads.
_explorer_opencode_overlay() {
  local role; role="$(_explorer_role_json)" || return 1
  # The overlay travels as ONE word (the --here --env list, the run env file, the crash restore all split on
  # whitespace), so the spaces inside its strings are spelt \u0020, which JSON reads back as a space.
  local out
  out="$(printf '%s' "$role" | jq -c --arg model "$(_explorer_model opencode)" --arg eff "$HW_EXPLORER_EFFORT_OPENCODE" '
    {agent: {(.name): {mode: "subagent", description: .description, prompt: .prompt, model: $model,
                       reasoningEffort: $eff, permission: {edit: "deny"}}}}')" || return 1
  out="${out// /\\u0020}"
  case "$out" in *[[:space:]]*) return 1 ;; esac
  printf '%s' "$out"
}

# codex: the ` -c` words that register the agent; its model, effort and instructions live in the tracked file.
# Spaces in the description are TOML   escapes, for the same word-list reason as claude's JSON.
_explorer_codex_args() {
  local f role desc
  f="$(_explorer_codex_file)"; [ -r "$f" ] || return 1
  case "$f" in *[[:space:]]*) return 1 ;; esac
  role="$(_explorer_role_json)" || return 1
  desc="$(printf '%s' "$role" | jq -r '.description')" || return 1; desc="${desc// /\\u0020}"
  printf ' -c agents.%s.description="%s" -c agents.%s.config_file="%s"' "$HW_EXPLORER_NAME" "$desc" "$HW_EXPLORER_NAME" "$f"
}

# pi: the directory pi-subagents reads as an extra user-scope agent dir (PATH-style list).
_explorer_pi_agent_dir() {
  local d; d="$(_explorer_pi_dir)"
  [ -r "$d/$HW_EXPLORER_NAME.md" ] || return 0
  case "$d" in ""|*[[:space:]:]*) return 0 ;; esac
  printf '%s' "$d"
}

# What hw says about the explorer on the manifest and in the receipt. EXPLORER_LINE is set by _explorer_resolve,
# AFTER the vendor's launch words are built, and it checks the explorer is IN them (AGENT_ARGS, the opencode overlay,
# the pi directory list) rather than that it could have been: a line that reports what was requested where the reader
# needs what will happen is the defect hw's manifest keeps closing.
EXPLORER_LINE=""; EXPLORER_INJECTED=0; EXPLORER_ARM=0  # EXPLORER_ARM: claude's launch (_agent_start) writes the file and arms the pane
_explorer_resolve() {
  EXPLORER_LINE=""; EXPLORER_INJECTED=0; EXPLORER_ARM=0
  local m; m="$(_explorer_model "${AGENT:-}")"
  case "${AGENT:-}" in
    claude)
      # Not in AGENT_ARGS any more (see the header): EXPLORER_ARM is what _agent_start acts on, set here only when the
      # JSON builds, so the line says what the launch WILL do.
      if _explorer_claude_json >/dev/null 2>&1; then  # MUTATION-ANCHOR: 1003-M02
        EXPLORER_INJECTED=1; EXPLORER_ARM=1
        EXPLORER_LINE="$HW_EXPLORER_NAME on $m — claude --agents (JSON from _agents/$HW_EXPLORER_NAME.md in the run directory's explorer-agents.json, read by a shell function typed into the pane; nothing written into the repo)"
      else
        EXPLORER_LINE="NOT INJECTED — $(_explorer_role_file) is missing or malformed, so no --agents reached the launch"
      fi ;;
    opencode)
      if _opencode_config_content 2>/dev/null | jq -e --arg n "$HW_EXPLORER_NAME" '.agent[$n].model' >/dev/null 2>&1; then
        EXPLORER_INJECTED=1
        EXPLORER_LINE="$HW_EXPLORER_NAME on $m (reasoningEffort $HW_EXPLORER_EFFORT_OPENCODE) — OPENCODE_CONFIG_CONTENT agent overlay, this session only"
      else
        EXPLORER_LINE="NOT INJECTED — $(_explorer_role_file) is missing or malformed, so the overlay carries no agent"
      fi ;;
    codex)
      case " ${AGENT_ARGS:-} " in
        *" -c agents.$HW_EXPLORER_NAME.config_file="*)
          EXPLORER_INJECTED=1
          EXPLORER_LINE="$HW_EXPLORER_NAME on $m — codex -c agents.$HW_EXPLORER_NAME (config_file setup/codex/agents/$HW_EXPLORER_NAME.toml)" ;;
        *) EXPLORER_LINE="NOT INJECTED — setup/codex/agents/$HW_EXPLORER_NAME.toml or the role file is missing, so no -c agents.* reached the launch" ;;
      esac ;;
    pi)
      if ! _pi_subagents_ready; then
        EXPLORER_LINE="NOT INJECTED — pi-subagents is not loaded in this pi executor (see the pi subagents line): it has no subagent tool, so there is nothing to delegate to"
      else
        case ":$(_pi_agent_dirs ""):" in
          *":$(_explorer_pi_dir):"*)
            EXPLORER_INJECTED=1
            EXPLORER_LINE="$HW_EXPLORER_NAME on $m — setup/pi/agents via PI_SUBAGENT_EXTRA_AGENT_DIRS (pi-subagents user scope)" ;;
          *) EXPLORER_LINE="NOT INJECTED — setup/pi/agents/$HW_EXPLORER_NAME.md is missing" ;;
        esac
      fi ;;
    *) EXPLORER_LINE="not applicable — unknown vendor '${AGENT:-}'" ;;
  esac
}

# THE TIER TABLE THE PROMPT STATES (requirement 2). One paragraph, the explorer's exact name, and for each
# vendor the mechanism that calls it. `$1` = vendor. Prints nothing when the explorer was not injected for that
# vendor, so the prompt never names an agent that is not there. A re-task (hw next) goes through the same
# function with the vendor persisted in HW_EXECUTOR_VENDOR; the injected state is re-derived from disk.
_explorer_tier_note() {
  local vendor="$1" call implement m
  # A RE-TASK (hw next) hands this prompt to a session that was LAUNCHED earlier, perhaps before the explorer existed:
  # it states the tier table only when that run's receipt says the explorer went in (HW_EXPLORER_RUNDIR is set by
  # lib/hw/next.sh), so the prompt never names an agent the session does not have.
  if [ -n "${HW_EXPLORER_RUNDIR:-}" ]; then
    case "$(jq -r 'select(.key=="explorer") | .value' "$HW_EXPLORER_RUNDIR/receipt.jsonl" 2>/dev/null | tail -1)" in
      "$HW_EXPLORER_NAME on "*) ;;
      *) return 0 ;;
    esac
  fi
  m="$(_explorer_model "$vendor")"
  case "$vendor" in
    claude)
      _explorer_claude_json >/dev/null 2>&1 || return 0
      call="the Agent tool with subagent_type \"$HW_EXPLORER_NAME\""
      implement="a sonnet subagent (the Agent tool with model \"sonnet\"), when the edit is already defined" ;;
    opencode)
      _explorer_opencode_overlay >/dev/null 2>&1 || return 0
      call="the task tool with subagent_type \"$HW_EXPLORER_NAME\""
      implement="a mid-tier subagent (general-terra), when the edit is already defined" ;;
    codex)
      _explorer_codex_args >/dev/null 2>&1 || return 0
      call="the spawn_agent tool with agent_type \"$HW_EXPLORER_NAME\""
      implement="a mid-tier agent (terra), when the edit is already defined" ;;
    pi)
      [ -n "$(_explorer_pi_agent_dir)" ] && _pi_subagents_ready || return 0
      call="the subagent tool with agent \"$HW_EXPLORER_NAME\""
      implement="yourself: pi's roster already runs builds on its build model" ;;
    *) return 0 ;;
  esac
  printf 'DELEGATE BY MODEL TIER. Reading is the expensive part of this work, so the top tier is kept for deciding.\n'
  printf '  searching, mapping code, finding call sites, pulling docs -> %s (%s): %s. It returns the answer with file:line, not the files.\n' "$HW_EXPLORER_NAME" "$m" "$call"
  printf '  edits and running tests -> %s.\n' "$implement"
  printf '  design, audit and the verdict -> you.\n'
  printf 'Before you read more than three files to answer one question, hand that question to %s.' "$HW_EXPLORER_NAME"
}
