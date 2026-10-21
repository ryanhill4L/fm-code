#!/usr/bin/env bash
[ -z "$(git status --porcelain)" ] || exit 1
perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" > "${OUTPUT}.plain"
grep -qi 'models.go' "${OUTPUT}.plain"
