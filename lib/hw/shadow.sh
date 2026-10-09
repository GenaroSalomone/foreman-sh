# lib/hw/shadow.sh — `hw shadow`: the switch that puts shadow in every pane, and the clock of its 7-day gate.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/shadow.sh). A library: nothing runs on source. It uses the
# shim's own helpers (_core_impl_file, _core_shadow_calls, _core_find, _core_stale), defined earlier in bin/hw.
#
#   hw shadow            the report: the switch, hw-core, and per verb the days, calls and diffs
#   hw shadow on         write the switch file (`shadow`), building hw-core first when it is missing or stale
#   hw shadow off        remove the switch file: bash again, the history is kept
#
# THE CLOCK. setup/attachments/migracion-hw.md §5a asks for 7 days of live shadow with shadow.jsonl empty. Per verb
# that is: whole days since the later of its first shadow call and its last difference. A difference restarts it.
# The calls come from shadow.calls.tsv (one line per shadow call, equal or not, written by the shim); the
# differences from shadow.jsonl. Neither file is ever rewritten by this verb.
#
# NOT EVERY PORTED VERB IS SHADOWED. chaining-lease-expire (a pane closed) and outbox (a message sent) have an
# effect outside the run directory, so `shadow` runs them in bash alone: they never accumulate, and the report
# says so instead of showing a zero that reads as "not yet".

_SHADOW_VERBS="receipt log handle help worktrees status ruling human-boundary executor-turn-end chaining-lease-start chaining-lease-expire outbox output stage"
_SHADOW_BASH_ONLY="chaining-lease-expire outbox"

_shadow_core_state() {  # → "fresh <path>" | "stale <path>" | "missing"
  local b
  b="$(_core_find)"
  if [ -z "$b" ]; then printf 'missing'
  elif [ -z "${HW_CORE:-}" ] && _core_stale "$b"; then printf 'stale %s' "$b"
  else printf 'fresh %s' "$b"; fi
}

_shadow_report() {
  local f state calls jsonl
  f="$(_core_impl_file)"
  state="$(_shadow_core_state)"
  calls="$(_core_shadow_calls)"
  jsonl="${HW_SHADOW_JSONL:-${XDG_STATE_HOME:-${HOME:-/nonexistent}/.local/state}/hw/shadow.jsonl}"
  printf 'switch    %s  ' "$f"
  if [ -r "$f" ]; then printf '→ %s\n' "$(tr '\n' ' ' <"$f" | sed 's/  */ /g; s/ $//')"; else printf '→ absent (bash)\n'; fi
  case "$state" in
    fresh*) printf 'hw-core   fresh  %s\n' "${state#fresh }" ;;
    stale*) printf 'hw-core   STALE — core/ has sources newer than %s: hw falls back to bash and NO shadow accumulates. `hw shadow on` rebuilds it (setup/build-core --native)\n' "${state#stale }" ;;
    *)      printf 'hw-core   MISSING — hw falls back to bash and NO shadow accumulates. `hw shadow on` builds it (setup/build-core --native)\n' ;;
  esac
  printf 'calls     %s\ndiffs     %s\n\n' "$calls" "$jsonl"
  python3 -I - "$calls" "$jsonl" "$_SHADOW_VERBS" "$_SHADOW_BASH_ONLY" <<'PY'
import sys, json, datetime as dt
calls_f, jsonl_f, verbs, bash_only = sys.argv[1], sys.argv[2], sys.argv[3].split(), sys.argv[4].split()
now = dt.datetime.now(dt.timezone.utc)

def ts(s):
    try:
        return dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)
    except ValueError:
        return None

st = {}
def row(v):
    return st.setdefault(v, {"calls": 0, "days": set(), "first": None, "last": None, "diffs": 0, "ldiff": None})

try:
    for line in open(calls_f, encoding="utf-8", errors="replace"):
        parts = line.rstrip("\n").split("\t")
        if len(parts) < 2 or ts(parts[0]) is None:
            continue
        t, r = ts(parts[0]), row(parts[1])
        r["calls"] += 1; r["days"].add(t.date())
        r["first"] = t if r["first"] is None or t < r["first"] else r["first"]
        r["last"] = t if r["last"] is None or t > r["last"] else r["last"]
