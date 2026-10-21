#!/usr/bin/env bash
[ "$(go run . -upper -name bob 2>&1)" = "HELLO, BOB" ] && [ "$(go run . -name bob 2>&1)" = "hello, bob" ]
