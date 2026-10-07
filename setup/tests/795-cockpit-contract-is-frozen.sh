#!/usr/bin/env bash
# The cockpit contract's fixtures say what its schema says
#
# cockpit/schema.json, cockpit/verbs.txt and cockpit/fixtures/ are what the
# writer (hw), the preflight and the mod build against in parallel. Claims:
#
#   · every state and preflight fixture VALIDATES against schema.json (types,
#     enums, required and extra keys, patterns, lengths), except the fixtures
#     that are meant not to;
#   · every state fixture keeps the invariants a reader relies on: totals match
#     rows, ids are unique, rows are in attention order, an action that is false
#     has a why_not and one that is true has none, herdr down means no rows;
#   · fixtures/index.json gives each state fixture the class its own bytes earn
#     (generated_at against now_ms, per x-state-classes) and the number of rows
#     it draws, and every class has an example;
#   · verbs.txt is exactly done, ruling, reply, challenge-reply, receipt,
#     preflight, each line four columns, its argv starting `hw <verb>` (the two
#     reply verbs start `channel-send`, with the pane's own hold file), with the
#     confirm and stdin attributes the contract states;
#   · an ask row carries `pending_reply` (the hold file the reply verbs name),
#     a string exactly when the ask is delivered and answerable, else null;
#   · no contract byte has the shape of an engram id, a home path, an email or a
#     private address. Lane and person names are the export gate's (test 606).
#
# Run alone while working on this subject:
#     bash setup/tests/795-cockpit-contract-is-frozen.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

C="$ROOT/cockpit"
[ -d "$C" ] || { pass "795: this tree carries no cockpit"; exit 0; }

python3 -I - "$C" <<'PY' || fail "795: the cockpit contract and its fixtures disagree (see above)"
import json, os, re, sys
C = sys.argv[1]; F = os.path.join(C, "fixtures")
ORDER = ["challenge", "ask", "blocked", "report", "working", "idle", "gone"]
def bad(m): print("  ✗ " + m); sys.exit(1)

schema = json.load(open(os.path.join(C, "schema.json")))
DEFS = schema["$defs"]

def check(v, sc, path):
    """The subset of JSON Schema schema.json uses. Returns a list of errors."""
    if "$ref" in sc:
        e = check(v, DEFS[sc["$ref"].split("/")[-1]], path)
        return e
    errs = []
    if "oneOf" in sc:
        ok = [s for s in sc["oneOf"] if not check(v, s, path)]
        return [] if len(ok) == 1 else ["%s: matches %d of oneOf" % (path, len(ok))]
    if "const" in sc and v != sc["const"]: return ["%s: != %r" % (path, sc["const"])]
    if "enum" in sc and v not in sc["enum"]: return ["%s: %r not in enum" % (path, v)]
    t = sc.get("type")
    if t:
        ts = t if isinstance(t, list) else [t]
        py = {"object": dict, "array": list, "string": str, "boolean": bool, "null": type(None), "number": (int, float), "integer": int}
        ok = any(isinstance(v, py[x]) and not (x in ("integer", "number") and isinstance(v, bool)) for x in ts)
        if not ok: return ["%s: type %s, wanted %s" % (path, type(v).__name__, ts)]
    if isinstance(v, (int, float)) and not isinstance(v, bool):
        if "minimum" in sc and v < sc["minimum"]: errs.append("%s: below minimum" % path)
        if "maximum" in sc and v > sc["maximum"]: errs.append("%s: above maximum" % path)
    if isinstance(v, str):
        if "maxLength" in sc and len(v) > sc["maxLength"]: errs.append("%s: too long" % path)
        if "pattern" in sc and not re.search(sc["pattern"], v): errs.append("%s: pattern" % path)
    if isinstance(v, list):
        if "maxItems" in sc and len(v) > sc["maxItems"]: errs.append("%s: too many items" % path)
        if "items" in sc:
            for i, x in enumerate(v): errs += check(x, sc["items"], "%s[%d]" % (path, i))
    if isinstance(v, dict):
        for k in sc.get("required", []):
            if k not in v: errs.append("%s: missing %s" % (path, k))
        props = sc.get("properties", {})
        if sc.get("additionalProperties") is False:
            for k in v:
                if k not in props: errs.append("%s: unexpected %s" % (path, k))
        for k, x in v.items():
            if k in props: errs += check(x, props[k], path + "." + k)
    for a in sc.get("allOf", []):
        i = a.get("if"); th = a.get("then")
        if i is not None and not check(v, i, path) and th is not None: errs += check(v, th, path)
    return errs

def classify(d, size, now):
    """x-state-classes, in its order. d is None when unparsable."""
    if d is None or size > 1048576 or d.get("schema") != 1: return "dead"
    age = now - d["generated_at"]
    if age < 0 or age > 60000: return "dead"
    if age > 15000: return "stale"
    return "herdr-down" if not d["herdr"]["ok"] else "fresh"

index = json.load(open(os.path.join(F, "index.json")))["cases"]
indexed = {c["file"] for c in index if c["file"]}
for f in sorted(os.listdir(F)):
    if f == "index.json" or f.startswith("preflight-"): continue
    if f not in indexed: bad("fixtures/%s is not named by fixtures/index.json" % f)
if {c["class"] for c in index} != {"fresh", "stale", "dead", "herdr-down"}: bad("index.json lacks an example of a state class")

