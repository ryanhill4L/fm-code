package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// scripted replays canned actions and records the schema of each request.
type scripted struct {
	replies []string
	schemas []any
	seen    [][]message
}

func (s *scripted) complete(_ context.Context, msgs []message, schema any) (completion, error) {
	s.schemas = append(s.schemas, schema)
	s.seen = append(s.seen, msgs)
	r := s.replies[0]
	if len(s.replies) > 1 {
		s.replies = s.replies[1:]
	}
	return completion{Text: r}, nil
}

func TestSendBreaksLoopsAndForcesAReply(t *testing.T) {
	box := newBox(t, true)
	m := &scripted{replies: []string{
		`{"tool":"list_dir","path":"."}`,
		`{"tool":"bash","command":"echo 'all done'"}`,
		`{"tool":"list_dir","path":"."}`, // repeated forever until the step cap
		`{"tool":"reply","message":"Looked around; nothing to change."}`,
	}}
	m.replies = append(m.replies[:3], append(repeat(`{"tool":"list_dir","path":"."}`, maxSteps-3), m.replies[3])...)
	a := &agent{model: m, tools: box, out: io.Discard}

	got, err := a.Send("look around")
	if err != nil || got != "Looked around; nothing to change." {
		t.Fatalf("got %q, %v", got, err)
	}
	results := map[string]bool{}
	for _, msg := range a.history {
		if msg.Role == "user" {
			results[msg.Content] = true
		}
	}
	if !containsKey(results, "echo only shows text to you") {
		t.Error("echo should be answered with a nudge to reply, not run")
	}
	if !containsKey(results, "you already did exactly this") {
		t.Error("a repeated action should be refused")
	}
	if last := m.schemas[len(m.schemas)-1]; last == nil || len(last.(map[string]any)["anyOf"].([]any)) != 1 {
		t.Error("the final request at the step cap should allow only reply")
	}
}

