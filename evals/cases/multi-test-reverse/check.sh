#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
git diff --quiet HEAD -- strutil.go || exit 1
ls *_test.go >/dev/null 2>&1 || exit 1
grep -q 'Reverse(' *_test.go || exit 1
o=$(go test -v ./... 2>&1) || exit 1
echo "$o" | grep -q -- '--- PASS'
