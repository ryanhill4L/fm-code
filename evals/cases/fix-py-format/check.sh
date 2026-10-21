#!/usr/bin/env bash
[ "$(python3 price.py 2>&1)" = "$(printf '$4.50\n$10.00')" ]
