# Harness changes and eval history

This file records how fm-code's harness changed to get more out of the on-device Apple Foundation Model, and what each change scored. The model itself never changed. Every gain below comes from the harness: the prompts, the tools, the context and the checks around the model.

All work is on the `harness-improvements` branch. Scores come from `evals/run.sh`, with 101 cases (105 from v8 on) run one at a time against the real model. Run `python3 evals/summarize.py <run> [older-run]` to get the per-category breakdown and the cases that flipped.

> **How to read the numbers.** Runs that behave almost the same still flip about 20 cases: v5 and v6 differ only in features that are off by default, and scored 56 and 55 with 23 cases changing result between them. Treat a difference of 2–4 points as noise. Trust a trend across several runs or a large move in one category.

## Summary

| Run | Commit | Score | Avg time per case | Main change |
|---|---|---|---|---|
| baseline | `main` | 31/101 | 19.0s | Original harness |
| v2 | `4bd59b2` | 37/101 | 11.2s | Hardened agent loop, post-edit checks, real context budget |
| v3 | — | 17/41 | 11.6s | Partial run, not comparable |
| v4 | `f395b1f` | 41/101 | 7.2s | Read-only question mode, loop breaking, edit repairs |
| v5 | `77e3cc1` | 56/101 | 9.1s | Relevant files loaded up front, security review fixes |
| v6 | `4277b46` | 55/101 | 8.9s | Same as v5 plus opt-in resampling and JSON edits (run with defaults) |
| v6 `-samples 3` | `4277b46` | 59/101 | 9.7s | Failed edits undone and retried |
| v6 `-think` | `4277b46` | 46/101 | 13.6s | Separate planning call before acting |
| v7 | `34cf1ff` | 55/101 | 8.6s | Real-repo file finding, effect-based auto-mode judge, resampling on by default |
| v8 | `92feae2` | 61/101 (+0/4 self) | 15.4s* | More edit fallbacks, review fixes, small projects shown whole |

\* v8 includes four slow self-hosting cases. On the original 101 cases its average is about 13s.

### By category

| Category | baseline | v2 | v4 | v5 | v6 | v6 s3 | v6 think | v7 | v8 |
|---|---|---|---|---|---|---|---|---|---|
| add (13) | 2 | 4 | 5 | 5 | 5 | 7 | 6 | 7 | 6 |
| ask (13) | 4 | 4 | 3 | 8 | 8 | 7 | 9 | 11 | 7 |
| chat (8) | 6 | 8 | 8 | 8 | 8 | 8 | 7 | 8 | 8 |
| edit (13) | 3 | 4 | 6 | 6 | 5 | 3 | 5 | 4 | 7 |
| fix (14) | 8 | 6 | 6 | 9 | 7 | 11 | 6 | 9 | 11 |
| multi (8) | 2 | 3 | 1 | 3 | 2 | 2 | 2 | 3 | 3 |
| nav (12) | 1 | 1 | 1 | 5 | 5 | 5 | 2 | 2 | 5 |
| safe (8) | 3 | 3 | 4 | 5 | 5 | 7 | 3 | 4 | 6 |
| shell (12) | 2 | 4 | 7 | 7 | 10 | 9 | 6 | 7 | 8 |
| self (4) | — | — | — | — | — | — | — | — | 0 |

## What the model is like

These measurements shaped every change below.

- **Context window:** about 8,180 tokens for prompt and reply together. Past that, `fm` returns an error instead of truncating. Go source runs about 2.8 characters per token.
- **Reply cap:** `fm serve` ignores `max_tokens` and honors `max_completion_tokens`. Without a cap, the model sometimes loops inside a JSON string until the context fills, which takes about 3 minutes.
- **Schema:** the JSON schema used for constrained decoding costs no prompt tokens. The model sees tool documentation only through the system prompt.
- **Free-form thinking hurts.** A "thought" field placed before the answer dropped a probe from 11/12 to 3/12, with runaways. Answer first, then thought, scored the same as no thought at all.
- **No concurrency:** requests run one at a time, so parallel calls don't help. Running a probe during an eval caused timeouts.
- **Strengths:** reads what is put in front of it reasonably well.
- **Weaknesses:**
  - Searching on its own.
  - Copying text exactly: it adds or drops indentation, line numbers, punctuation and escapes.
  - Knowing when to stop.

## Changes by version

### Baseline → v2 (31 → 37)

Evaluation and observability:
- `evals/` suite with 101 cases in 9 categories. Each case has a prompt, a starting project and a `check.sh` that grades the result.
- `-trace file.jsonl` records every model request and response.
- `-debug` (or `/debug` in a session) shows each raw model action with its token counts, latency and how much history fits the window.

