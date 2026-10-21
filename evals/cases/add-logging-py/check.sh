#!/usr/bin/env bash
grep -q 'import logging' app.py && ! grep -qE '^\s*print\(' app.py || exit 1
python3 -c "
import io, logging
s = io.StringIO()
logging.basicConfig(level=logging.DEBUG, stream=s, force=True)
import app
assert app.run([1, 2]) == [2, 4]
assert 'processing' in s.getvalue()
"
