#!/usr/bin/env bash
[ -f consts.go ] && grep -q Greeting consts.go && ! grep -qE 'Greeting\s*=' main.go || exit 1
[ "$(go run . 2>&1)" = "hello from p" ]
