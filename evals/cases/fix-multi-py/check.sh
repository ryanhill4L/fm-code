#!/usr/bin/env bash
[ "$(python3 main.py 2>&1)" = "$(printf 'area: 12\nperimeter: 14')" ]
