#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
! grep -q '^import os' tool.py || exit 1
grep -q '^import sys' tool.py && grep -q '^import json' tool.py && grep -q '^import re' tool.py || exit 1
[ "$(deleted tool.py)" -le 2 ] && [ "$(added tool.py)" = "0" ] || exit 1
python3 tool.py | head -1 | grep -q "('a', '1')"
