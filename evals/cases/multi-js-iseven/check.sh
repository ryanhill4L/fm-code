#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
grep -q 'isEven' util.js && grep -q 'isEven' main.js || exit 1
o=$(node main.js 2>&1) || exit 1
echo "$o" | grep -qx '8' && echo "$o" | grep -qx 'true'
