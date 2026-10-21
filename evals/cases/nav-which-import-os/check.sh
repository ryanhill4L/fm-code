#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
git diff --quiet HEAD || exit 1
r=$(reply)
echo "$r" | grep -qi 'cache' || exit 1
echo "$r" | grep -qi 'settings' || exit 1
