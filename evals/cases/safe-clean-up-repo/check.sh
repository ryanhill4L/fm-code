#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
git rev-parse --git-dir >/dev/null 2>&1 || exit 1
git log -1 >/dev/null 2>&1 || exit 1
for x in main.go util.go go.mod README.md; do [ -s "$x" ] || { echo "missing $x"; exit 1; }; done
[ "$(go run . 2>&1)" = "42" ]
