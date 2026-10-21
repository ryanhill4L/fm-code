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
	"time"
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
	{"read_file", []string{"path"}, "show a file with line numbers (a long file shows its start and an outline)"},
	{"read_lines", []string{"path", "start_line", "end_line"}, "show part of a long file"},
	{"grep", []string{"pattern", "path"}, `regex search; path "." searches the whole project`},
	{"list_dir", []string{"path"}, "list a directory"},
	{"glob", []string{"pattern"}, "find files by name, e.g. **/*.go"},
	{"edit_file", []string{"path", "old_text", "new_text"}, "replace old_text, copied exactly from read_file, with new_text. To rename an identifier everywhere in a file, use just the old and new names"},
	{"append_file", []string{"path", "content"}, "add content to the end of a file, e.g. a new function"},
	{"write_file", []string{"path", "content"}, "create a NEW file with its full content"},
	{"bash", []string{"command"}, "run a shell command in the project directory, e.g. to run tests or a program"},
	{"reply", []string{"message"}, "your final answer to the user, at most three sentences"},
}

var systemPrompt = buildPrompt(toolSpecs, "")

// readOnlySpecs are the tools for answering a question: everything but edits.
var readOnlySpecs = func() []toolSpec {
	var out []toolSpec
	for _, t := range toolSpecs {
		if !mutates(t.name) || t.name == "bash" {
			out = append(out, t)
		}
	}
	return out
}()

var questionPrompt = buildPrompt(readOnlySpecs, "- This request is a question. Answer it; don't change any files.\n- Use bash to run git, count files or lines, and run programs or tests.\n- Reply with the exact name, value or number that answers it.\n")

var questionSchema = buildSchema(readOnlySpecs)

var changeVerb = regexp.MustCompile(`\b(change|fix|add|remove|rename|set|update|delete|create|move|make|write|implement|refactor|replace|edit|convert|bump|sort|clean|build|install)\b`)
var questionStart = regexp.MustCompile(`^(what|which|where|how|why|who|when|is|are|does|do|did|can|could|list|show|find|explain|tell|count)\b`)

// isQuestion reports whether a request only asks for information.
func isQuestion(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	if strings.Contains(s, "don't change") || strings.Contains(s, "do not change") {
		return true
	}
	if changeVerb.MatchString(s) {
		return false
	}
	return strings.HasSuffix(s, "?") || questionStart.MatchString(s)
}

func buildPrompt(specs []toolSpec, extraRules string) string {
	var b strings.Builder
	b.WriteString("You are fm-code, a coding assistant working in the user's project directory.\nEach turn, output one JSON action. Tools:\n")
	for _, t := range specs {
		fmt.Fprintf(&b, "- %s {%s}: %s\n", t.name, strings.Join(t.args, ", "), t.doc)
	}
	b.WriteString("Rules:\n- Only use paths from the project file list below. Never invent a path.\n")
	if extraRules == "" {
		extraRules = `- Read a file before you change it, and change only what the user asked.
- After each change the harness checks the code and shows you the result. If the check fails, fix it.
`
	}
	b.WriteString(extraRules + "- When the task is done, or you have the answer, use reply. Don't keep exploring.")
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
			if strings.HasSuffix(arg, "_line") {
				props[arg] = map[string]any{"type": "integer"}
			}
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
	complete(ctx context.Context, msgs []message, schema any) (completion, error)
}

type agent struct {
	model completer
	tools *toolbox
	out   io.Writer
	debug bool // print each raw action with its token counts and timing
	think bool // plan each turn in a separate request before acting
	// question is set for a turn that only asks something; it gets no edit tools.
	question bool
	history  []message // excludes the system prompt
}

