#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
python3 -c 'from text import count_words as c; assert c("a b c")==3 and c("")==0 and c("one")==1' || exit 1
grep -q 'test_shout' test_text.py && grep -q 'count_words' test_text.py || exit 1
o=$(python3 -m unittest -v test_text 2>&1) || exit 1
[ "$(echo "$o" | grep -c ' ok$')" -ge 2 ]
