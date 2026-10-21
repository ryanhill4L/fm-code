package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxSteps = 12
	// The on-device model has a ~8k token window shared by prompt and reply, and
	// code runs ~2.8 characters a token. Budget the prompt in characters and leave
	// room for a reply that writes a whole file.
	historyBudget = 16000
	// maxTreeEntries caps the project listing put in the system prompt.
	maxTreeEntries = 60
)

// toolSpec describes one tool to the model. The system prompt and the JSON
// schema are both generated from toolSpecs, because the model never sees the
// schema: the prompt is the only place it learns what each tool does.
type toolSpec struct {
	name string
	args []string // all required by the schema
	doc  string
}

var toolSpecs = []toolSpec{
	{"read_file", []string{"path"}, "show a file with line numbers"},
	{"grep", []string{"pattern", "path"}, `regex search; path "." searches the whole project`},
	{"list_dir", []string{"path"}, "list a directory"},
	{"glob", []string{"pattern"}, "find files by name, e.g. **/*.go"},
	{"edit_file", []string{"path", "old_text", "new_text"}, "replace old_text, copied exactly from read_file, with new_text. To rename an identifier everywhere in a file, use just the old and new names"},
	{"append_file", []string{"path", "content"}, "add content to the end of a file, e.g. a new function"},
	{"write_file", []string{"path", "content"}, "create a NEW file with its full content"},
	{"bash", []string{"command"}, "run a shell command in the project directory, e.g. to run tests or a program"},
	{"reply", []string{"message"}, "your final answer to the user, at most three sentences"},
}

var systemPrompt = buildPrompt(toolSpecs)

func buildPrompt(specs []toolSpec) string {
	var b strings.Builder
	b.WriteString("You are fm-code, a coding assistant working in the user's project directory.\nEach turn, output one JSON action. Tools:\n")
	for _, t := range specs {
		fmt.Fprintf(&b, "- %s {%s}: %s\n", t.name, strings.Join(t.args, ", "), t.doc)
	}
	b.WriteString(`Rules:
- Only use paths from the project file list below. Never invent a path.
- Read a file before you change it, and change only what the user asked.
- After each change the harness checks the code and shows you the result. If the check fails, fix it.
- When the task is done, or you have the answer, use reply. Don't keep exploring.`)
	return b.String()
}

// actionSchema is a union with one variant per tool, so the model must fill in
// every argument that tool needs. The shape (titles, $defs, x-order) mirrors
// what `fm schema object --anyOf` generates, which is what fm's decoder expects.
var actionSchema = buildSchema(toolSpecs)

// replySchema allows only reply, to make the model wrap up.
var replySchema = buildSchema(toolSpecs[len(toolSpecs)-1:])

func buildSchema(specs []toolSpec) map[string]any {
	var refs []any
	defs := map[string]any{}
	for _, t := range specs {
		props := map[string]any{"tool": map[string]any{"type": "string", "enum": []string{t.name}}}
		for _, arg := range t.args {
			props[arg] = map[string]any{"type": "string"}
		}
		order := append([]string{"tool"}, t.args...)
		defs[t.name] = map[string]any{
			"title":                t.name,
			"type":                 "object",
			"properties":           props,
			"required":             order,
			"x-order":              order,
			"additionalProperties": false,
		}
		refs = append(refs, map[string]any{"$ref": "#/$defs/" + t.name})
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
	a.tools.read = nil // read-before-write is checked per turn; old reads may have scrolled out
	seen := map[string]bool{}
	retried := false
	for step := 0; step < maxSteps; step++ {
		act, raw, err := a.next(actionSchema)
		if err != nil {
			// One retry: a cut-off or garbled action is usually a runaway reply.
			if retried {
				return "", err
			}
			retried = true
			a.history = append(a.history, message{"user", "Your last action was cut off or invalid. Output a shorter action; a reply must be at most three sentences."})
			continue
		}
		if act.Tool == "reply" {
			return act.Message, nil
		}
		fmt.Fprintf(a.out, "\n%s⏺ %s%s(%s%s%s)\n", green, act.Tool, reset, dim, describe(act), reset)
		var result string
		switch {
		case seen[raw]:
			result = "You already did exactly this and nothing has changed since. Do something different, or reply."
		case act.Tool == "bash" && echoOnly.MatchString(act.Command):
			result = "echo only shows text to you, not the user. To tell the user something, use reply."
		default:
			result = a.tools.run(act)
		}
		if mutates(act.Tool) {
			clear(seen) // files may have changed, so repeating a read is legitimate again
		}
		seen[raw] = true
		first, _, _ := strings.Cut(result, "\n")
		fmt.Fprintf(a.out, "  %s⎿  %s%s\n", dim, clip(first, 70), reset)
		a.history = append(a.history, message{"user", "Result of " + act.Tool + ":\n" + result})
	}
	// Out of steps: have the model report what it did rather than fail silently.
	a.history = append(a.history, message{"user", "Stop now. Tell the user in one or two sentences what you found or changed, and what is left."})
	act, _, err := a.next(replySchema)
	if err != nil || act.Tool != "reply" {
		return "", fmt.Errorf("stopped after %d steps without a final reply", maxSteps)
	}
	return act.Message, nil
}

// next asks the model for one action and records it in the history.
func (a *agent) next(schema any) (action, string, error) {
	var act action
	raw, err := a.complete(schema)
	if err != nil {
		return act, raw, err
	}
	if a.verbose {
		fmt.Fprintf(a.out, "%s%s%s\n", dim, raw, reset)
	}
	if err := json.Unmarshal([]byte(raw), &act); err != nil {
		return act, raw, fmt.Errorf("model produced invalid action %q: %v", clip(raw, 200), err)
	}
	a.history = append(a.history, message{"assistant", raw})
	return act, raw, nil
}

var echoOnly = regexp.MustCompile(`^\s*echo\s[^|>;&]*$`)

func mutates(tool string) bool {
	return tool == "edit_file" || tool == "write_file" || tool == "append_file" || tool == "bash"
}

// complete asks for the next action, retrying with less history if the
// prompt overflows the model's context.
func (a *agent) complete(schema any) (string, error) {
	budget := historyBudget
	for {
		raw, err := a.model.complete(context.Background(), a.window(budget), schema)
		if err == nil || !strings.Contains(err.Error(), "context size") || budget < 4000 {
			return raw, err
		}
		budget /= 2
	}
}

func (a *agent) window(budget int) []message {
	sys := systemPrompt
	if a.tools != nil {
		sys += "\n\nProject files (paths are relative to the project root):\n" + projectTree(a.tools.root, maxTreeEntries)
	}
	return windowed(sys, a.history, budget)
}

// projectTree lists the project's files, one per line, so the model never has
// to guess a path. Large projects are cut off with a note to use glob.
func projectTree(root string, limit int) string {
	var files []string
	more := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); p != root && (strings.HasPrefix(n, ".") || n == "node_modules" || n == "__pycache__" || n == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= limit {
			more++
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, rel)
		return nil
	})
	if len(files) == 0 {
		return "(the project is empty)"
	}
	if more > 0 {
		files = append(files, fmt.Sprintf("... and %d more (use glob to find them)", more))
	}
	return strings.Join(files, "\n")
}

// windowed returns sys plus as many recent messages as fit the budget.
// It always keeps the latest message, and never starts on an orphaned assistant turn.
func windowed(sys string, history []message, budget int) []message {
	used, start := len(sys), len(history)
	for start > 0 {
		n := len(history[start-1].Content)
		if used+n > budget && start < len(history) {
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
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	cyan   = "\x1b[36m"
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
)
