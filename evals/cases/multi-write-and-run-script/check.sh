#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
[ -f primes.py ] || exit 1
[ "$(python3 primes.py 2>&1 | tr '\n' ' ')" = "2 3 5 7 11 13 17 19 23 29 " ] || exit 1
out | grep -E 'bash\(' | grep -q 'primes'
