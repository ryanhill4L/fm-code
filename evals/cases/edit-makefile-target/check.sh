#!/usr/bin/env bash
[ "$(make run 2>/dev/null | tail -1)" = "verbose" ] && make -n test | grep -q unittest
