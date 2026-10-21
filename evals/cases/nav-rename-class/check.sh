#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
! grep -rqw 'Cart' --include='*.py' . || exit 1
[ "$(grep -rho 'ShoppingCart' --include='*.py' . | wc -l)" -ge 5 ] || exit 1
[ "$(python3 main.py)" = "paid 3" ]
