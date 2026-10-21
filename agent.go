package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	maxSteps = 12
	// The on-device model has a ~4k token window; budget history in characters.
	historyBudget = 7000
)

const systemPrompt = `You are fm-code, a coding assistant working in the user's project directory.
Each turn you must output one JSON action. Tools:
- list_dir {path}: list a directory
- read_file {path, start_line?, end_line?}: read a file with line numbers
- glob {pattern}: find files by pattern, e.g. **/*.go
- grep {pattern, path?}: regex search across files
- bash {command}: run a shell command, e.g. to run tests
- write_file {path, content}: create or overwrite a file
- edit_file {path, old_text, new_text}: replace one exact, unique snippet in a file
- reply {message}: give your final answer to the user
Rules: look before you edit (read_file first). Never guess file contents. After a tool result, either use another tool or reply. When the task is done, use reply with a short summary.`

// actionSchema is a union with one variant per tool, so the model must fill in
// every argument that tool needs. The shape (titles, $defs, x-order) mirrors
// what `fm schema object --anyOf` generates, which is what fm's decoder expects.
var actionSchema = buildSchema(map[string][]string{
	"list_dir":   {"path"},
	"read_file":  {"path"},
	"glob":       {"pattern"},
	"grep":       {"pattern", "path"},
	"bash":       {"command"},
	"write_file": {"path", "content"},
	"edit_file":  {"path", "old_text", "new_text"},
	"reply":      {"message"},
})

func buildSchema(tools map[string][]string) map[string]any {
	var refs []any
	defs := map[string]any{}
	for _, tool := range toolNames {
		args := tools[tool]
		props := map[string]any{"tool": map[string]any{"type": "string", "enum": []string{tool}}}
		for _, arg := range args {
			props[arg] = map[string]any{"type": "string"}
		}
		order := append([]string{"tool"}, args...)
		defs[tool] = map[string]any{
			"title":                tool,
			"type":                 "object",
			"properties":           props,
			"required":             order,
			"x-order":              order,
			"additionalProperties": false,
		}
		refs = append(refs, map[string]any{"$ref": "#/$defs/" + tool})
	}
	return map[string]any{"title": "Action", "anyOf": refs, "$defs": defs}
}

type completer interface {
	complete(ctx context.Context, msgs []message, schema any) (string, error)
}

type agent struct {
	model   completer
	tools   *toolbox
	out     io.Writer
	verbose bool
	history []message // excludes the system prompt
}

// Send runs one user turn to completion and returns the final reply.
func (a *agent) Send(input string) (string, error) {
	a.history = append(a.history, message{"user", input})
	for step := 0; step < maxSteps; step++ {
		raw, err := a.model.complete(context.Background(), a.window(), actionSchema)
		if err != nil {
			return "", err
		}
		var act action
		if err := json.Unmarshal([]byte(raw), &act); err != nil {
			return "", fmt.Errorf("model produced invalid action %q: %v", raw, err)
		}
		if a.verbose {
			fmt.Fprintf(a.out, "\x1b[2m%s\x1b[0m\n", raw)
		}
		a.history = append(a.history, message{"assistant", raw})
		if act.Tool == "reply" {
			return act.Message, nil
		}
		fmt.Fprintf(a.out, "\n%s⏺ %s%s(%s%s%s)\n", green, act.Tool, reset, dim, describe(act), reset)
		result := a.tools.run(act)
		first, _, _ := strings.Cut(result, "\n")
		fmt.Fprintf(a.out, "  %s⎿  %s%s\n", dim, clip(first, 70), reset)
		a.history = append(a.history, message{"user", "Result of " + act.Tool + ":\n" + result})
	}
	return "", fmt.Errorf("stopped after %d steps without a final reply", maxSteps)
}

func (a *agent) window() []message { return windowed(systemPrompt, a.history) }

// windowed returns sys plus as many recent messages as fit the budget.
// It always keeps the latest message, and never starts on an orphaned assistant turn.
func windowed(sys string, history []message) []message {
	used, start := len(sys), len(history)
	for start > 0 {
		n := len(history[start-1].Content)
		if used+n > historyBudget && start < len(history) {
			break
		}
		used += n
		start--
	}
	for start < len(history)-1 && history[start].Role != "user" {
		start++
	}
	return append([]message{{"system", sys}}, history[start:]...)
}

// describe is the short argument preview shown next to a tool call.
func describe(a action) string {
	switch a.Tool {
	case "grep":
		return clip(fmt.Sprintf("%s in %s", a.Pattern, a.Path), 60)
	case "glob":
		return clip(a.Pattern, 60)
	case "bash":
		return clip(a.Command, 60)
	default:
		return clip(a.Path, 60)
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	green = "\x1b[32m"
	cyan  = "\x1b[36m"
	red   = "\x1b[31m"
)
