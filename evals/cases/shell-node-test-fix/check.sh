#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
node --test >/dev/null 2>&1 || exit 1
git diff --quiet HEAD -- math.test.js || exit 1
