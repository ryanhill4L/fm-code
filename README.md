# fm-code

A small Go coding assistant that runs on the on-device Apple Foundation Model via the macOS `fm` command. No network, no API key.

## Requirements

- An Apple Silicon Mac with Apple Intelligence enabled and the `fm` command (`/usr/bin/fm`). Check with `fm available`.
- Go 1.21+, only if you build from source.
- **You must agree to the `fm` license first.** Run `fm license` once, read the terms and agree. fm-code checks `fm license --status` on startup and exits with instructions if you haven't. It never agrees on your behalf.

## Install

One-line install (downloads the latest release, verifies its checksum, installs to `~/.local/bin`):

    curl -fsSL https://raw.githubusercontent.com/ryanhill4L/fm-code/main/install.sh | bash

Or with Go:

    go install github.com/ryanhill4L/fm-code@latest

Or from a clone:

    make install        # PREFIX=/usr/local make install to change the location

Then agree to the `fm` license (see above) and run `fm-code`. The installer never agrees on your behalf.

## Usage

    fm-code                         # interactive session in the current directory
    fm-code 'fix the typo in main.go'
    fm-code -C ~/proj -yes 'add a doc comment to Foo'   # -yes skips edit confirmation

**Next-command suggestions.** After each answer, fm-code forks the conversation into a separate model request that looks at the latest output and predicts what you'd ask next (a failing test suggests the fix, finished work suggests running the tests). It appears as grey text at the prompt; press **→** to accept it, then Enter to run it, or just keep typing to ignore it. It runs in the background so the prompt is never delayed, and the fork never touches the assistant's own history. It only appears in an interactive terminal; turn it off with `-no-suggest`, which also switches back to the terminal's plain line input.

In a session: `/clear` resets the conversation, `/help` lists commands, **↑/↓** browse earlier prompts (kept in `~/.fm-code_history`), and `exit` or Ctrl-D quits.

**Permission modes.** **Shift+Tab** cycles the mode, shown before the prompt:

- **ask** (default): every write, edit and shell command asks `[y/N]`.
- **auto**: edits inside the project and safe commands (builds, tests, read-only tools, non-destructive git) run without asking. Commands that match a risky pattern always ask: deleting, `sudo`, network, installing, rewriting git history, or paths outside the project. Anything the rules can't classify goes to a separate model request. That request sees only the command, never the conversation, so nothing the agent read can talk it into approving.
- **autopilot**: nothing asks. Same as `-yes`.

Flags: `-C dir` project root, `-mode ask|auto|autopilot`, `-yes` (autopilot), `-no-suggest` disable next-command suggestions, `-v` print the model's raw actions, `-trace file.jsonl` record every model request and response for debugging, `-version`.

## How it works

`fm respond` has no tool-calling, so fm-code starts `fm serve` on a private unix socket and constrains each model turn to a JSON action (an `anyOf` schema with one variant per tool). Go runs the tool and feeds the result back, until the model emits `reply`.

Tools: `list_dir`, `read_file`, `glob` (supports `**`), `grep`, `write_file`, `edit_file` (exact unique-snippet replace) and `bash` (30s timeout, runs in the project root). Paths are confined to the project root. Writes, edits and shell commands ask `[y/N]` unless `-yes`. `bash` is not sandboxed, so a command can still touch anything you can.

The on-device model has a window of about 8k tokens, shared by prompt and reply. History is trimmed to the most recent ~16k characters, and a request that still overflows is retried with less. The project's file list goes in the system prompt so the model doesn't guess paths. Every edit is followed by a quick check (`go build`, a Python parse, `node --check`, or JSON validation), and the result is shown to the model. `write_file` refuses to replace a file the model hasn't read, or to drop most of its lines. `edit_file` repairs the indentation mistakes small models make. Expect fm-code to be best at small, focused edits.

## Evals

`evals/run.sh` runs fm-code against every case in `evals/cases/` and scores each one with its `check.sh`. `evals/summarize.py <run> [<older run>]` prints the pass rate by category, common failure signatures, and the cases that changed between two runs. Requests to the model are serialized, so run one suite at a time.

## License

MIT. See [LICENSE](LICENSE). Your use of the `fm` command and Apple Foundation Models is governed separately by Apple's terms.
