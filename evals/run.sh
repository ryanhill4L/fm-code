#!/usr/bin/env bash
# Runs fm-code against every eval case (or the ones named) and scores each with its check.sh.
#
# A case is a directory under evals/cases/<name>/ holding:
#   prompt.txt  what the user types
#   files/      the starting project (copied to a fresh temp dir)
#   check.sh    run in the project dir afterwards; exit 0 means pass.
#               $OUTPUT is the path to fm-code's captured stdout+stderr.
#
# Usage: evals/run.sh [-o outdir] [-x "extra fm-code flags"] [case...]
# The model serializes requests, so cases run one at a time.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(dirname "$here")
out="$here/results/$(date +%Y%m%d-%H%M%S)"
extra=""
while getopts "o:x:" opt; do
  case $opt in
    o) out=$OPTARG ;;
    x) extra=$OPTARG ;;
    *) exit 2 ;;
  esac
done
shift $((OPTIND - 1))
mkdir -p "$out" && out=$(cd "$out" && pwd)
(cd "$repo" && go build -o "$out/fm-code" .) || exit 1

cases=("$@")
if [ ${#cases[@]} -eq 0 ]; then
  for d in "$here"/cases/*/; do cases+=("$(basename "$d")"); done
fi

pass=0; total=0
printf "case\tresult\tsecs\tsteps\n" > "$out/results.tsv"
for c in "${cases[@]}"; do
  src="$here/cases/$c"
  [ -f "$src/prompt.txt" ] || { echo "skip $c (no prompt.txt)"; continue; }
  work=$(mktemp -d)
  [ -d "$src/files" ] && cp -R "$src/files/." "$work/"
  (cd "$work" && git init -q && git add -A && git -c user.email=e@e -c user.name=e commit -qm init --allow-empty) >/dev/null 2>&1
  log="$out/$c"; mkdir -p "$log"
  start=$(date +%s)
  # shellcheck disable=SC2086
  (cd "$work" && perl -e 'alarm shift; exec @ARGV' 240 "$out/fm-code" -yes -no-suggest -trace "$log/trace.jsonl" $extra "$(cat "$src/prompt.txt")" </dev/null >"$log/output.txt" 2>&1)
  secs=$(( $(date +%s) - start ))
  if (cd "$work" && OUTPUT="$log/output.txt" bash "$src/check.sh" >"$log/check.txt" 2>&1); then
    r=PASS; pass=$((pass + 1))
  else
    r=FAIL
  fi
  (cd "$work" && git diff --stat HEAD && git status --short) >"$log/diff.txt" 2>&1
  (cd "$work" && git diff HEAD) >>"$log/diff.txt" 2>&1
  steps=$(grep -c '"schema":"Action"' "$log/trace.jsonl" 2>/dev/null || echo 0)
  total=$((total + 1))
  printf "%s\t%s\t%s\t%s\n" "$c" "$r" "$secs" "$steps" | tee -a "$out/results.tsv"
  rm -rf "$work"
done
echo "passed $pass/$total  ->  $out"
