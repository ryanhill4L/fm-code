#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
! grep -rqw 'calc' --include='*.go' . || exit 1
grep -q 'func total(' calc.go || exit 1
[ "$(go run . 2>&1 | tr '\n' ' ')" = "6 0 " ]
