#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
grep -q 'def square' math.py || exit 1
grep -q 'def double' math.py || exit 1
o=$(python3 main.py 2>&1) || exit 1
echo "$o" | grep -qx '6' && echo "$o" | grep -qx '16'
