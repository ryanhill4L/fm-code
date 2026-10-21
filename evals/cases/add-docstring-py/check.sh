#!/usr/bin/env bash
python3 -c "
import stats
d = stats.average.__doc__
assert d and len(d.strip()) > 5
assert stats.average([1, 2, 3]) == 2 and stats.total([1, 2]) == 3
"
