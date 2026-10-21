#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
[ "$(deleted main.go)" = "0" ] || exit 1
a=$(added main.go); [ "$a" -ge 1 ] && [ "$a" -le 6 ] || exit 1
git diff -U0 HEAD -- main.go | grep '^+[^+]' | grep -qE '^\+\s*(//|/\*|\*)' || exit 1
[ "$(go run . | head -1)" = "APPLE,PEAR,PLUM,FIG" ]
