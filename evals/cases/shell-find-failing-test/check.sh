#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
git diff --quiet HEAD || exit 1
reply | grep -qi 'discount' || exit 1
reply | grep -qiE 'percent|pct|/ ?100|fraction|10 ?%|1000|-1800|1800' || exit 1
