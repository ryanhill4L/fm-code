package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// permMode decides which tool calls need the user's say-so. Shift+Tab cycles it.
type permMode int

const (
	modeAsk       permMode = iota // confirm every write, edit and command
	modeAuto                      // edits and safe commands run; risky commands ask
	modeAutopilot                 // everything runs (same as -yes)
)

func (m permMode) next() permMode { return (m + 1) % 3 }

// label is what the prompt shows for the mode; ask mode shows nothing.
func (m permMode) label() string {
	switch m {
	case modeAuto:
		return yellow + "[auto] " + reset
	case modeAutopilot:
		return red + "[autopilot] " + reset
	}
	return ""
}

func parseMode(s string) (permMode, error) {
	switch s {
	case "ask":
		return modeAsk, nil
	case "auto":
		return modeAuto, nil
	case "autopilot":
		return modeAutopilot, nil
	}
	return 0, fmt.Errorf("unknown mode %q (want ask, auto or autopilot)", s)
}

type verdict struct {
	safe   bool
	reason string
}

// riskyCommand matches commands auto mode always asks about, whatever the judge says:
// deleting, privilege, network, history rewriting, process control, and paths outside the project.
var riskyCommand = regexp.MustCompile(strings.Join([]string{
	`(^|[\s;&|(])(rm|rmdir|sudo|su|doas|dd|mkfs\S*|shutdown|reboot|kill|killall|pkill|chown|eval|exec)(\s|$)`,
	`(^|[\s;&|(])(curl|wget|ssh|scp|rsync|nc|ncat|brew|apt|apt-get|pip3?|npm\s+(i|install|publish)|yarn\s+add)(\s|$)`,
	`(^|[\s;&|(])git\s+(push|reset|clean|rebase|checkout\s+--|restore|stash\s+drop|branch\s+-D|filter-branch)(\s|$)`,
	`chmod\s+-R`, `(^|\s)find\s.*-(delete|exec)`, `>\s*/dev/(sd|disk)`, `:\(\)\s*\{`,
	`(^|[\s='"])(~|\.\.)(/|\s|$)`,
	`(^|[\s='"])/(etc|usr|bin|sbin|var|System|Library|Users|home|private|opt)(/|\s|$)`,
}, "|"))

// safeCommands are programs whose ordinary use only reads or builds inside the project.
var safeCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true, "grep": true, "rg": true,
	"find": true, "echo": true, "pwd": true, "which": true, "sort": true, "uniq": true, "cut": true,
	"tr": true, "diff": true, "file": true, "stat": true, "tree": true, "du": true, "date": true,
	"true": true, "test": true, "[": true, "printf": true, "awk": true, "sed": true, "jq": true,
	"node": true, "python3": true, "python": true, "go": true, "cargo": true, "swift": true,
	"make": true, "pytest": true, "tsc": true, "gofmt": true, "rustc": true, "gcc": true, "clang": true,
}

// safeGit are the git subcommands that cannot lose work.
var safeGit = map[string]bool{"status": true, "log": true, "diff": true, "show": true, "branch": true, "rev-parse": true, "blame": true, "ls-files": true, "add": true, "commit": true}

// ruleVerdict classifies a command by rules alone. ok is false when the rules
// can't tell and the judge should decide.
func ruleVerdict(cmd string) (v verdict, ok bool) {
	if m := riskyCommand.FindString(cmd); m != "" {
		return verdict{false, "matches a risky pattern: " + strings.TrimSpace(m)}, true
	}
	if strings.ContainsAny(cmd, "`") || strings.Contains(cmd, "$(") || strings.Contains(cmd, "sed -i") {
		return verdict{}, false
	}
	for _, seg := range regexp.MustCompile(`&&|\|\||;|\|`).Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) == 0 {
			continue
		}
		if f[0] == "cd" && len(f) == 2 {
			continue // ".." and absolute paths were caught above
		}
		if f[0] == "git" && len(f) > 1 && safeGit[f[1]] {
			continue
		}
		if !safeCommands[f[0]] {
			return verdict{}, false
		}
	}
	return verdict{true, "only runs read and build commands"}, true
}

const judgePrompt = `You review shell commands that a coding assistant wants to run in the user's project directory.
A command is safe if it only reads files, builds, or runs tests or the project's own programs.
It is risky if it deletes or overwrites files outside a build, changes git history, installs software, uses the network, changes permissions, or touches anything outside the project.
When unsure, say risky.`

var judgeSchema = map[string]any{
	"title": "Verdict",
	"type":  "object",
	"properties": map[string]any{
		"reason":  map[string]any{"type": "string"},
		"verdict": map[string]any{"type": "string", "enum": []string{"safe", "risky"}},
	},
	"required":             []string{"reason", "verdict"},
	"x-order":              []string{"reason", "verdict"},
	"additionalProperties": false,
}

// judgeCommand classifies a shell command for auto mode: rules first, then a
// separate model request that sees only the command, not the conversation, so
// nothing the agent read can talk it into approving.
func judgeCommand(ctx context.Context, model completer, cmd string) verdict {
	if v, ok := ruleVerdict(cmd); ok {
		return v
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := model.complete(ctx, []message{{"system", judgePrompt}, {"user", "Command:\n" + cmd}}, judgeSchema)
	if err != nil {
		return verdict{false, "couldn't check it: " + err.Error()}
	}
	var out struct {
		Reason  string `json:"reason"`
		Verdict string `json:"verdict"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.Verdict != "safe" {
		return verdict{false, out.Reason}
	}
	return verdict{true, out.Reason}
}
