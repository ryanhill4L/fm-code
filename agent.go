package main

import (
	"encoding/json"
	"fmt"
	"io"
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
- grep {pattern, path?}: regex search across files
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
	"grep":       {"pattern", "path"},
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
	complete(msgs []message, schema any) (string, error)
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
		raw, err := a.model.complete(a.window(), actionSchema)
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
		fmt.Fprintf(a.out, "\x1b[2m→ %s\x1b[0m\n", describe(act))
		result := a.tools.run(act)
		a.history = append(a.history, message{"user", "Result of " + act.Tool + ":\n" + result})
	}
	return "", fmt.Errorf("stopped after %d steps without a final reply", maxSteps)
}

// window returns the system prompt plus as many recent messages as fit the budget.
// It always keeps the latest message, and never starts on an orphaned assistant turn.
func (a *agent) window() []message {
	used, start := len(systemPrompt), len(a.history)
	for start > 0 {
		n := len(a.history[start-1].Content)
		if used+n > historyBudget && start < len(a.history) {
			break
		}
		used += n
		start--
	}
	for start < len(a.history)-1 && a.history[start].Role != "user" {
		start++
	}
	return append([]message{{"system", systemPrompt}}, a.history[start:]...)
}

func describe(a action) string {
	switch a.Tool {
	case "grep":
		return fmt.Sprintf("grep %q %s", a.Pattern, a.Path)
	default:
		return a.Tool + " " + a.Path
	}
}
