#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
! grep -rq 'getUserName' --include='*.go' . || exit 1
[ "$(grep -rho 'fetchUserName' --include='*.go' . | wc -l)" -ge 5 ] || exit 1
[ "$(go run .)" = 'Hello, Ada Lovelace [Ada Lovelace] by Ada Lovelace' ]
