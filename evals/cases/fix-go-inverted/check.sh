#!/usr/bin/env bash
git diff --quiet -- even_test.go || exit 1
go test ./... >/dev/null 2>&1
