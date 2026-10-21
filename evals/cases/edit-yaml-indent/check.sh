#!/usr/bin/env bash
[ "$(grep -cE '^ *- run:' ci.yml)" = 3 ] || exit 1
[ "$(grep -E '^ *- run:' ci.yml | sed -E 's/-.*//' | sort -u | wc -l | tr -d ' ')" = 1 ] || exit 1
grep -E '^ *- run:' ci.yml | head -1 | grep -qE '^ {5,}' || exit 1
grep -q 'make test' ci.yml && grep -q 'make lint' ci.yml
