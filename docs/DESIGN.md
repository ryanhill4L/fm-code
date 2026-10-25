# Design: getting a small model to do coding work

fm-code runs on Apple's on-device Foundation Model, which has an ~8k-token window, no tool calling, and a few reliable weaknesses. The model can't be changed, so every gain comes from the harness around it. This page lists the rules the harness follows and where each one lives in the code. [`evals/HISTORY.md`](../evals/HISTORY.md) has the chronology and the eval score for each change (31/101 at baseline, 61/101 at v8).

## What the model is like

Measured, not assumed. Everything below follows from these.

- About 8,180 tokens for prompt and reply together, and an overflow is an error, not a truncation. Code runs about 2.8 characters per token.
- `fm serve` honors `max_completion_tokens` and ignores `max_tokens`. Uncapped, a reply can loop inside a JSON string for minutes.
- The JSON schema used for constrained decoding costs no prompt tokens, and the model never sees it.
- A free-form "thought" field makes answers worse. Bounded fields don't.
- Requests run one at a time.
- It reads what is put in front of it well. It searches badly, copies text inexactly, and doesn't know when to stop.

## Rules the harness follows

### Do the searching for it

The model is poor at finding files and good at reading them, so each request starts with the files it most likely concerns.

- `relevantFiles` in `context.go` scores files by name matches, identifiers from the request (most of all where they're defined), and rare words. A word that's in every file, as a README is, counts for almost nothing. Big files are shown as excerpts around the matches. A project small enough to fit the budget is shown whole.
- `projectTree` puts the file list in the system prompt so the model doesn't invent paths. Big directories fold into one line with a count, and shallow files come first.
- `grep` is smart-case: a lowercase pattern ignores case.

### Spend the window carefully

- `historyBudget` and `windowed` in `agent.go` keep the newest messages that fit, and never start on an orphaned assistant turn. `complete` halves the budget and retries when `fm` reports an overflow.
- `maxReplyTokens` in `fm.go` caps each reply.
- `readFile` in `tools.go` shows a long file as its first 60 lines plus an outline of declarations. `read_lines` shows any range.

### Forgive inexact copying

`edit_file` is where most failures used to happen. `locate` in `edit.go` accepts what the model meant when there is exactly one place it could mean:

- Indentation mistakes are repaired, and spaces become tabs where the file uses tabs.
- Line numbers copied from `read_file` output are stripped, and doubled `\n` escapes are decoded.
- When there's no exact match, it tries a ladder of fallbacks, strictest first: a declaration found by name (`matchDeclaration`), lines that occur apart (`matchLineSet`), then one line with the same words in order (`matchTokens`).
- A JSON file can be edited with a fragment written in another layout. `jsonedit.go` patches it and keeps the file's own key order and indentation.

Every fallback needs a unique match. Anything looser is reported with the closest line instead of applied.

### Check every edit, and undo the bad ones

- After every edit, `check` in `check.go` runs `go vet`, a Python parse, `node --check` or JSON validation, plus the tests outside ask mode. It feeds the result back to the model.
- `runSampled` in `agent.go` handles an edit that fails to apply or fails its check. It undoes the edit, drops it from history and samples again, up to `-samples` tries (default 3). The model only sees the attempt that was kept.
- An edit that leaves a file that used to parse unparseable is always undone (`unparseable`).
- `writeFile` refuses to replace a file the model hasn't read in full, or to drop most of its lines. Test files are locked unless the request asks for test changes, so the model can't fix a bug by editing the test.

### Stop it from wandering

All of these are in `Send` in `agent.go`:

- **Questions get no edit tools.** `isQuestion` classifies the request. Given edit tools, the model "answers" questions by changing code.
- **No repeats.** An action identical to an earlier one, compared after parsing because the model shuffles key order, is refused until a file changes.
- **Failed edits end the turn.** Three failed edits to one file stop it.
- **Nudges to reply.** After a successful change or an answering read, the model is told to reply if every part of the request is done.
- **A forced report.** At the step cap the model must report what it did and what's left.

### Keep auto mode safe without trusting the model's judgment

`permissions.go`:

- `ruleVerdict` decides most commands with plain rules: an allowlist of read-only tools, builds and tests, and a pattern list of risky commands that always ask.
- `judgeCommand` handles what's left in a separate request that sees only the command, never the conversation. It doesn't ask whether the command is safe; the model was right only about half the time on that. It asks six yes/no questions about effects, and any yes means the user is asked.

## Tried and dropped

- **`-think`:** a separate planning call before acting. It scored 46/101 against 55 and ran slower, so it's off by default.
- **A free-form thought field:** a probe dropped from 11/12 to 3/12.

## Measuring a change

Run `evals/run.sh`, then `python3 evals/summarize.py <run> <older-run>`. Two runs of the same build differ by 2–4 points, so trust trends and large moves in one category, not a single small delta. Add a case under `evals/cases/` for any new behavior. A case can start from a commit of this repo (`snapshot`) to test on real code.
