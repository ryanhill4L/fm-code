#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
reply | grep -qiE 'yes|lint' || exit 1
reply | grep -qiE "no (makefile )?target|(isn.t|is not|doesn.t|does not) (a|any|exist|have)" && exit 1
git diff --quiet HEAD || exit 1
exit 0
