#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
python3 - <<'PY' || exit 1
import re, glob
n = 0
for p in glob.glob("handlers/*.py"):
    lines = open(p).read().split("\n")
    for i, l in enumerate(lines):
        if l.startswith("def handle_"):
            body = [x for x in lines[i+1:] if x.strip()]
            first = body[0]
            assert re.search(r"print\(|log", first), (p, l, first)
            n += 1
assert n == 4, n
PY
out=$(python3 main.py 2>&1 | tail -1)
[ "$out" = "{'user': 1} {'order': 'pen'}" ]
