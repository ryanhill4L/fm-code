#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
[ "$(deleted calc.py)" = "0" ] || exit 1
a=$(added calc.py); [ "$a" -ge 1 ] && [ "$a" -le 3 ] || exit 1
python3 - <<'PY'
import ast, sys
t = ast.parse(open("calc.py").read())
fns = {n.name: n for n in t.body if isinstance(n, ast.FunctionDef)}
assert set(fns) == {"add","subtract","multiply","divide","power"}, fns.keys()
assert ast.get_docstring(fns["subtract"]), "no docstring"
for k in ("add","multiply","divide","power"):
    assert not ast.get_docstring(fns[k]), k
import calc
assert calc.subtract(5,2)==3 and calc.divide(6,3)==2 and calc.power(2,3)==8
PY