Agent loop:
- The system prompt and the JSON schema are generated from one tool table, so they can't drift apart.
- The project's file list goes in the system prompt, so the model stops inventing paths.
- The context budget is set to the real ~8k-token window, with automatic backoff when a request overflows.
- Replies are capped at 1,500 tokens, sent as `max_completion_tokens`. The request timeout is 90 seconds.
- A repeated identical action is refused. So is using `echo` to talk to the user.
- At the step cap, the model must reply instead of failing with an error.
- An invalid action gets one retry.

Edits and checks:
- Every edit is checked: `go vet`/`go test`, a Python parse plus `unittest`, `node --check`, or JSON validity. The result goes back to the model with a nudge to reply if it's done.
- `write_file` refuses to replace a file the model hasn't read, or to drop most of its lines.
- New `append_file` tool for "add a function" tasks.
- `edit_file`:
  - Repairs indentation and matches whole lines ignoring surrounding whitespace.
  - Renames a bare identifier everywhere in the file.
  - Rejects no-op edits.
  - Shows the file when `old_text` isn't found.
- Errors use project-relative paths and include "did you mean" hints.

### v2 → v4 (37 → 41)

Based on a case-by-case read of all 64 v2 failures:

- **Questions are answered read-only.** A question gets a toolset without edit tools, plus a nudge to answer once a result contains the answer. Twelve v2 failures had "answered" a question by editing files.
- **Loop breaking:**
  - Repeats are detected on the parsed action, not raw JSON, because the model shuffles key order.
  - Three failed edits to the same file end the turn.
- **More edits land:**
  - Line numbers copied from `read_file` output are stripped.
  - A misquoted declaration is found by name.
  - Non-adjacent lines can be edited in place.
  - An empty `old_text` means append.
  - Deleting a block's header deletes the whole block.
  - A miss names the closest matching line.
  - Loose matches are never applied silently.
- **Long files:** a file over 150 lines reads as its first 60 lines plus an outline of its declarations. A new `read_lines` tool shows any range.
- **Test files are locked** unless the request asks to change tests. This stops the model from "fixing" a bug by editing the test.
- **Smaller fixes:**
  - `python` runs `python3` when that's the only one installed.
  - `read_file` no longer shows a phantom last line.
  - A rename lists other files that still use the old name.
- **Eval fix:** `ask-*` checks grade only the final reply. Two cases had passed because the answer appeared in a tool's output.
- **`-think` added**, opt-in: a separate, bounded three-step plan before acting. A free-form thought field was ruled out by the probe above.

### v4 → v5 (41 → 56)

This was the biggest single jump.

- **Relevant files loaded up front.** Each request starts with the contents of the files it most likely concerns: files named in it, or files containing its words, up to about 6,000 characters. The model is poor at searching but good at reading. In v4 it often gave up after a single case-sensitive grep.
- **Smart-case grep:** a lowercase pattern searches case-insensitively.
- **Security review fixes**, each with a regression test:
  - **Safe-command rules:** auto mode treats a command as safe by rule only when it is plain and its arguments are on an allowlist. Redirections, variables, newlines, background jobs, output flags (`-o`, `-w`, `--in-place`), interpreter `-c`/`-e`, absolute paths and `$HOME` are never safe by rule.
  - **Path confinement:** paths are checked after resolving symlinks, and `.git` is off limits.
  - **Process control:** commands and checks run in their own process group and are killed on timeout.
  - **Ask mode:** post-edit tests run only outside ask mode.
  - **Edit hardening:**
    - Block detection skips braces inside strings and comments.
    - Renames skip keywords, strings and comments.
    - Exact matching respects word boundaries.
    - CRLF line endings are preserved.

### v5 → v6 (56 → 55 with defaults; 59 with `-samples 3`; 46 with `-think`)

- **`-samples N`** is a verifier-guided retry at the edit level. When an edit fails to apply or fails its check, the harness undoes it, drops it from history and asks the model again, up to N tries. The post-edit check acts as the verifier.
- **JSON whole-document edits:** when `old_text` is the whole JSON file written in another layout, often on one line, `new_text` is written back in the file's own indentation and key order.
- **A/B results on the same build:**
  - **`-samples 3`:** 59/101 against 55. Fix rose from 7/14 to 11/14, and failed post-edit checks dropped from 12 to 5.
  - **`-think`:** 46/101. It was worse and slower in most categories, so it stays off.

### v6 → v7 (55 → 55)

- **`-samples 3` is the default.**
- **`-think` is marked experimental.**
- **Effect-based auto-mode judge.** The model was right on only 12 of 24 held-out commands when asked whether a command was safe, about chance. Its reasons often contradicted its verdicts. Commands the rules can't decide now get six yes/no questions:
  - deletes or empties files
  - changes existing files
  - uses the network
  - runs hidden code
  - changes settings outside the project
  - installs software

  Any yes means the user is asked. In the live test this approved no risky command, and it ran about 15 times faster (24s against 345s).