except OSError:
    pass
try:
    for line in open(jsonl_f, encoding="utf-8", errors="replace"):
        try:
            d = json.loads(line)
        except ValueError:
            continue
        if not isinstance(d, dict) or not isinstance(d.get("verb"), str):
            continue
        t, r = ts(d.get("at", "")), row(d["verb"])
        r["diffs"] += 1
        if t is not None and (r["ldiff"] is None or t > r["ldiff"]):
            r["ldiff"] = t
except OSError:
    pass

def day(t):
    return t.strftime("%Y-%m-%d") if t else "-"

print("%-22s %6s %5s %-10s %5s %-10s %6s  %s" % ("verb", "calls", "days", "since", "diffs", "last diff", "clean", "gate (7 clean days)"))
for v in verbs + sorted(k for k in st if k not in verbs):
    r = st.get(v)
    if v in bash_only:
        print("%-22s %6s %5s %-10s %5s %-10s %6s  %s" % (v, "-", "-", "-", "-", "-", "-", "bash only under shadow (its effect is outside the run dir)"))
        continue
    if not r or r["calls"] == 0:
        print("%-22s %6d %5d %-10s %5d %-10s %6s  %s" % (v, 0, 0, "-", r["diffs"] if r else 0, day(r["ldiff"]) if r else "-", "-", "no shadow yet"))
        continue
    start = max(t for t in (r["first"], r["ldiff"]) if t is not None)
    clean = max(0, (now - start).days)
    if r["ldiff"] is not None and r["ldiff"] >= r["first"]:
        why = "DIFF — the clock restarted at the last one"
    else:
        why = ""
    gate = ("OK" if clean >= 7 else "%d more day%s" % (7 - clean, "" if 7 - clean == 1 else "s")) + (" · " + why if why else "")
    print("%-22s %6d %5d %-10s %5d %-10s %6d  %s" % (v, r["calls"], len(r["days"]), day(r["first"]), r["diffs"], day(r["ldiff"]), clean, gate))
PY
}

cmd_shadow() {
  local sub="${1:-report}" f state
  case "$sub" in
    report|"") _shadow_report ;;
    on|off)
      [ "${_HW_CALLER_EXECUTOR:-0}" != 1 ] || die "hw shadow $sub is a brainer verb: an executor reads the report, it does not flip the switch for every pane"
      f="$(_core_impl_file)"
      if [ "$sub" = off ]; then
        rm -f "$f" && info "shadow is off: $f removed, so the bash implementation answers (the history in $(_core_shadow_calls) is kept)"
        return 0
      fi
      state="$(_shadow_core_state)"
      case "$state" in
        fresh*) ;;
        *)
          if [ -x "$HW_BIN_DIR/../setup/build-core" ] && command -v go >/dev/null 2>&1; then
            info "hw-core is ${state%% *}: building it (setup/build-core --native)"
            "$HW_BIN_DIR/../setup/build-core" --native >&2 || die "setup/build-core --native failed, so shadow would not run: the switch is NOT written"
          else
            die "hw-core is ${state%% *} and it cannot be built here (needs the go toolchain and setup/build-core beside bin/): the switch is NOT written, because shadow would silently run bash"
          fi
          state="$(_shadow_core_state)"
          case "$state" in fresh*) ;; *) die "hw-core is still ${state%% *} after the build: the switch is NOT written" ;; esac
          ;;
      esac
      mkdir -p "$(dirname "$f")" && printf '# hw: the default mode of the ported verbs (bash, go or shadow); `hw shadow off` removes this file\nshadow\n' >"$f"
      info "shadow is on: $f → shadow; every ported verb in every pane runs bash and Go and logs to $(_core_shadow_calls)"
      info "read it with: hw shadow"
      ;;
    -h|--help|help) printf 'usage: hw shadow [on|off]\n  (no argument)  the report: the switch, hw-core, and per verb the days, calls and diffs\n  on             write the switch file (shadow), building hw-core first when it is missing or stale\n  off            remove the switch file: bash again, the history kept\n' ;;
    *) die "unknown: hw shadow $sub (usage: hw shadow [on|off])" ;;
  esac
}
