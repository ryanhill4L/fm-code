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

In a session: `/clear` resets the conversation (worth doing often, since the model's memory is tiny), `/help` lists commands, `exit` or Ctrl-D quits.

Flags: `-C dir` project root, `-yes` auto-apply edits, `-v` print the model's raw actions, `-version`.

## How it works

`fm respond` has no tool-calling, so fm-code starts `fm serve` on a private unix socket and constrains each model turn to a JSON action (an `anyOf` schema with one variant per tool). Go runs the tool and feeds the result back, until the model emits `reply`.

Tools: `list_dir`, `read_file`, `glob` (supports `**`), `grep`, `write_file`, `edit_file` (exact unique-snippet replace) and `bash` (30s timeout, runs in the project root). Paths are confined to the project root. Writes, edits and shell commands ask `[y/N]` unless `-yes`. `bash` is not sandboxed, so a command can still touch anything you can.

The on-device model has a ~4k token window, so history is trimmed to the most recent ~7k characters and tool results are capped at 2.5k. Expect it to be best at small, focused edits.

## License

MIT. See [LICENSE](LICENSE). Your use of the `fm` command and Apple Foundation Models is governed separately by Apple's terms.
