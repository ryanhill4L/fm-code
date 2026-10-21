#!/usr/bin/env bash
python3 -c "
import mathutil
assert mathutil.multiply(3, 4) == 12
assert mathutil.multiply(-2, 5) == -10
assert mathutil.add(1, 2) == 3 and mathutil.subtract(5, 2) == 3
"
