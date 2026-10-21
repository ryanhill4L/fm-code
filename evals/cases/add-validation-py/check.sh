#!/usr/bin/env bash
python3 -c "
import mathx
assert mathx.factorial(5) == 120 and mathx.factorial(0) == 1
try:
    mathx.factorial(-1)
except ValueError:
    pass
else:
    raise SystemExit(1)
"
