#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
! grep -rq 'invalid input' . --include='*.py' || exit 1
[ "$(python3 main.py)" = "error: bad input" ] || exit 1
grep -q 'invalid token' auth.py && grep -q 'missing key' store.py
