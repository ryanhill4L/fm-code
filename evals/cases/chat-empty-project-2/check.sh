#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
clean || exit 1
[ -z "$(ls -A | grep -v '^\.git$')" ] || exit 1
out | grep -qiE 'empty|no files|nothing|not (any|contain)|n.t (any|contain)|doesn.t (have|contain)|does not (have|contain)|no (source|code|project)'