- **Risky-command rules extended** to settings changes: `crontab`, `export`, `unset`, `alias`, `history`, `launchctl`, `defaults write`, `osascript`, and `git config`/`remote`/`submodule`.
- **Finding files in real repos.** Running fm-code on its own repository exposed gaps the small fixtures hide:
  - **File list:** it showed only eval fixtures. Directories with many files now fold into one line with a count, so top-level sources always appear.
  - **Search depth:** file search stopped after the first 400 files it walked. It now looks at the shallowest files first.
  - **Scoring:** `README.md` matched every request. Scoring now weighs rare words more than common ones, files named after a request word, and identifiers from the request, most of all where they are defined. Files too big to show whole are shown as excerpts around the matches.
- **Edit repairs:** line numbers the model copies onto their own lines are dropped.
- **Syntax undo:** an edit that leaves a file unable to parse is undone.

v7 scored the same as v6 overall, but nav fell from 5/12 to 2/12. The new scoring was leaving out files a rename or search needed on small projects. That was fixed in v8.

### v7 → v8 (55 → 61)

- **More misquoted edits land:**
  - A one-line `old_text` with the right words but different punctuation, such as `version "1.2.0"` for `"version": "1.2.0",`, matches when exactly one line holds those words in order. Only the changed words are swapped, and the line keeps its own punctuation.
  - Newlines escaped twice, arriving as a literal `\n`, are decoded.
  - Spaces where the file indents with tabs, as in Makefiles, become tabs.
- **JSON fragment patches.** When `old_text` is an object from the file written differently, the harness finds where it sits, applies the change `new_text` makes (keys removed, changed or added), and rewrites the file in its own indentation and key order. This also works when the model leaves out a closing brace.
- **Nudge wording:** post-edit messages now say "If every part of the request is done, reply now". With "the task", the model had replied after making one of two requested changes.
- **Small projects shown whole.** A project that fits the context budget is shown in full, which fixed the v7 nav drop.
- **Fixes from a second independent review**, each with a regression test:
  - **No false success:** an edit that only changes an operator now fails instead of reporting success with the file unchanged.
  - **Strings left alone:** word matching ignores words inside string literals, except in JSON.
  - **Escapes kept:** decoding a literal `\n` keeps an intended `\\n` intact.
  - **Numbers kept:** only line numbers copied from `old_text` are dropped from `new_text`, so real number-only lines survive.
  - **JSON characters kept:** JSON patches leave `<`, `>` and `&` in untouched strings alone.
  - **Narrower undo:** the syntax undo fires only if the file parsed before the edit and the error names that file. Fixing one of two syntax errors is no longer undone.
  - **Read guard:** a file is marked read only when it has actually been shown in full. Otherwise the guard against overwriting unread files could be bypassed.
- **Self-hosting eval cases.** A case can now start from a commit of this repository instead of a small fixture. Four `self-*` cases do small real tasks on fm-code itself: raise the history limit, add a `/version` command, add `telnet` to the network commands, and answer which commands count as network access.

v8 scored 61 on the original 101 cases, the best run yet, with edit rising from 3/13 to 7/13. The self-hosting cases scored 0/4:
- **History limit:** the model edited the wrong line even though the right file was shown.
- **Telnet:** it broke a regex declaration.
- **Network question:** it wandered through malformed `grep` commands instead of reading the excerpt it was given.
- **`/version`:** none of its edits to `main.go` matched the file.

### After v8 (not yet measured)

- **Go parse errors:** the syntax undo now recognizes Go parse errors of the form "expected X, found Y". It had missed the broken regex declaration in the telnet case.

## Features added along the way

These came from the original request and don't change scores directly:

- **↑/↓ history:** browse earlier prompts. History is saved across sessions, up to 500 entries.
- **Permission modes:** Shift+Tab cycles **ask** → **auto** → **autopilot**. The current mode shows before the prompt and can also be set with `-mode`. `-yes` means autopilot.
- **Auto mode:** deterministic rules come first. A separate model request decides what the rules can't, and it sees only the command, never the conversation, so nothing the agent read can talk it into approving.
- **Debugging:** `-debug`/`-v`/`/debug`, plus `-trace`.
- **Optional features:** `-think` and `-samples`.

## Remaining failure patterns (v8)

| Signature | Failing cases |
|---|---|
| Hit the step cap (forced reply) | 24 |
| Repeated an action | 20 |
| Edit `old_text` not found | 16 |
| A post-edit check failed | 11 |

An earlier classification of failures found about a third were model capability limits: the edit applied, but the fix itself was wrong. The harness can't fix those. Candidates for the next round:

- Steering the model to the right line when the file is already shown.
- Stopping open-ended bash exploration when answering questions.
- Using the eval traces to tune prompts.
- Retrieving worked examples into the prompt.
