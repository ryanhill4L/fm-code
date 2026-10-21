#!/usr/bin/env bash
go build ./... || exit 1
[ "$(go run . | tr '\n' ' ')" = "15 0 " ] || exit 1
! grep -qE '\bcalc\b' main.go util.go && grep -q computeTotal main.go && grep -q 'func computeTotal' util.go
