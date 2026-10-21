#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
git diff --quiet HEAD -- test_stats.py || exit 1
python3 -m unittest -q test_stats >/dev/null 2>&1 || exit 1
out | grep -E 'bash\(' | grep -qE 'unittest|pytest|test_stats|python3? '
