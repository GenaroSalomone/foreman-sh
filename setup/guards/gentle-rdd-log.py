#!/usr/bin/env python3
"""A LOCAL, one-line-per-run log of the RDD reviewer roles of `--sdd gentle`.

Registered by `hw gentle-home` as a SubagentStop hook in rdd-settings.json, for
an executor dispatched with `review: rdd` only. Telemetry stays off: this
reads the hook payload and the subagent's own transcript on disk, appends one
JSON line to $HW_ARTIFACTS/rdd-log.jsonl, and makes no network call. It never blocks
(exit 0 whatever happens) and ignores every subagent that is not a review role.

Line: ts, role, model, duration_s, tokens {input, output, cache_read,
cache_creation}, agent_transcript_path, verdict (clean | the highest severity
the role returned | unparsed).
"""
import json
import os
import sys
from datetime import datetime

ROLES = ("review-risk", "review-reliability", "review-resilience",
         "review-readability", "review-refuter", "review-validator")
RANK = ["SUGGESTION", "WARNING", "CRITICAL", "BLOCKER"]


def role_of(agent_type):
    base = (agent_type or "").split(":")[-1]
    return base if base in ROLES else None


def parse_ts(s):
    try:
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except Exception:
        return None


def transcript_facts(path):
    model, first, last = None, None, None
    tok = {"input": 0, "output": 0, "cache_read": 0, "cache_creation": 0}
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            for raw in f:
                try:
                    e = json.loads(raw)
                except ValueError:
                    continue
                t = parse_ts(e.get("timestamp") or "")
                if t:
                    first = first or t
                    last = t
                m = e.get("message") or {}
                if isinstance(m, dict) and m.get("role") == "assistant":
                    model = m.get("model") or model
                    u = m.get("usage") or {}
                    tok["input"] += u.get("input_tokens", 0) or 0
                    tok["output"] += u.get("output_tokens", 0) or 0
                    tok["cache_read"] += u.get("cache_read_input_tokens", 0) or 0
                    tok["cache_creation"] += u.get("cache_creation_input_tokens", 0) or 0
    except OSError:
        pass
    dur = round((last - first).total_seconds(), 1) if first and last else None
    return model, dur, tok


def verdict_of(text):
    try:
        data = json.loads(text)
        sev = [f.get("severity") for f in data.get("findings", [])]
    except Exception:
        return "unparsed"
    sev = [s for s in sev if s in RANK]
    return max(sev, key=RANK.index) if sev else "clean"


def main():
    try:
        p = json.load(sys.stdin)
    except Exception:
        return
    role = role_of(p.get("agent_type"))
    out = os.environ.get("HW_ARTIFACTS")
    if not role or not out:
        return
    tp = p.get("agent_transcript_path") or ""
    model, dur, tok = transcript_facts(tp)
    line = {"ts": datetime.now().astimezone().isoformat(timespec="seconds"), "role": role,
            "model": model, "duration_s": dur, "tokens": tok,
            "agent_transcript_path": tp, "verdict": verdict_of(p.get("last_assistant_message") or "")}
    try:
        with open(os.path.join(out, "rdd-log.jsonl"), "a", encoding="utf-8") as f:
            f.write(json.dumps(line, ensure_ascii=False) + "\n")
    except OSError:
        pass


if __name__ == "__main__":
    try:
        main()
    finally:
        sys.exit(0)
