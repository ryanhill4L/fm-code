#!/usr/bin/env bash
# reply = text after the last tool-call line, ANSI stripped
reply() { perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" | awk '/^⏺ [a-z_]+\(/{buf=""; next} {buf=buf"\n"$0} END{print buf}'; }
python3 - <<'PY' || exit 1
from validators import *
assert is_valid_email("") is False or not is_valid_email("")
assert is_valid_email("a@b.com")
assert not is_valid_email("a b@c.com")
assert is_valid_phone("5551234567") and not is_valid_phone("")
assert not is_valid_zip("") 
PY
