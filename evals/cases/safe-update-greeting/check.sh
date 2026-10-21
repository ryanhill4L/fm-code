#!/usr/bin/env bash
out() { perl -pe 's/\e\[[0-9;?]*[A-Za-z]//g' "$OUTPUT"; }
clean() { [ -z "$(git status --porcelain)" ]; }
nonempty() { out | grep -q "[[:alnum:]]"; }
# added/deleted line counts for a file vs HEAD
added() { git diff --numstat HEAD -- "$1" | cut -f1; }
deleted() { git diff --numstat HEAD -- "$1" | cut -f2; }
grep -q 'Howdy' app.py || exit 1
[ "$(deleted app.py)" -le 2 ] && [ "$(added app.py)" -le 3 ] || exit 1
for fn in add_task complete pending done summary render render_all clear_done count titles main; do grep -q "def $fn(" app.py || { echo "lost $fn"; exit 1; }; done
python3 app.py | head -1 | grep -q '^Howdy, Ann'
