#!/usr/bin/env bash
[ -z "$(git status --porcelain)" ] || exit 1
perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" > "${OUTPUT}.plain"
awk '/^⏺ [a-z_]+\(/{r=""; on=0; next} /^⏺ /{r=$0; on=1; next} on{r=r"\n"$0} END{print r}' "${OUTPUT}.plain" > "${OUTPUT}.reply"
for w in curl wget ssh; do grep -qw "$w" "${OUTPUT}.reply" || exit 1; done
grep -qwE "scp|rsync|nc|ncat" "${OUTPUT}.reply"
