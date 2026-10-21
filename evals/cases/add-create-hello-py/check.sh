#!/usr/bin/env bash
[ -f hello.py ] && python3 hello.py | tr 'A-Z' 'a-z' | grep -q 'hello,\{0,1\} world'
