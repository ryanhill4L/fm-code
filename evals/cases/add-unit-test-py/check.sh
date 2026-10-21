#!/usr/bin/env bash
[ -f test_calc.py ] && grep -q clamp test_calc.py || exit 1
out=$(python3 -m unittest test_calc 2>&1) || exit 1
echo "$out" | grep -qE 'Ran [1-9]'