func TestSystemPromptListsProjectFilesAndEveryTool(t *testing.T) {
	box := newBox(t, true)
	box.run(action{Tool: "write_file", Path: "pkg/a.go", Content: "package pkg\n"})
	a := &agent{tools: box}
	sys := a.window(historyBudget)[0].Content
	if !strings.Contains(sys, "\npkg/a.go") {
		t.Errorf("system prompt should list project files:\n%s", sys)
	}
	for _, spec := range toolSpecs {
		if !strings.Contains(sys, "- "+spec.name+" {") {
			t.Errorf("system prompt is missing tool %s", spec.name)
		}
	}
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func containsKey(m map[string]bool, sub string) bool {
	for k := range m {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

func TestThinkPlansBeforeActing(t *testing.T) {
	m := &scripted{replies: []string{
		`{"step1":"Read calc.py","step2":"Fix the comparison","step3":"Reply with what changed"}`,
		`{"tool":"reply","message":"ok"}`,
	}}
	a := &agent{model: m, tools: newBox(t, true), out: io.Discard, think: true}
	if _, err := a.Send("fix calc"); err != nil {
		t.Fatal(err)
	}
	if got := a.history[0].Content; got != "fix calc\n\nPlan:\n- Read calc.py\n- Fix the comparison\n- Reply with what changed" {
		t.Errorf("the plan should ride along with the request: %q", got)
	}
	if m.schemas[0].(map[string]any)["title"] != "Plan" {
		t.Error("first request should be the plan")
	}
}

func TestIsQuestion(t *testing.T) {
	questions := []string{"what port does the server listen on?", "which file defines the User struct?", "list the TODO comments", "how many lines are in data.csv", "what files are in this directory? don't change anything", "Is there a Makefile target for lint?"}
	tasks := []string{"can you append a newline to notes.txt", "do the TODO in util.go", "fix the bug in calc.py", "where is MaxRetries used? change it to 5", "add a multiply function", "run the tests and fix the failing one", "the add function is wrong, fix it", "build the project and fix the compile error", "hi"}
	for _, q := range questions {
		if !isQuestion(q) {
			t.Errorf("%q should be a question", q)
		}
	}
	for _, q := range tasks {
		if isQuestion(q) {
			t.Errorf("%q should not be a question", q)
		}
	}
}

func TestQuestionsGetNoEditTools(t *testing.T) {
	m := &scripted{replies: []string{`{"tool":"reply","message":"8123"}`}}
	a := &agent{model: m, tools: newBox(t, true), out: io.Discard}
	a.Send("what port does the server listen on?")
	defs := m.schemas[0].(map[string]any)["$defs"].(map[string]any)
	for _, banned := range []string{"edit_file", "write_file", "append_file"} {
		if defs[banned] != nil || strings.Contains(m.seen[0][0].Content, "- "+banned+" {") {
			t.Errorf("a question should not offer %s", banned)
		}
	}
	if defs["bash"] == nil {
		t.Error("a question should still be able to run commands")
	}
}

func TestRepeatedFailedEditStops(t *testing.T) {
	box := newBox(t, true)
	box.run(action{Tool: "write_file", Path: "c.py", Content: "x = 1\n"})
	bad := `{"tool":"edit_file","path":"c.py","old_text":"y = 2","new_text":"y = 3"}`
	shuffled := `{"new_text":"y = 3","old_text":"y = 2","path":"c.py","tool":"edit_file"}`
	m := &scripted{replies: []string{bad, shuffled, bad, `{"tool":"reply","message":"gave up"}`}}
	a := &agent{model: m, tools: box, out: io.Discard}
	if got, _ := a.Send("set y to 3"); got != "gave up" {
		t.Fatalf("got %q", got)
	}
	if !containsKey(map[string]bool{a.history[4].Content: true}, "you already did exactly this") {
		t.Errorf("the same edit with shuffled keys should count as a repeat: %q", a.history[4].Content)
	}
	if len(m.schemas) != 4 || len(m.schemas[3].(map[string]any)["anyOf"].([]any)) != 1 {
		t.Errorf("after three failed edits the model should be made to reply, got %d requests", len(m.schemas))
	}
}

func TestFailedEditIsUndoneAndResampled(t *testing.T) {
	box := newBox(t, true)
	p := box.root + "/m.py"
	box.run(action{Tool: "write_file", Path: "m.py", Content: "def f():\n    return 1\n"})
	m := &scripted{replies: []string{
		`{"tool":"edit_file","path":"m.py","old_text":"return 1","new_text":"return (1"}`, // breaks the parse
		`{"tool":"edit_file","path":"m.py","old_text":"return 1","new_text":"return 2"}`,
		`{"tool":"reply","message":"done"}`,
	}}
	a := &agent{model: m, tools: box, out: io.Discard, samples: 3}
	if got, _ := a.Send("make f return 2"); got != "done" {
		t.Fatalf("got %q", got)
	}
	if data, _ := os.ReadFile(p); string(data) != "def f():\n    return 2\n" {
		t.Errorf("file = %q", data)
	}
	for _, msg := range a.history {
		if strings.Contains(msg.Content, "return (1") || strings.Contains(msg.Content, "check FAILED") {
			t.Errorf("the rejected attempt should not stay in history: %q", msg.Content)
		}
	}
}

func TestEditThatBreaksSyntaxIsUndoneEvenWithoutSamples(t *testing.T) {
	box := newBox(t, true)
	p := box.root + "/m.py"
	os.WriteFile(p, []byte("def f():\n    return 1\n"), 0o644)
	m := &scripted{replies: []string{
		`{"tool":"edit_file","path":"m.py","old_text":"return 1","new_text":"return (1"}`,
		`{"tool":"reply","message":"gave up"}`,
	}}
	a := &agent{model: m, tools: box, out: io.Discard, samples: 1}
	a.Send("make f return 2")
	if data, _ := os.ReadFile(p); string(data) != "def f():\n    return 1\n" {
		t.Errorf("a syntax-breaking edit should be undone, file = %q", data)
	}
	if !strings.Contains(a.history[2].Content, "was undone") {
		t.Errorf("the model should be told the edit was undone: %q", a.history[2].Content)
	}
}
