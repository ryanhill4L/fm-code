#!/usr/bin/env bash
python3 -c "
import config as c
assert c.DEBUG is False and c.TIMEOUT == 60 and c.HOST == 'localhost' and c.RETRIES == 3
"
