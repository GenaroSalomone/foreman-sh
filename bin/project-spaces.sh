# project-spaces.sh — the ONE table mapping a project to its herdr workspace.
#
# WHY THIS FILE EXISTS. `bin/brain` labelled its workspace `brain:<proj>` and
# `bin/hw --tab` labelled its own `hw:<project>`, so a project had two workspaces
# and neither meant "the project". Making them agree needs one table, in one
# place: two copies of a mapping is the drift that produced most of 2026-08-24's
# frictions, where a rule corrected in one file stayed wrong in another.
#
# Sourced by bin/hw and bin/brain. NOT unconditionally folded into
# invoker-common.sh: hw sources it late and on purpose, inside cmd_next and
# _deliver_brief, and pulling it into every invoker at startup would drag its
# globals along for callers that never need them. The one exception, since
# 2026-09-15: invoker-common.sh conditionally sources this file — guarded by
# `[ -r ... ]`, swallowing errors, falling back to its own literal if anything
# here is missing or fails — purely to read all_lane_worktree_roots() for
# INVOKER_ENV_ROOTS. That is a narrow, defensive read, not a standing
# dependency: done-invoker/ask-invoker keep working even if this file is
# absent or broken.
#
# One space per project, one tab per agent:
#
#   lane-a                brainer · <task> · <task>
#   lane-b                brainer · <task>
#   lane-c                brainer · <task> · <task>

