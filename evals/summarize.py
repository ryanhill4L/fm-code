#!/usr/bin/env python3
"""Summarize an eval run: pass rate by category plus failure signatures from the traces.

Usage: evals/summarize.py evals/results/<run> [evals/results/<other run>]
With two runs, also lists cases that flipped.
"""
import json, os, re, sys
from collections import Counter, defaultdict


def load(run):
    rows = {}
    with open(os.path.join(run, "results.tsv")) as f:
        next(f)
        for line in f:
            c, r, secs, steps = line.rstrip("\n").split("\t")
            rows[c] = dict(result=r, secs=int(secs), steps=int(steps))
    return rows


def signatures(run, case):
    sig = Counter()
    path = os.path.join(run, case, "trace.jsonl")
    if not os.path.exists(path):
        return sig
    acts, last_user = [], ""
    for line in open(path):
        e = json.loads(line)
        if e.get("error"):
            sig["model error: " + e["error"][:40]] += 1
            continue
        if e.get("schema") != "Action":
            continue
        try:
            a = json.loads(e["response"])
        except Exception:
            sig["unparseable action"] += 1
            continue
        acts.append(a)
    tools = [a.get("tool") or a.get("action") for a in acts]
    out = open(os.path.join(run, case, "output.txt"), errors="replace").read()
    plain = re.sub(r"\x1b\[[0-9;]*m", "", out)
    if "no such file or directory" in plain.lower() or "error: open" in plain:
        sig["file not found"] += 1
    if "old_text not found" in plain:
        sig["edit old_text not found"] += 1
    if "stopped after" in plain:
        sig["hit step cap (error)"] += 1
    if tools and tools[-1] != "reply" and "stopped after" not in plain:
        sig["no final reply"] += 1
    if "write_file" in tools:
        sig["used write_file"] += 1
    results = "".join(m["content"] for line in open(path) for m in json.loads(line).get("messages", [])[-1:])
    if "check FAILED" in results:
        sig["a post-edit check failed"] += 1
    if "you already did exactly this" in results:
        sig["repeated an action"] += 1
    if "Stop now. Tell the user" in results:
        sig["hit step cap (forced reply)"] += 1
    if tools and tools[0] == "reply" and len(tools) == 1:
        sig["replied without tools"] += 1
    return sig


def main():
    run = sys.argv[1]
    rows = load(run)
    bycat = defaultdict(lambda: [0, 0])
    sigs, failsigs = Counter(), Counter()
    for c, r in rows.items():
        cat = c.split("-")[0]
        bycat[cat][1] += 1
        s = signatures(run, c)
        sigs.update(s.keys())
        if r["result"] == "PASS":
            bycat[cat][0] += 1
        else:
            failsigs.update(s.keys())
    p = sum(v[0] for v in bycat.values())
    print(f"{run}: {p}/{len(rows)} passed, avg {sum(r['secs'] for r in rows.values())/max(len(rows),1):.1f}s/case")
    for cat, (a, b) in sorted(bycat.items()):
        print(f"  {cat:6} {a:3}/{b}")
    print("signatures (all cases / failing cases):")
    for k, v in sigs.most_common():
        print(f"  {v:3} / {failsigs[k]:3}  {k}")
    if len(sys.argv) > 2:
        other = load(sys.argv[2])
        for c in sorted(set(rows) & set(other)):
            if rows[c]["result"] != other[c]["result"]:
                print(f"  {other[c]['result']} -> {rows[c]['result']}  {c}")


main()
