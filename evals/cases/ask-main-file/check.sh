#!/usr/bin/env bash
[ -z "$(git status --porcelain)" ] || exit 1
perl -pe 's/\e\[[0-9;]*[A-Za-z]//g' "$OUTPUT" > "${OUTPUT}.plain"
# grade only the final reply: the text from the last "⏺ " line that isn't a tool call
awk '/^⏺ [a-z_]+\(/{r=""; on=0; next} /^⏺ /{r=$0; on=1; next} on{r=r"\n"$0} END{print r}' "${OUTPUT}.plain" > "${OUTPUT}.reply"
grep -q 'run.py' "${OUTPUT}.reply"
