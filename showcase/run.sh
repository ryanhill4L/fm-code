#!/usr/bin/env bash
# Runs fm-code on showcase tasks and saves everything a grader needs to judge them.
#
# Each task runs in a fresh copy of showcase/project/ (git-initialised so the diff is
# exact). Results land in showcase/results/<stamp>/<task-id>/:
#   task.txt     the prompt, tier, mode and what a good result looks like
#   output.txt   fm-code's stdout+stderr
#   diff.txt     every file it changed, created or deleted
#   checks.txt   go build/vet/test, node and python runs on the project afterwards
#   trace.jsonl  every model request and reply
# baseline-checks.txt in the run directory holds the same checks on the untouched project.
#
# Usage: showcase/run.sh [-o outdir] [-x "extra fm-code flags"] [task-id | tier ...]
#   showcase/run.sh                    # all tasks
#   showcase/run.sh strong safety      # whole tiers
#   showcase/run.sh m-fix-go-test      # one task
# The model serializes requests, so tasks run one at a time.
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

checks() {
  echo "== go build"; go build -o /dev/null . 2>&1 && echo ok
  echo "== go vet"; go vet ./... 2>&1 && echo ok
  echo "== go test"; go test ./... 2>&1 | tail -15
  echo "== go run ."; perl -e 'alarm shift; exec @ARGV' 30 go run . 2>&1
  echo "== node web/format.test.js"; node web/format.test.js 2>&1 | tail -5
  echo "== python3 scripts/report.py"; python3 scripts/report.py 2>&1 | tail -10
}

fresh() {
  local work; work=$(mktemp -d)
  cp -R "$here/project/." "$work/"
  (cd "$work" && git init -q && git add -A && git -c user.email=e@e -c user.name=e commit -qm init) >/dev/null 2>&1
  echo "$work"
}

base=$(fresh)
(cd "$base" && checks) >"$out/baseline-checks.txt" 2>&1
rm -rf "$base"

ran=0
while IFS=$'\t' read -r id tier mode prompt expected; do
  [ "$id" = id ] && continue
  if [ $# -gt 0 ]; then
    want=0
    for a in "$@"; do [ "$a" = "$id" ] || [ "$a" = "$tier" ] && want=1; done
    [ $want = 1 ] || continue
  fi
  log="$out/$id"; mkdir -p "$log"
  printf "id: %s\ntier: %s\nmode: %s\nprompt: %s\nexpected: %s\n" "$id" "$tier" "$mode" "$prompt" "$expected" >"$log/task.txt"
  work=$(fresh)
  printf "%-22s %-7s ..." "$id" "$tier"
  start=$(date +%s)
  # stdin is empty, so in ask or auto mode any confirmation prompt is answered "no".
  # shellcheck disable=SC2086
  (cd "$work" && perl -e 'alarm shift; exec @ARGV' 240 "$out/fm-code" -mode "$mode" -no-suggest -trace "$log/trace.jsonl" $extra "$prompt" </dev/null >"$log/output.txt" 2>&1)
  secs=$(( $(date +%s) - start ))
  perl -pi -e 's/\e\[[0-9;]*m//g' "$log/output.txt" # drop colours so graders read plain text
  (cd "$work" && git add -A && git diff --cached --stat HEAD && git diff --cached HEAD) >"$log/diff.txt" 2>&1
  (cd "$work" && checks) >"$log/checks.txt" 2>&1
  steps=$(grep -c '"schema":"Action"' "$log/trace.jsonl" 2>/dev/null || echo 0)
  echo "secs: $secs  steps: $steps" >>"$log/task.txt"
  printf "\r%-22s %-7s %4ss %3s steps\n" "$id" "$tier" "$secs" "$steps"
  rm -rf "$work"
  ran=$((ran + 1))
done <"$here/tasks.tsv"
echo "ran $ran tasks  ->  $out"
echo "grade with: claude \"grade $out following showcase/GRADING.md\""
