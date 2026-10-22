#!/usr/bin/env bash
grep -n "curl|wget" permissions.go | grep -q telnet && go test ./... >/dev/null 2>&1