for c in index:
    if not c["file"]: got, n = "dead", 0
    else:
        p = os.path.join(F, c["file"])
        if not os.path.exists(p): bad("index names a missing fixture " + c["file"])
        raw = open(p).read()
        try: d = json.loads(raw)
        except ValueError: d = None
        size = len(raw) + (1048577 - len(raw) if "synthesized" in c.get("note", "") else 0)
        got = classify(d, size, c["now_ms"])
        n = len(d["rows"]) if got in ("fresh", "stale") else 0
    if got != c["class"]: bad("index: %s is declared %s but its bytes classify %s" % (c["file"], c["class"], got))
    if n != c["rows_rendered"]: bad("index: %s draws %d rows, declared %d" % (c["file"], n, c["rows_rendered"]))

NOT_VALID = {"dead-unparsable.txt", "dead-wrong-schema.json"}
for f in sorted(os.listdir(F)):
    p = os.path.join(F, f)
    if f == "index.json": continue
    if f == "dead-unparsable.txt":
        try: json.load(open(p)); bad(f + " parses, so it is not the unparsable example")
        except ValueError: pass
        continue
    d = json.load(open(p))
    if f.startswith("preflight-"):
        errs = check(d, DEFS["preflight"], f)
        if errs: bad("; ".join(errs[:3]))
        order = ["ok", "warn", "block"]
        top = max([order.index(r["level"]) for r in d["rules"]] or [0])
        if d["level"] != order[top]: bad(f + ": level is not the highest rule level")
        continue
    errs = check(d, DEFS["state"], f)
    if f in NOT_VALID:
        if not errs: bad(f + " validates, so it is not the invalid example")
        continue
    if errs: bad("; ".join(errs[:3]))
    rows = d["rows"]
    if d["totals"]["executors"] != len(rows): bad(f + ": totals.executors != rows")
    for t, a in (("working", "working"), ("idle", "idle"), ("asks", "ask"), ("reports", "report"), ("blocked", "blocked"), ("challenges", "challenge")):
        if d["totals"][t] != sum(1 for r in rows if r["attention"] == a): bad("%s: totals.%s" % (f, t))
    if d["totals"]["rulings_pending"] != sum(r["rulings_pending"]["count"] for r in rows): bad(f + ": totals.rulings_pending")
    ids = [r["id"] for r in rows]
    if len(set(ids)) != len(ids): bad(f + ": duplicate ids")
    key = [(ORDER.index(r["attention"]), r["attention_since"] or 0, r["id"]) for r in rows]
    if key != sorted(key): bad(f + ": rows are not in attention order")
    for r in rows:
        if r["id"] != r["project"] + ":" + r["task"]: bad(f + ": id is not project:task")
        a = r["actions"]
        for v in ("verify", "ruling", "done", "receipt"):
            if a[v] == (v in a["why_not"]): bad("%s %s: action %s true with why_not, or false without" % (f, r["id"], v))
        if r["agent_status"] == "idle" and r["attention"] != "blocked" and a["ruling"]:
            bad("%s %s: ruling true on an idle executor, which hw ruling refuses" % (f, r["id"]))
        if r["attention"] in ("ask", "challenge") and (not r["ask"] or r["ask"]["kind"] != r["attention"]): bad(f + ": attention/ask kind")
        if r["ask"] and (r["ask"]["state"] == "delivered") != isinstance(r["ask"]["pending_reply"], str): bad(f + " " + r["id"] + ": pending_reply is a string exactly when the ask is delivered")
        if r["attention"] in ("report", "blocked") and not r["report"]: bad(f + ": attention report without report")
        if r["ctx"]["pct"] is None and (r["ctx"]["tokens"] is not None or r["ctx"]["source"] is not None): bad(f + ": ctx half-null")

verbs = []
for l in open(os.path.join(C, "verbs.txt")):
    if not l.strip() or l.startswith("#"): continue
    cols = [x.strip() for x in l.split(" | ")]
    if len(cols) != 4: bad("verbs.txt: a line without four columns: " + l.strip())
    verbs.append(cols)
if [v[0] for v in verbs] != ["done", "ruling", "reply", "challenge-reply", "receipt", "preflight"]: bad("verbs.txt is not exactly done, ruling, reply, challenge-reply, receipt, preflight")
WANT = {"done": ("hw done <project> <task>", "confirm=yes", "stdin=no"),
        "ruling": ("hw ruling <pane> -", "confirm=no", "stdin=text"),
        "reply": ("channel-send --report --reply-hold <pending_reply> herdr <pane> - -", "confirm=no", "stdin=text"),
        "challenge-reply": ("channel-send --ruling --reply-hold <pending_reply> herdr <pane> - -", "confirm=no", "stdin=text"),
        "receipt": ("hw receipt <project> <task>", "confirm=no", "stdin=no"),
        "preflight": ("hw preflight --json -- <argv...>", "confirm=no", "stdin=no")}
for v in verbs:
    if tuple(v[1:]) != WANT[v[0]]: bad("verbs.txt: %s is %r, the contract says %r" % (v[0], v[1:], WANT[v[0]]))

pat = re.compile(r"#\d{3,}|/Users/|@[A-Za-z0-9-]+\.[a-z]{2,}|\b(?:10|172\.16|192\.168)\.\d+\.\d+")
for root, _, fs in os.walk(C):
    for f in fs:
        if pat.search(open(os.path.join(root, f)).read()): bad("%s holds a private-looking token" % os.path.join(root, f))
print("  contract consistent")
PY
pass "795: schema, verbs and fixtures agree; every state class has an example"
