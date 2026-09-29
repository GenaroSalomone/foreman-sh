#!/usr/bin/env python3
"""The two WIDER versions of the `unmeasured` rule, kept so their counts can be
re-derived instead of believed.

`bin/lint-shell`'s `unmeasured` hazard is deliberately narrow, and the argument
for the narrowing is a pair of numbers: a naive version produced 111 findings
over `bin/` and a variable-flow version 142, against 4 for the shipped rule. A
rule at that volume is one people learn to scroll past.

BOTH JUDGMENT DAY JUDGES CALLED THOSE NUMBERS UNVERIFIABLE on 2026-09-07, and
they were right: the prototypes lived in a scratch directory and only the 4 was
reproducible from the tree. Reconstructions from their own regexes gave 13, 20,
45 and 56 — all honest, none the same, because a count means nothing without the
implementation that produced it. That is this whole line of work's subject: a
number printed as a property with no artifact behind it.

So the prototypes ship. They are NOT the rule and nothing calls them; they exist
so the argument for the rule's shape can be checked:

    python3 setup/guards/unmeasured-rule-prototypes.py <brain>/bin

Case A is the naive shape — any discarded-stderr substitution written inside a
test. Case B adds variable flow: assigned with stderr discarded, then read by a
later conditional. Run against `bin/` at 591675a they give 111 together; with
Case B scoped to the enclosing function it rises to 142, because scoping stops
the de-duplication by variable name rather than removing findings.

The shipped rule keeps Case A, drops Case B entirely, and adds three exclusions.
"""
import re, sys, glob, os

DEVNULL = re.compile(r"2>\s*/dev/null|2>&1")
SWALLOW = re.compile(r"\|\|\s*(?:true|:)\s*$")

def scan(path):
    src = open(path, encoding="utf-8", errors="replace").read()
    lines = src.split("\n")
    out = []
    # Case A: a substitution with 2>/dev/null written directly inside a test.
    for i, l in enumerate(lines):
        s = l.strip()
        if s.startswith("#"):
            continue
        if re.search(r"(?:\[\[?|if|while|until)\s[^\n]*\$\([^)]*2>\s*/dev/null", l):
            out.append((i + 1, "A", s))
    # Case B: assigned with stderr discarded, then read by a later test.
    assigns = {}
    for i, l in enumerate(lines):
        s = l.strip()
        if s.startswith("#"):
            continue
        m = re.match(r'^\s*(?:local\s+|export\s+)?([A-Za-z_][A-Za-z0-9_]*)="?\$\(', l)
        if m and DEVNULL.search(l):
            assigns.setdefault(m.group(1), i + 1)
    for var, ln in assigns.items():
        pat = re.compile(r'(?:\[\[?\s+-[nz]\s+"?\$\{?%s\b|case\s+"?\$\{?%s\b|\[\[?\s+"?\$\{?%s\}?"?\s*(?:=|!=))' % (var, var, var))
        for j, l in enumerate(lines):
            if j + 1 == ln or l.strip().startswith("#"):
                continue
            if pat.search(l):
                out.append((ln, "B", "%s -> tested at line %d: %s" % (lines[ln-1].strip()[:70], j + 1, l.strip()[:60])))
                break
    return out

total = 0
per = {}
for p in ([] if "--scoped" in sys.argv else sorted(glob.glob(sys.argv[1] + "/*"))):
    if not os.path.isfile(p):
        continue
    try:
        head = open(p, "rb").read(64)
    except OSError:
        continue
    if b"sh" not in head.split(b"\n")[0]:
        continue
    hits = scan(p)
    if hits:
        per[os.path.basename(p)] = hits
        total += len(hits)
for f, hits in per.items():
    print("== %s (%d)" % (f, len(hits)))
    for ln, kind, txt in hits:
        print("   %s:%s [%s] %s" % (f, ln, kind, txt[:120]))
if "--scoped" not in sys.argv:
    print("\nTOTAL:", total)


# ── THE FUNCTION-SCOPED VARIANT (the 142) ──────────────────────────────────
#
# Case B above de-duplicates by variable NAME across the whole file, which both
# hides real pairs and invents cross-function ones (`found` assigned in one
# function, "tested" in another 3000 lines away). Scoping the search to the
# enclosing function fixes the false pairs and RAISES the count, because the
# de-duplication goes away. 142 over `bin/` at 591675a — and 142 findings is not
# a rule, it is a wall. That is the number that settled the shape of the shipped
# one.
#
#     python3 setup/guards/unmeasured-rule-prototypes.py <dir> --scoped

FUNC = re.compile(r"^\s*(?:function\s+)?([A-Za-z_][A-Za-z0-9_:-]*)\s*\(\)\s*\{")

def func_spans(lines):
    """(start, end) line indices for each top-level function body."""
    spans = []
    i = 0
    while i < len(lines):
        if FUNC.match(lines[i]):
            depth = 0
            j = i
            while j < len(lines):
                depth += lines[j].count("{") - lines[j].count("}")
                if depth <= 0 and j > i:
                    break
                j += 1
            spans.append((i, j))
            i = j + 1
        else:
            i += 1
    return spans

def enclosing(spans, idx):
    best = None
    for a, b in spans:
        if a <= idx <= b and (best is None or a > best[0]):
            best = (a, b)
    return best

def scan(path):
    lines = open(path, encoding="utf-8", errors="replace").read().split("\n")
    spans = func_spans(lines)
    A, B = [], []
    for i, l in enumerate(lines):
        if l.strip().startswith("#"):
            continue
        if re.search(r"(?:\[\[?|if|while|until)\s[^\n]*\$\([^)]*2>\s*/dev/null", l):
            A.append((i + 1, l.strip()))
    seen = set()
    for i, l in enumerate(lines):
        if l.strip().startswith("#"):
            continue
        m = re.match(r'^\s*(?:local\s+|export\s+)?([A-Za-z_][A-Za-z0-9_]*)="?\$\(', l)
        if not (m and re.search(r"2>\s*/dev/null|2>&1", l)):
            continue
        var = m.group(1)
        sp = enclosing(spans, i)
        lo, hi = sp if sp else (0, len(lines) - 1)
        pat = re.compile(r'(?:\[\[?\s+-[nz]\s+"?\$\{?%s\b|case\s+"?\$\{?%s\b|\[\[?\s+"?\$\{?%s\}?"?\s*(?:=|!=))' % (var, var, var))
        for j in range(lo, min(hi + 1, len(lines))):
            if j == i or lines[j].strip().startswith("#"):
                continue
            if pat.search(lines[j]):
                key = (path, i + 1)
                if key not in seen:
                    seen.add(key)
                    B.append((i + 1, lines[i].strip(), j + 1, lines[j].strip()))
                break
    return A, B

if "--scoped" not in sys.argv:
    raise SystemExit(0)

ta = tb = 0
for p in sorted(glob.glob(sys.argv[1] + "/*")):
    if not os.path.isfile(p):
        continue
    try:
        first = open(p, "rb").readline()
    except OSError:
        continue
    if b"sh" not in first:
        continue
    A, B = scan(p)
    if A or B:
        print("== %s   A=%d B=%d" % (os.path.basename(p), len(A), len(B)))
        for ln, t in A:
            print("   A %s:%s  %s" % (os.path.basename(p), ln, t[:110]))
    ta += len(A); tb += len(B)
print("\nCASE A (substitution inside a test):", ta)
print("CASE B (assigned then tested, same function):", tb)
