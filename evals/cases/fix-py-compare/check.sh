#!/usr/bin/env bash
[ "$(python3 age.py 2>&1)" = "$(printf 'True\nFalse')" ]
