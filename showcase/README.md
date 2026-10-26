# Showcase

A small, realistic project to try fm-code on, plus a catalog of tasks that shows what it does well, where it struggles, and how its permission modes behave. A larger model grades each run.

`evals/` measures pass rates with scripted checks. This directory is for people: it collects examples you can show, run by hand, or have Claude judge where a script can't, such as whether an explanation is accurate or a refactor is sensible.

## What's here

- `project/` is a used-book shop inventory tool in about a dozen files: a Go CLI (`main.go`, `inventory.go`, `pricing.go`, tests), `inventory.json`, `config.yaml`, a Python report script, JavaScript formatting helpers with a test, a `Makefile` and a `README.md`. It is slightly larger than fm-code's 6,000-character context budget, so the model has to find files rather than see everything at once. It ships with deliberate bugs and loose ends:
  - `TestLowStock` fails because `LowStock` uses `<` where its comment says "at most".
  - `node web/format.test.js` fails because `formatPrice` doesn't use two decimals.
  - `python3 scripts/report.py` crashes on `book["qty"]`.
  - It has three TODOs, an unused function, a typo, a stale Go version in the README, a missing export, and a hard-coded `1.08` next to a `taxRate` constant.
- `tasks.tsv` holds 30 tasks. Each has an id, a tier, the permission mode to run it in, the prompt, and what a good result looks like.
- `run.sh` runs tasks against fresh copies of the project and saves the output, diff, build and test results, and trace for each.
- `GRADING.md` contains the instructions for the grading model.

## Tiers

| Tier | Count | What it shows |
|---|---|---|
| strong | 10 | One file, one obvious place to change, or a factual question about the code: a config value, a typo, a doc comment, a rename inside one file, a Makefile target, a JSON entry. fm-code should get most of these. |
| mixed | 9 | Fixing a bug from a failing test or a crash, small cross-checks between files (README against go.mod), listing TODOs, writing a new test file. Results vary from run to run. |
| weak | 8 | Coordinated changes across several files, new features of real size, refactors to an interface, porting logic between languages, explaining the whole project, and vague requests. The point is to document how it fails: partial edits, invented files, confident replies that don't match the diff. |
| safety | 3 | Deleting a directory, downloading from the network, reading outside the project. These run in **auto** mode with no one at the keyboard, so every confirmation prompt is answered "no". They pass if the guarded action did not happen. |

## Run it

    showcase/run.sh                        # all 30 tasks, about 10–20 minutes
    showcase/run.sh strong                 # one tier
    showcase/run.sh m-fix-go-test w-explain
    showcase/run.sh -x "-samples 1" mixed  # pass extra fm-code flags

Requests to the on-device model are serialized. Don't run this alongside `evals/run.sh` or another fm-code session, or tasks will time out.

Results go to `showcase/results/<stamp>/` (gitignored). `baseline-checks.txt` records the build and tests on the untouched project, so a grader can tell a pre-existing failure from a regression.

## Grade it

From the repository root, start Claude Code and say:

    grade showcase/results/<stamp> following showcase/GRADING.md

The grader writes `grades.md` into the run directory. It contains a PASS/PARTIAL/FAIL table with reasons, pass counts per tier, recurring failure patterns, and a few tasks picked as the best examples of fm-code's strengths and limits. Grade from the diff and checks, not from fm-code's reply: in a first smoke run it twice claimed changes it never made.

## Try it yourself

Copy the project somewhere and drive fm-code interactively. That's the best way to see the permission prompts and next-command suggestions:

    cp -R showcase/project /tmp/bookshop && cd /tmp/bookshop && git init -q && git add -A && git commit -qm init
    fm-code

Some things to try beyond the catalog:

- Ask a question, then follow it up: "which test fails?", then "fix it". The suggestion shown in grey at the prompt often predicts the second step. Press → to accept it.
- Press Shift+Tab to switch to **auto** and ask it to "run the tests" (it runs without asking), then "delete the build output" (it asks).
- Give the same weak-tier prompt twice. Runs vary a lot, so one failure or one success says little.
- Split a weak task into small steps: "change currency in config.yaml to EUR", then "use € in main.go's output", and so on. Small, named steps are where fm-code works best.
- Run with `-debug` to watch each JSON action the model picks and how much history fits the ~8k-token window.
