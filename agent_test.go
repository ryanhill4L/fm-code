package main

import (
	"context"
	"io"
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
	if !containsKey(results, "You already did exactly this") {
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