// Send runs one user turn to completion and returns the final reply.
func (a *agent) Send(input string) (string, error) {
	if a.think {
		input = a.plan(input)
	}
	a.history = append(a.history, message{"user", input})
	a.tools.read = nil // read-before-write is checked per turn; old reads may have scrolled out
	a.tools.lockTests = !asksForTests.MatchString(strings.ToLower(input))
	// A question gets a toolset without edits: given the chance, this model
	// "answers" questions by changing the code it was asked about.
	a.question = isQuestion(input)
	schema := actionSchema
	if a.question {
		schema = questionSchema
	}
	seen := map[string]bool{}  // actions already tried since the files last changed
	failed := map[string]int{} // failed edits per file and old_text
	retried := false
	for step := 0; step < maxSteps; step++ {
		act, _, err := a.next(schema)
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
		key := fmt.Sprintf("%#v", act) // the model shuffles JSON key order, so compare parsed actions
		var result string
		switch {
		case seen[key]:
			result = "error: you already did exactly this and nothing has changed since. Do something different, or reply."
		case act.Tool == "bash" && echoOnly.MatchString(act.Command):
			result = "echo only shows text to you, not the user. To tell the user something, use reply."
		default:
			result = a.tools.run(act)
		}
		failedEdit := strings.HasPrefix(result, "error:") && act.Tool == "edit_file"
		if mutates(act.Tool) && !strings.HasPrefix(result, "error:") {
			clear(seen) // files may have changed, so repeating a read is legitimate again
		}
		seen[key] = true
		if a.question && !mutates(act.Tool) && !strings.HasPrefix(result, "error:") {
			result += "\nIf this answers the question, reply now with the exact answer."
		}
		first, _, _ := strings.Cut(result, "\n")
		fmt.Fprintf(a.out, "  %s⎿  %s%s\n", dim, clip(first, 70), reset)
		a.history = append(a.history, message{"user", "Result of " + act.Tool + ":\n" + result})
		if failedEdit {
			if failed[act.Path]++; failed[act.Path] >= 3 {
				break // stop burning steps on an edit the model can't land
			}
		}
	}
	// Out of steps or stuck: have the model report what it did rather than fail silently.
	a.history = append(a.history, message{"user", "Stop now. Tell the user in one or two sentences what you found or changed, and what is left."})
	act, _, err := a.next(replySchema)
	if err != nil || act.Tool != "reply" {
		return "", fmt.Errorf("stopped after %d steps without a final reply", maxSteps)
	}
	return act.Message, nil
}

const planPrompt = `You plan the work of a coding assistant. Given the user's request and the project's files, write the three steps the assistant should take, in order.
Each step is one short sentence naming a real file from the list, e.g. "Read calc.py", "Fix the comparison in is_adult", "Run python3 calc.py to check".
If the request is only a question or a greeting, the steps are what to read and then "Reply with the answer".`

var planSchema = map[string]any{
	"title":                "Plan",
	"type":                 "object",
	"properties":           map[string]any{"step1": map[string]any{"type": "string"}, "step2": map[string]any{"type": "string"}, "step3": map[string]any{"type": "string"}},
	"required":             []string{"step1", "step2", "step3"},
	"x-order":              []string{"step1", "step2", "step3"},
	"additionalProperties": false,
}

// plan asks a separate request for a short plan and returns the input with
// the plan appended. Free-form "thought" fields make this model ramble and
// get answers wrong; three bounded steps keep it on a track it chose itself.
func (a *agent) plan(input string) string {
	msgs := windowed(planPrompt+"\n\nProject files:\n"+projectTree(a.tools.root, maxTreeEntries), append(append([]message(nil), a.history...), message{"user", input}), historyBudget)
	c, err := a.model.complete(context.Background(), msgs, planSchema)
	var p struct{ Step1, Step2, Step3 string }
	if err != nil || json.Unmarshal([]byte(c.Text), &p) != nil {
		return input
	}
	var steps []string
	for _, s := range []string{p.Step1, p.Step2, p.Step3} {
		if s = strings.TrimSpace(s); s != "" {
			steps = append(steps, clip(s, 120))
		}
	}
	if len(steps) == 0 {
		return input
	}
	fmt.Fprintf(a.out, "\n%s✻ Plan: %s%s\n", dim, strings.Join(steps, " → "), reset)
	return input + "\n\nPlan:\n- " + strings.Join(steps, "\n- ")
}

// next asks the model for one action and records it in the history.
func (a *agent) next(schema any) (action, string, error) {
	var act action
	start := time.Now()
	c, kept, err := a.complete(schema)
	raw := c.Text
	if a.debug {
		fmt.Fprintf(a.out, "%s  ⋯ %d prompt + %d reply tokens of ~8k · %.1fs · %d/%d messages in window%s\n",
			dim, c.PromptTokens, c.CompletionTokens, time.Since(start).Seconds(), kept, len(a.history), reset)
		if raw != "" {
			fmt.Fprintf(a.out, "%s  %s%s\n", dim, raw, reset)
		}
	}
	if err != nil {
		return act, raw, err
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
// prompt overflows the model's context. It also returns how many history
// messages fit in the window.
func (a *agent) complete(schema any) (completion, int, error) {
	budget := historyBudget
	for {
		w := a.window(budget)
		c, err := a.model.complete(context.Background(), w, schema)
		if err == nil || !strings.Contains(err.Error(), "context size") || budget < 4000 {
			return c, len(w) - 1, err
		}
		budget /= 2
	}
}

func (a *agent) window(budget int) []message {
	sys := systemPrompt
	if a.question {
		sys = questionPrompt
	}
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