# ── the lane table itself: brain/projects.json ──────────────────────────────
#
# EVERY PER-LANE VALUE BELOW COMES FROM ONE FILE. Until the second stage of the
# harness opening (setup/decisions.md, «El harness va hacia reusable…»), each
# function in this file was a `case` with one arm per lane, and so were a dozen
# more in bin/hw and bin/brain; adding a lane meant finding every one. The
# values now live in `projects.json` at the brain root, and this is its only
# reader: lane_config_load turns it into shell variables once per process, and
# lane_get / lane_path read them back. A lane added there reaches every
# function here without an edit in bin/.
#
# WHY SHELL VARIABLES AND NOT A jq CALL PER LOOKUP. bin/hw asks these questions
# dozens of times per dispatch, from bash 3.2 (no associative arrays) and from
# zsh (shell/hw-restore-env.zsh sources this file). One jq pass that prints
# `_LC_<lane>__<field>='value'` assignments, eval'd once, answers both without a
# second dialect.
#
# WHAT IS CHECKED AT LOAD, because eval'ing a file demands it: lane names and
# aliases are lowercase words (they become parts of variable names), a
# checkout_var is an upper-case identifier, every value is emitted through
# jq's @sh quoting, and an alias may not shadow a lane or another alias. A
# file that fails any of these loads NOTHING, and the caller decides whether
# that is fatal (bin/hw and bin/brain die; the invokers keep their fallback).
#
# PATHS: a leading `~` is $HOME, resolved here. `{brain}`, `{checkout}`,
# `{work}`, `{lane}` and `{task}` are resolved per call, because BRAIN, WORK and
# the checkout variables can be set by the caller (bin/hw reads BRAIN live, as
# its old literal did), and a task name is only known per call.
#
# HW_PROJECTS_JSON names another table file, for a test or a second checkout
# that has to read a table other than its root's; the brain root itself is
# still $1.
lane_config_load() {  # $1 = brain root; reads $1/projects.json
  local _lc_root="$1" _lc_file="${HW_PROJECTS_JSON:-$1/projects.json}" _lc_out
  [ -r "$_lc_file" ] || { printf 'lane table: cannot read %s\n' "$_lc_file" >&2; return 1; }
  command -v jq >/dev/null 2>&1 || { printf 'lane table: jq is not on PATH, so %s cannot be read\n' "$_lc_file" >&2; return 1; }
  _lc_out="$(jq -r --arg home "$HOME" --arg brain "$_lc_root" '
    def word: test("^[a-z][a-z0-9-]*$");
    def key: gsub("-"; "_");
    def exp: if type == "string" then sub("^~(?=/|$)"; $home) else . end;
    def emit($lane; $field; $v): "_LC_\($lane | key)__\($field)=\($v | tostring | @sh)";
    (.lanes // error("no `lanes` object")) as $lanes
    | ($lanes | keys_unsorted) as $names
    | if ($names | length) == 0 then error("`lanes` is empty") else . end
    | if ($names | all(word)) then . else error("a lane name is not a lowercase word: \($names | map(select(word | not)))") end
    | (["product_repo","hw_aliases","brain_aliases","space","engram","vendor","model","account","artifacts","repoless",
        "checkout","checkout_var","base","base_ref_prefix","branch","worktree_root","worktree","ports",
        "hint_aliases","build","agents_from","deps","db","devserver","reap","sweep","brain_guard","brief_note","sdd_modes",
        "model_floor","model_pins","requested_by","opencode_config_dir","suite_lock"]) as $fields
    | ({model_floor: ["tier","accepts"], deps: ["line","contention"], db: ["line","provisioned","no_worktree","no_db_inert"],
        devserver: ["start","line","off"], reap: ["copies","copies_from","max_worktrees","max_disk_pct","max_build_gb"]}) as $sub
    | ([$lanes | to_entries[] | .key as $l | .value | to_entries[] | select($sub[.key] != null) | .key as $f
        | if (.value | type) == "object" then (.value | keys[] | select(. as $k | $sub[$f] | index([$k]) | not) | "\($l).\($f).\(.)")
          else "\($l).\($f) (not an object)" end]) as $badsub
    | if ($badsub | length) == 0 then . else error("unknown lane field(s): \($badsub | join(", "))") end
    | ([$lanes | to_entries[] | .key as $l | (.value | keys[]) | select(. as $k | $fields | index([$k]) | not) | "\($l).\(.)"]) as $unknown
    | if ($unknown | length) == 0 then . else error("unknown lane field(s): \($unknown | join(", "))") end
    | ([$lanes | to_entries[] | .key as $l | select(.value.sdd_modes != null) | .value.sdd_modes
        | if type == "array" and all(. == "gentle") then empty else "\($l).sdd_modes" end]) as $badmodes
    | ([$lanes | to_entries[] | .key as $l | select(.value.requested_by != null)
        | if (.value.requested_by | IN("required", "warn")) then empty else "\($l).requested_by" end]) as $badreq
    | if ($badreq | length) == 0 then . else error("requested_by is required or warn: \($badreq | join(", "))") end
    | ([$lanes | to_entries[] | .key as $l | select(.value.suite_lock != null)
        | if (.value.suite_lock | IN("lane", "none")) then empty else "\($l).suite_lock" end]) as $badlock
    | if ($badlock | length) == 0 then . else error("suite_lock is lane or none: \($badlock | join(", "))") end
    | ([$lanes | to_entries[] | .key as $l | select(.value.artifacts != null)
        | if (.value.artifacts | IN("deny", "default", "personal")) then empty else "\($l).artifacts" end]) as $badart
    | if ($badart | length) == 0 then . else error("artifacts is deny or an account (default, personal): \($badart | join(", "))") end
    | ([$lanes | to_entries[] | .key as $l | select(.value.model_floor != null) | .value.model_floor
        | if (.tier | IN("haiku", "sonnet", "opus")) and ((.accepts // []) | type == "array" and all(type == "string"))
          then empty else "\($l).model_floor" end]) as $badfloor
    | ([$lanes | to_entries[] | .key as $l | select(.value.model_pins != null) | .value.model_pins
        | if type == "object" and (to_entries | all(.key | IN("haiku", "sonnet", "opus", "fable")) and all(.value | type == "string" and test("^claude-[a-z0-9.-]+$")))
          then empty else "\($l).model_pins" end]) as $badpins
    | if ($badpins | length) == 0 then . else error("model_pins maps an alias (haiku, sonnet, opus, fable) to a full claude-* id: \($badpins | join(", "))") end
    | if ($badfloor | length) == 0 then . else error("model_floor.tier is haiku, sonnet or opus, and accepts is a list of model ids: \($badfloor | join(", "))") end
    | if ($badmodes | length) == 0 then . else error("sdd_modes lists the modes a lane adds to speckit and none, and the only one is gentle: \($badmodes | join(", "))") end
    | ((keys - ["comment","work","operator","metrics_direct","survey_order","retention","lanes"])) as $top
    | if ($top | length) == 0 then . else error("unknown top-level key(s): \($top | join(", "))") end
    | ((.retention // {}) as $r
       | if ($r | type == "object") and ((($r | keys_unsorted) - ["artifact_retention_days","backup_retention_days"]) | length) == 0
            and ([$r.artifact_retention_days // 30, $r.backup_retention_days // 60] | all(type == "number" and . >= 0 and . == floor))
         then . else error("retention is {artifact_retention_days, backup_retention_days}, each a whole number of days >= 0 (0 disables that window)") end) as $_ret
    | ((.survey_order // []) as $so
       | if ($so | all(. as $n | $names | index([$n]))) then . else error("survey_order names a lane that `lanes` does not have") end
       | ($so + ($names - $so))) as $survey
    | ([$lanes | to_entries[] | .key as $l | ((.value.hw_aliases // [])[] | {kind: "hw", alias: ., lane: $l}),
                                          ((.value.brain_aliases // [])[] | {kind: "brain", alias: ., lane: $l})]) as $aliases
    | if ($aliases | all(.alias | type == "string" and word)) then . else error("an alias is not a lowercase word") end
    | if ($aliases | all(.alias as $a | $names | index([$a]) | not)) then . else error("an alias shadows a lane name") end
    | if ([$aliases | group_by(.kind)[] | group_by(.alias)[] | select(length > 1)] | length) == 0 then . else error("an alias is claimed twice") end
    | "HW_LANES=\($names | join(" ") | @sh)",
      "HW_LANES_SURVEY=\($survey | join("\n") | @sh)",
      "_LC_WORK_DEFAULT=\(.work // error("no top-level `work`") | exp | @sh)",
      "_LC_RETENTION_ARTIFACT_DAYS=\(.retention.artifact_retention_days // 30 | tostring | @sh)",
      "_LC_RETENTION_BACKUP_DAYS=\(.retention.backup_retention_days // 60 | tostring | @sh)",
      "_LC_OPERATOR_DEFAULT=\(.operator // "the operator" | tostring | @sh)",
      ($aliases[] | "_LA_\(.kind)__\(.alias | key)=\(.lane | @sh)"),
      ($lanes | to_entries[] | .key as $l | .value as $v
        | if ($v.checkout_var // "" | test("^([A-Z_][A-Z0-9_]*)?$")) then . else error("checkout_var of \($l) is not an identifier") end
        | emit($l; "space"; $v.space // error("lane \($l) has no space label")),
          emit($l; "engram"; $v.engram // "brain"),
          emit($l; "vendor"; $v.vendor // "claude"),
          emit($l; "model"; $v.model // ""),
          emit($l; "model_floor"; $v.model_floor.tier // ""),
          emit($l; "model_floor_accepts"; ($v.model_floor.accepts // []) | join(" ")),
          emit($l; "model_pins"; ($v.model_pins // {}) | to_entries | map("\(.key)=\(.value)") | join(" ")),
          emit($l; "requested_by"; $v.requested_by // ""),
          emit($l; "suite_lock"; $v.suite_lock // ""),
          emit($l; "account"; $v.account // "default"),
          emit($l; "artifacts"; $v.artifacts // $v.account // "default"),
          emit($l; "repoless"; if $v.repoless == true then "1" else "" end),
          emit($l; "product_repo"; if $v.product_repo == true then "1" else "" end),
          emit($l; "checkout"; $v.checkout // "" | exp),
          emit($l; "checkout_var"; $v.checkout_var // ""),
          emit($l; "base"; $v.base // ""),
          emit($l; "base_ref_prefix"; $v.base_ref_prefix // "origin/"),
          emit($l; "branch"; $v.branch // "task/{task}"),
          emit($l; "worktree_root"; $v.worktree_root // "" | exp),
          emit($l; "worktree"; $v.worktree // "" | exp),
          emit($l; "hint_aliases"; ($v.hint_aliases // []) | join("|")),
          emit($l; "build"; $v.build // ""),
          emit($l; "sweep"; if $v.sweep == false then "0" else "1" end),
          emit($l; "brain_guard"; if $v.brain_guard == false then "0" else "1" end),
          emit($l; "agents_from"; $v.agents_from // ""),
          emit($l; "brief_note"; $v.brief_note // ""),
          emit($l; "sdd_modes"; ($v.sdd_modes // []) | join(" ")),
          emit($l; "opencode_config_dir"; $v.opencode_config_dir // "" | exp),
          emit($l; "deps_line"; $v.deps.line // ""),
          emit($l; "deps_contention"; if $v.deps.contention == true then "1" else "" end),
          emit($l; "db_line"; $v.db.line // ""),
          emit($l; "db_provisioned"; if $v.db.provisioned == true then "1" else "" end),
          emit($l; "db_no_worktree"; $v.db.no_worktree // ""),
          emit($l; "db_no_db_inert"; if $v.db.no_db_inert == true then "1" else "" end),
          emit($l; "devserver"; $v.devserver.start // ""),
          emit($l; "devserver_line"; $v.devserver.line // ""),
          emit($l; "devserver_off"; $v.devserver.off // ""),
          emit($l; "reap_copies"; ($v.reap.copies // []) | join("\n")),
          emit($l; "reap_copies_from"; $v.reap.copies_from // ""),
          emit($l; "reap_max_worktrees"; $v.reap.max_worktrees // ""),
          emit($l; "reap_max_disk_pct"; $v.reap.max_disk_pct // ""),
          emit($l; "reap_max_build_gb"; $v.reap.max_build_gb // ""),
          (($v.ports // {}) | to_entries[]
            | if (.key | test("^[a-z]+$")) and (.value | type == "number") then . else error("a port of \($l) is not name: number") end
            | emit($l; "port_\(.key)"; .value)))
  ' "$_lc_file")" || { printf 'lane table: %s is not a valid lane table (nothing was loaded)\n' "$_lc_file" >&2; return 1; }
  eval "$_lc_out"
  : "${WORK:=$_LC_WORK_DEFAULT}"
  # The person this harness answers to, as the messages name them: an
  # executor is told to leave a decision to them, a brainer to ask them.
  # `operator` in projects.json; "the operator" when the table names no one.
  : "${HW_OPERATOR:=$_LC_OPERATOR_DEFAULT}"
  # How long `hw reap` keeps the artifacts of a task that reported, and its
  # backups (`retention` in projects.json); 0 disables that window. An env
  # override set before sourcing wins, like WORK.
  : "${HW_ARTIFACT_RETENTION_DAYS:=$_LC_RETENTION_ARTIFACT_DAYS}"
  : "${HW_BACKUP_RETENTION_DAYS:=$_LC_RETENTION_BACKUP_DAYS}"
  # The checkout variables a lane names keep their old spelling (each lane's
  # checkout_var, <LANE>_MAIN) because callers and tests set them to redirect a lane, and
  # `:=`-style: an override set before sourcing wins.
  # A `read` loop over the newline list, not `for x in $HW_LANES`: zsh does
  # not word-split an unquoted scalar.
  local _lc_lane _lc_var _lc_val
  while IFS= read -r _lc_lane; do
    [ -n "$_lc_lane" ] || continue
    _lc_var="$(lane_get "$_lc_lane" checkout_var)"
    [ -n "$_lc_var" ] || continue
    eval "_lc_val=\"\${$_lc_var:-}\""
    [ -n "$_lc_val" ] || eval "$_lc_var=\"\$(_lane_expand \"\$(lane_get \"\$_lc_lane\" checkout)\")\""
  done <<EOF
$HW_LANES_SURVEY
EOF
  ALL_KNOWN_PROJECTS="$HW_LANES_SURVEY"
  HW_LANE_TABLE="$_lc_file"
}

# Is $1 a lane the table knows? The charset check is not decoration: the name
# becomes part of a variable name in lane_get's eval.
lane_known() {
  case "$1" in ''|*[!a-z0-9-]*) return 1 ;; esac
  case " ${HW_LANES:-} " in *" $1 "*) return 0 ;; esac
  return 1
}

# lane_get <lane> <field> — the raw value, or nothing. Never fails loudly: an
# unknown lane or field prints nothing and returns 1, so callers keep choosing
# their own default exactly as the old `*)` arms did.
lane_get() {
  lane_known "$1" || return 1
  case "$2" in ''|*[!a-z_]*) return 1 ;; esac
  eval "printf '%s' \"\${_LC_${1//-/_}__$2-}\""
}

# The lane's checkout directory: its checkout_var's current value when it has
# one (so an override is honoured), the table's otherwise. Empty for a lane with
# no checkout.
lane_checkout() {
  local _var _val
  _var="$(lane_get "$1" checkout_var)" || return 0
  if [ -n "$_var" ]; then eval "_val=\"\${$_var:-}\""; printf '%s' "$_val"
  else _lane_expand "$(lane_get "$1" checkout)"; fi
}

# The `{brain}` token, resolved against the caller's BRAIN — empty when the
# caller has none, exactly as bin/hw's old `${BRAIN:-}` was.
_lane_expand() {
  local _t="$1" _k='{brain}'
  case "$_t" in *"$_k"*) _t="${_t//$_k/${BRAIN:-}}" ;; esac
  printf '%s' "$_t"
}

# lane_path <lane> <field> [task] — a path template with every token resolved.
lane_path() {
  local _t _k
  _t="$(lane_get "$1" "$2")" || return 0
  [ -n "$_t" ] || return 0
  _k='{checkout}'; case "$_t" in *"$_k"*) _t="${_t//$_k/$(lane_checkout "$1")}" ;; esac
  _k='{work}';     _t="${_t//$_k/${WORK:-}}"
  _k='{lane}';     _t="${_t//$_k/$1}"
  _k='{task}';     _t="${_t//$_k/${3:-}}"
  _lane_expand "$_t"
}

# Does the lane put its worktrees where its plain work directories go
# ({work}/{lane}/<task>)? Then a directory there may be either, and only the
# directory itself — a `.git` or not — can say which. bin/hw keys the artifacts
# location and the close's verification directory on this, never on a name.
lane_worktree_is_workdir() {  # $1 = lane
  local _r
  _r="$(lane_path "$1" worktree_root)"
  [ -n "$_r" ] && [ "$_r" = "${WORK:-}/$1" ]
}

# lane_alias <hw|brain> <word> — the lane an alias names, or nothing.
lane_alias() {
  case "$2" in ''|*[!a-z0-9-]*) return 1 ;; esac
  case "$1" in hw|brain) ;; *) return 1 ;; esac
  local _v="_LA_$1__${2//-/_}"
  eval "printf '%s' \"\${$_v-}\""
}

# "a | b | c" — the known lanes, in table order, for a refusal message.
lane_names_bar() {
  printf '%s' "${HW_LANES:-}" | sed 's/ / | /g'
}

# "a|x | b | c" — the same list with the aliases a lane's `hint_aliases`
# names, the spelling the status and reports refusals print.
lane_names_hint() {
  local _l _h _out=""
  for _l in ${HW_LANES:-}; do
    _h="$(lane_get "$_l" hint_aliases)" || _h=""
    _out="$_out${_out:+ | }$_l${_h:+|$_h}"
  done
  printf '%s' "$_out"
}

# lane_build_hook <lane> — the file that builds a task's worktree for that lane,
# or nothing when the lane names none. Relative to the table's own directory,
# so a second checkout's table points at its own hooks.
lane_build_hook() {
  local _b
  _b="$(lane_get "$1" build)" || return 0
  [ -n "$_b" ] || return 0
  case "$_b" in /*) printf '%s' "$_b" ;; *) printf '%s/%s' "$(dirname "${HW_LANE_TABLE:-.}")" "$_b" ;; esac
}

# The workspace label for a project. Lowercase, no prefix — it is the space's
# name as a human reads it in herdr, not a namespace.
project_space_label() {
  lane_known "$1" || return 1
  lane_get "$1" space
}

# The tab label for an agent inside that space. The brainer is long-lived and
# there is one; executors are named by their task, which is also what `hw done`
# looks up.
project_brainer_tab() { printf 'brainer'; }

# The vendor a project's brainer actually runs. A lane's brainer may be opencode —
# `opencode.json`, its orchestrator agent, its whole history — so
# `brain <lane>` with no flag used to start a CLAUDE brainer beside it. Observed
# 2026-08-24 during the tab migration: it created a second brainer tab nobody
# asked for. A default that contradicts the project is not a default.
project_default_kind() {
  local v; v="$(lane_get "$1" vendor)" || v=""
  printf '%s' "${v:-claude}"
}

# The provider a lane's own decision pins, for the vendor above. A product lane's
# lane decision requires OpenCode direct `openai/gpt-5.6-sol`, and for a while
# that decision lived only in memory and in whoever remembered to type it — so
# an abbreviated command silently launched something else and the manifest
# called it `(default)`. A default that matches the decision is the decision;
# a default that contradicts it is a trap. Empty means "no pinned provider,
# let the vendor choose"; --model always wins.
# WHICH MODELS ARE FOR BUILDING, AND ONLY BUILDING.
#
# `openai/gpt-5.6-sol` is a BUILD model — its own runner labels every turn
# "Build". A product lane pins it as the lane default, and on 2026-08-25 that pin
# carried it into ten tasks that were not builds at all: bar-browser-diagnosis,
# classic-browser-diagnosis, classic-browser-diag-r2, elegant-browser-diag-r2,
# grid-compact-browser-audit, grid-compact-browser-diag-gpt,
# grid-compact-browser-diag-tab and three more. The same lane ran the SAME class
# of task on opus (grid-compact-browser-diagnosis, grid-opus-final-audit,
# three preapproval tasks), so the lane was not even consistent with
# itself — the pin simply reached whatever hw launched next.
#
# A lane default is the right answer for the work the lane mostly does. It is
# the wrong answer for the work that is deciding WHAT to build.
model_is_build_only() {
  case "$1" in
    openai/gpt-5.6-sol|openai/gpt-5.6-sol-*) return 0 ;;
    *) return 1 ;;
  esac
}

# THE CLAUDE TIER LADDER, AND NOTHING ELSE.
#
# The global rule is "empezar por el modelo mas barato que pueda hacer el
# trabajo" and "ante la duda entre dos tiers, el menor". Obeying it needs an
# ORDER, so one lives here beside the other model facts rather than being
# re-derived by each caller.
#
# Matched on the LAST path segment, exactly like _sdd_model_is_below_orchestrator
# in bin/hw, so an opencode `anthropic/claude-sonnet-5` ranks the same as
# claude's `sonnet`.
#
# EMPTY OUTPUT MEANS UNRANKABLE, and that is a real answer, not a gap:
# `openai/gpt-5.6-sol` and `fable` are not points on this ladder, and a caller
# that gets nothing back must make no claim. Same posture as the sonnet-
# orchestrator gate: this knows the claude tier ladder and claims nothing else.
model_tier() {
  case "${1##*/}" in
    haiku|haiku-*|*haiku*)   printf '1' ;;
    sonnet|sonnet-*|*sonnet*) printf '2' ;;
    opus|opus-*|*opus*)      printf '3' ;;
    *) : ;;  # MUTATION-ANCHOR: 159-M03
  esac
}

project_default_model() {
  lane_get "$1" model || true
}

# THE LOWEST TIER A LANE LAUNCHES WITHOUT A STATED REASON, on the ladder above.
# Empty means the lane declares no floor and nothing about its model changes.
# The first is opus: the operator rejected sonnet executors in that lane on
# 2026-09-22, 09-24 and 09-28, and the `--model` note was only informative
# (setup/decisions.md, 2026-09-28). The ladder is Claude's; `accepts` lists the off-ladder models the
# lane's own decisions admit, because "above opus" is not a question the ladder
# can answer about `openai/gpt-5.6-sol`.
project_model_floor() {
  lane_get "$1" model_floor || true
}

project_model_floor_accepts() {
  lane_get "$1" model_floor_accepts || true
}

# Whether a dispatch must cite the request it answers: required | warn | "".
project_requested_by_mode() {
  lane_get "$1" requested_by || true
}

# ── the Claude Code ACCOUNT a lane runs under ───────────────────────────────
#
# The operator's standing instruction: in a product lane, both brainers and executors
# default to the operator's personal account, and only for that project. One
# lane, both roles, by default — so it belongs in this table beside the vendor and the
# model, and not in a `.envrc` nobody sees.
#
# The ONLY mechanism is CLAUDE_CONFIG_DIR, which is why every consumer of this
# table also has to check the VENDOR: opencode and codex do not read it, so
# naming an account for them would be a field that claims a redirect that never
# happens. That belief — an executor that thinks it is on the personal account
# and is not — is the failure this table exists to make impossible, so the
# callers state APPLIED or NOT APPLIED rather than just the name.
#
# two more lanes joined it on the operator's standing
# instruction that every agent they launch runs under the personal account.
# For one of them it also closes an asymmetry that was already on
# disk: its brainer was started by hand with a personal CLAUDE_CONFIG_DIR,
# so `default` here would have handed its executors a different account than the
# brainer that dispatches them. The other is the operator's own personal project and its repo
# will be created under their personal GitHub account, so `personal` is coherent
# end to end there too.
# The first lane's `personal` is the operator's standing call;
# the other two stay on `default`, also theirs.
project_default_account() {
  local v; v="$(lane_get "$1" account)" || v=""
  printf '%s' "${v:-default}"
}

# WHERE A LANE'S EXECUTORS MAY PUBLISH claude.ai ARTIFACTS: `deny`, or the one
# account whose gallery receives them. Absent, it is the lane's own `account`.
#
# MEASURED 2026-09-29 (setup/decisions.md): the Artifact tool publishes into the
# gallery of the claude.ai login CLAUDE_CONFIG_DIR names — the same Claude Code,
# asked for `list`, showed the Teams gallery under ~/.claude and the personal
# one under ~/.claude-personal. So `--account` alone decides where an artifact
# lands, and an override of it moves the artifact with it. hw denies the tools
# whenever the account an executor runs under is not this one.
project_artifacts() {
  local v; v="$(lane_get "$1" artifacts)" || v=""
  printf '%s' "${v:-default}"
}

# THE FRAMEWORK MODES A LANE ADDS to the two every lane has (speckit, none).
# One exists: `gentle`, gentle-ai's ODD, enabled for one lane
# (setup/decisions.md). A lane that lists nothing answers exactly
# as it did before the mode existed.
project_sdd_modes() {
  local v; v="$(lane_get "$1" sdd_modes)" || v=""
  printf '%s' "$v"
}

# The config directory an account name resolves to. Empty means "whatever this
# machine's default is" — the launcher inherits nothing and sets nothing.
account_config_dir() {
  case "$1" in
    default)  printf '' ;;
    personal) printf '%s/.claude-personal' "$HOME" ;;
    *) return 1 ;;
  esac
}

# The command that OWNS the capability-parity check for an account.
#
# ~/.local/bin/claude-personal already refuses to launch when `skills/` count or
# `enabledPlugins` differ from the default account (setup/decisions.md,
# 2026-09-09). Re-implementing that here would be a second copy of a check whose
# whole value is that it is one — so hw and brain DELEGATE to it, and an account
# whose verifier is not on PATH is refused rather than launched unverified.
account_verifier() {
  case "$1" in
    default)  printf '' ;;
    personal) printf 'claude-personal' ;;
    *) return 1 ;;
  esac
}

# account_verify <account> — 0 verified (or nothing to verify), 1 refused.
# The diagnosis goes to stderr; the caller decides whether that is fatal.
account_verify() {
  local acct="$1" verifier
  verifier="$(account_verifier "$acct")" || {
    printf 'unknown account: %s (known: default, personal)\n' "$acct" >&2
    return 1
  }
  [ -n "$verifier" ] || return 0
  command -v "$verifier" >/dev/null 2>&1 || {
    printf 'account %s is verified by `%s`, which is not on PATH — so capability parity cannot be established and the account is refused.\n' \
      "$acct" "$verifier" >&2
    return 1
  }
  # THE VERIFIER'S STDOUT GOES TO STDERR, and this is not cosmetic. `claude-personal
  # --check` prints the config dir it verified; a caller that builds a value on
  # stdout — bin/brain's `brain_account_env`, whose whole output is one
  # `CLAUDE_CONFIG_DIR=...` flag — would otherwise splice that line into the
  # flag it hands herdr. Measured while driving the function directly: the flag
  # came back as "ok<newline>CLAUDE_CONFIG_DIR=…". The diagnosis stays visible
  # either way, and hw captures it with 2>&1 for its refusal text.
  "$verifier" --check >&2
}

# ── the roots a lane's work lives under ─────────────────────────────────────
#
# THE ONLY DEFINITION OF THESE THREE. They lived in `bin/hw` until 2026-09-14,
# when `bin/brain` grew a second reader of them; leaving hw's copy in place and
# adding a fallback here would have been two literals nobody keeps in sync, which
# is the exact drift this file was created to stop. hw now gets them from here.
#
# `:=` rather than `=` so a caller can still override one deliberately before
# sourcing; nothing in the tree does today.
#
# Lowercase `projects` is the CANONICAL on-disk name (APFS is case-insensitive,
# so `Projects` also resolves — which is how the wrong case survived unnoticed).
# It matters because herdr canonicalizes a pane's cwd and Python's os.getcwd()
# returns the true case, so a path spelled `Projects` here never matches the cwd
# a restored pane actually reports. That silently killed a product lane's worktree root
# for both the shell snippet and the invokers.
# WORK and the per-lane checkout variables (each lane's checkout_var,
# e.g. <LANE>_MAIN) are set by lane_config_load from projects.json,
# `:=`-style, so a caller's override made before loading still wins.

# The directories a lane's task work directories sit in. Some lanes
# put a task's worktree under the product checkout's
# `.worktrees`, and every lane also has a plain work directory; both are real and
# both hold `.hw` run records, so both are listed. Space separated, for a `for`.
lane_work_bases() {
  lane_known "$1" || return 1
  local root
  root="$(lane_path "$1" worktree_root)"
  if [ -z "$root" ] || [ "$root" = "$WORK/$1" ]; then printf '%s/%s' "$WORK" "$1"
  else printf '%s %s/%s' "$root" "$WORK" "$1"; fi
}

# Every lane projects.json knows, one per line, in its survey order.
#
# Newline-separated, not space-separated, and that is load-bearing: a bare
# `for x in $VAR` word-splits an unquoted scalar in bash but NOT in zsh (no
# SH_WORD_SPLIT by default), and shell/hw-restore-env.zsh sources this file
# directly into a real interactive zsh. Every reader below walks it with a
# `read` loop, which splits on IFS explicitly in both shells, so one literal
# drives bash and zsh callers without a second dialect of the same list.
ALL_KNOWN_PROJECTS=""  # filled from projects.json's survey order by lane_config_load

# The union of every known project's lane_work_bases(), one path per line, in
# project order, EXCLUDING anything already under $WORK — a caller that also
# lists $WORK itself as a blanket root (both callers below do) would otherwise
# get $WORK/<proj> duplicated as a nested root, which is harmless but noisy.
# What is left, after that filter, is exactly "the roots outside $WORK": one
# per repo-backed lane's own `.worktrees`.
#
# THE SINGLE SOURCE for the invoker return-channel's non-$WORK roots. Until
# 2026-09-15, bin/invoker-common.sh's INVOKER_ENV_ROOTS and
# shell/hw-restore-env.zsh's `roots` array were two hand-typed copies of
# "$WORK plus the repo-backed lane roots" — and a third, unrelated copy lived
# in bin/hw's `_next_autoclosed_run_for_pane`. None of the three had been
# updated for the newer lanes, which is why their executors'
# `done-invoker` walked up from
# ~/projects/<lane>/.worktrees/<task>/.hw/<run>/env, matched no listed root,
# and died with `no HW_INVOKER_PANE` despite the env file sitting right there
# (setup/briefs/el-canal-alcanza-cada-worktree.md). A lane that gets one
# `lane_work_bases()` arm and one line in ALL_KNOWN_PROJECTS now reaches every
# caller of this function without a second hand-edit anywhere.
#
# Skips a project lane_work_bases() refuses rather than failing the whole
# union — callers still keep their own hardcoded fallback for when sourcing or
# calling this fails outright, so a broken derivation never takes the invoker
# channel down with it.
all_lane_worktree_roots() {
  local proj bases base
  while IFS= read -r proj; do
    [ -n "$proj" ] || continue
    bases="$(lane_work_bases "$proj" 2>/dev/null)" || continue
    bases="$(printf '%s' "$bases" | tr ' ' '\n')"
    while IFS= read -r base; do
      [ -n "$base" ] || continue
      case "$base" in
        "$WORK"/*) continue ;;
      esac
      printf '%s\n' "$base"
    done <<EOF
$bases
EOF
  done <<EOF
$ALL_KNOWN_PROJECTS
EOF
}

# Where bin/runenv lives: beside this file, however it was sourced.
_PS_BIN_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# WHERE A RUN'S REPORT WAS SENT, from disk alone. `env` is the authoritative
# record — it is what the executor process actually received — and the dispatch
# manifest is the fallback for runs written before hw kept an env file.
#
# Shared rather than private to `hw` since 2026-09-14: `brain --reset` has to ask
# the same question about the same files, and a second reader of a format is the
# classic place for a quiet disagreement.
run_invoker_pane() {  # $1 = run directory
  local rundir="$1" pane=""
  if [ -r "$rundir/env" ]; then
    # bin/runenv is the one reader of this file. Exit 1 is "no such key" and
    # silent; anything else (a file it cannot read, runenv itself missing) is
    # named, because an empty pane here sends a report nowhere.
    local rc=0
    pane="$("$_PS_BIN_DIR/runenv" --lenient get "$rundir" HW_INVOKER_PANE)" || rc=$?
    if [ "$rc" -ne 0 ]; then
      pane=""
      [ "$rc" -eq 1 ] || printf 'run_invoker_pane: %s/env could not be read (runenv exit %s)\n' "$rundir" "$rc" >&2
    fi
  fi
  if [ -z "$pane" ] && [ -r "$rundir/dispatch" ]; then
    pane="$(sed -n 's/^ *invoker  *\([A-Za-z0-9]*:[A-Za-z0-9]*\).*$/\1/p' "$rundir/dispatch" 2>/dev/null | tail -1 || true)"
  fi
  printf '%s' "$pane"
}

# THE RUNS OF A LANE THAT NEVER REPORTED, and it is the SAME test `hw reports`
# prints as "never reported": a run directory that has a receipt on disk — so it
# was really dispatched — and no `done` marker, which is the only durable proof
# that a report was delivered. Not a second calculation; the one `cmd_reports`
# already makes, lifted so a second caller cannot disagree with it.
#
# One line per run:   <task>\t<run>\t<invoker pane or empty>\t<work directory>
lane_unreported_runs() {  # $1 = project
  local proj="$1" bases base dir rundir task
  bases="$(lane_work_bases "$proj")" || return 1
  for base in $bases; do
    [ -d "$base" ] || continue
    for dir in "$base"/*; do
      [ -d "$dir/.hw" ] || continue
      task="$(basename "$dir")"
      for rundir in "$dir"/.hw/*; do
        [ -d "$rundir" ] || continue
        case "$(basename "$rundir")" in
          2[0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]-*) ;;
          *) continue ;;
        esac
        [ -r "$rundir/receipt.jsonl" ] || continue
        if [ -f "$rundir/done" ]; then continue; fi
        printf '%s\t%s\t%s\t%s\n' \
          "$task" "$(basename "$rundir")" "$(run_invoker_pane "$rundir")" "$dir"
      done
    done
  done
}

# ONE COPY OF THE PANE-IDENTITY PREDICATE, and it stays one copy.
#
# Three readers ask this question now: `hw`'s launch guard, which REFUSES a
# foreign route; `hw reports`, which EXPLAINS a route a historical run already
# took; and `brain`, which until 2026-09-14 asked nothing at all and focused a
# brainer standing in the wrong directory as if it were fine. That asymmetry was
# the defect — `hw` refused to dispatch from `w7G:p1` (label=brain,
# cwd=<brain root>, expected <brain root>/setup) while `brain setup`
# focused it without a word.
#
# Prints `ok`, or `mismatch<TAB>label=… cwd=…`, or NOTHING when the pane is not
# in the list at all. The empty case is deliberate and load-bearing for the
# guard: at launch the pane was resolved FROM this list, so an absence there is
# an inconsistency rather than a verdict, and it must not read as a mismatch.
# `hw reports` reads a pane id baked months ago and treats empty as "gone".
lane_brainer_verdict() {  # $1 = pane id, $2 = expected brain cwd; stdin = herdr pane list
  HW_CHECK_PANE="$1" HW_CHECK_CWD="$2" python3 -c '
import json, os, sys
def canon(path):
    return os.path.realpath(path).rstrip("/").casefold()
try:
    panes = json.load(sys.stdin)["result"]["panes"]
except Exception:
    sys.exit(0)
want_id = os.environ["HW_CHECK_PANE"]
want = os.environ["HW_CHECK_CWD"]
for p in panes:
    if p.get("pane_id") != want_id:
        continue
    label_ok = p.get("label") == "brain"
    cwd_ok = canon(p.get("cwd") or "") == canon(want)
    if label_ok and cwd_ok:
        print("ok")
    else:
        print("mismatch\tlabel=%s cwd=%s" % (p.get("label") or "<none>", p.get("cwd") or "<none>"))
    sys.exit(0)
' 2>/dev/null
}

# ── Claude Code's first run, in an account nobody has used yet ──────────────
#
# MEASURED 2026-09-24 on a new macOS user: the installer, the guard, `brain` and
# a probe executor all worked, and each of them first stopped on a screen Claude
# Code shows ONCE per account — the welcome, the folder-trust question, the
# Bypass Permissions warning, the fullscreen-renderer offer. None of them says
# anything to the process that launched it; the pane just never becomes ready.
# So what those screens are, and how to tell one from another, is stated once
# here for install.sh, bin/brain and bin/hw.
#
# TWO OF THEM ARE THE PERSON'S, AND NOTHING HERE ANSWERS THEM. The login (it
# comes inside the welcome) and accepting the Bypass Permissions warning are a
# human's to give. These functions report them, with the command that clears
# them; they never write `bypassPermissionsModeAccepted`,
# `skipDangerousModePermissionPrompt` or `hasCompletedOnboarding`.

# ── permissions: skip (recommended) or ask ────────────────────────────────────
# Whether the agents hw and brain launch run with their permission prompts
# skipped. `skip` is the recommended default and what every earlier version did;
# `ask` leaves each vendor's own prompts on. ONE resolver for hw, brain and
# install.sh, so the three cannot disagree. Precedence, highest first:
#   1. the caller's flag value (`hw --permissions`, `install.sh --permissions`)
#   2. HW_PERMISSIONS in the environment
#   3. the machine file ($XDG_CONFIG_HOME/hw/permissions, one word: ask|skip)
#   4. the default, skip
# An unreadable or invalid machine file is NOT silently skip: it is reported.
hw_permissions_file() { printf '%s/hw/permissions' "${XDG_CONFIG_HOME:-$HOME/.config}"; }

# hw_permissions_resolve [<flag value>] — sets PERMISSIONS (ask|skip) and
# PERMISSIONS_SRC (where it came from). Returns 1, with PERMISSIONS_ERR set, when
# the value that won is neither ask nor skip.
hw_permissions_resolve() {
  local v="${1:-}" src="" f; PERMISSIONS_ERR=""
  f="$(hw_permissions_file)"
  if [ -n "$v" ]; then src="chosen (--permissions)"
  elif [ -n "${HW_PERMISSIONS:-}" ]; then v="$HW_PERMISSIONS"; src="HW_PERMISSIONS"
  elif [ -f "$f" ]; then v="$(tr -d '[:space:]' < "$f" 2>/dev/null || true)"; src="machine config $f"
  else v=skip; src="default (recommended)"; fi
  case "$v" in
    ask|skip) PERMISSIONS="$v"; PERMISSIONS_SRC="$src"; return 0 ;;
  esac
  PERMISSIONS=skip; PERMISSIONS_SRC="$src"
  PERMISSIONS_ERR="permissions must be ask or skip (got: '$v', from $src)"
  return 1
}

# The account's global config file. CLAUDE_CONFIG_DIR moves it inside that
# directory; without it Claude Code reads the legacy ~/.claude.json.
claude_account_json() {  # $1 = account config dir, empty for the default account
  if [ -n "${1:-}" ]; then printf '%s/.claude.json' "$1"; else printf '%s/.claude.json' "$HOME"; fi
}

# claude_first_run_pending [<config dir>] — one word per line for each once-per-
# account step still ahead: `onboarding` (the welcome, login included, never
# completed — `claude auth login` alone does not complete it) and `bypass` (the
# Bypass Permissions warning never accepted). Accepted means the account's
# `bypassPermissionsModeAccepted`, or `skipDangerousModePermissionPrompt` in
# its user settings, which is where accepting the warning records it today.
# Prints nothing when both are done.
claude_first_run_pending() {
  CLAUDE_FR_JSON="$(claude_account_json "${1:-}")" \
  CLAUDE_FR_SETTINGS="${1:-$HOME/.claude}/settings.json" python3 -c '
import json, os
def load(p):
    try:
        with open(p) as f:
            d = json.load(f)
        return d if isinstance(d, dict) else {}
    except Exception:
        return {}
g = load(os.environ["CLAUDE_FR_JSON"])
s = load(os.environ["CLAUDE_FR_SETTINGS"])
if g.get("hasCompletedOnboarding") is not True:
    print("onboarding")
if g.get("bypassPermissionsModeAccepted") is not True and s.get("skipDangerousModePermissionPrompt") is not True:
    print("bypass")
' 2>/dev/null || printf 'onboarding\nbypass\n'
}

# The ONE command that clears both steps, for a person to run in a terminal.
claude_first_run_command() {  # $1 = account config dir, empty for the default
  if [ -n "${1:-}" ]; then
    printf 'CLAUDE_CONFIG_DIR=%s claude --dangerously-skip-permissions' "$1"
  else
    printf 'claude --dangerously-skip-permissions'
  fi
}

# claude_startup_dialog — stdin is what a Claude Code pane shows; prints the
# first-run screen it is sitting on, or nothing. Matched on the dialog's own
# title text, as Claude Code 2.1.281 prints it.
claude_startup_dialog() {
  local screen
  screen="$(cat)"
  case "$screen" in
    *"Try the new fullscreen renderer?"*)                  printf 'fullscreen-offer' ;;
    *"running in Bypass Permissions mode"*)                printf 'bypass' ;;
    *"Is this a project you created or one you trust"*)    printf 'trust' ;;
    *"Choose the text style"*|*"Select login method"*)     printf 'onboarding' ;;
  esac
}

# What the person does about each screen, in one line.
claude_dialog_advice() {  # $1 = dialog, $2 = account config dir (may be empty)
  case "$1" in
    bypass)     printf 'the Bypass Permissions warning is waiting, and accepting it is yours: run `%s` once in a terminal, accept it, /exit, and try again' "$(claude_first_run_command "${2:-}")" ;;
    onboarding) printf 'Claude Code'"'"'s welcome (theme, then login) has never been completed in this account: run `%s` once in a terminal, finish it and accept the Bypass Permissions warning, /exit, and try again — `claude auth login` alone does not complete it' "$(claude_first_run_command "${2:-}")" ;;
    trust)      printf 'the folder-trust question is waiting: this directory is not marked trusted in %s' "$(claude_account_json "${2:-}")" ;;
    fullscreen-offer) printf 'the fullscreen-renderer offer is waiting; "Not now" (Esc) keeps the renderer as it is' ;;
  esac
}

# ── what every command of this toolchain says about itself ──────────────────
# `--version` is the same answer from hw and from brain, so it is one function.
# An installed brain reports what install.sh recorded in its marker; a tree that
# was never installed (a clone, or the brain itself) reports the tag HEAD sits
# on, else the short sha. Written for bash 3.2 and zsh: this file is sourced by both.
cli_version() {  # $1 = the root the command lives in
  local root="$1" v=""
  if [ -f "$root/.brain-install.json" ]; then
    v="$(jq -r '.version // ((.commit // "") | .[0:7]) // empty' "$root/.brain-install.json" 2>/dev/null || true)"
  fi
  if [ -z "$v" ] || [ "$v" = null ]; then
    v="$(git -C "$root" describe --tags --exact-match 2>/dev/null || git -C "$root" rev-parse --short HEAD 2>/dev/null || true)"
  fi
  printf '%s' "${v:-unknown}"
}
