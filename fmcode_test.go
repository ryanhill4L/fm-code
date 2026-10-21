package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newBox(t *testing.T, approve bool) *toolbox {
	root, err := absDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &toolbox{root: root, approve: func(string) bool { return approve }}
}

func TestEditFile(t *testing.T) {
	box := newBox(t, true)
	p := filepath.Join(box.root, "a.txt")
	os.WriteFile(p, []byte("one\ntwo\ntwo\n"), 0o644)

	if got := box.run(action{Tool: "edit_file", Path: "a.txt", OldText: "two", NewText: "x"}); !strings.Contains(got, "matches 2") {
		t.Errorf("ambiguous edit should fail, got %q", got)
	}
	if got := box.run(action{Tool: "edit_file", Path: "a.txt", OldText: "nope", NewText: "x"}); !strings.Contains(got, "not found") {
		t.Errorf("missing text should fail, got %q", got)
	}
	if got := box.run(action{Tool: "edit_file", Path: "a.txt", OldText: "one", NewText: "1"}); got != "edited a.txt" {
		t.Errorf("got %q", got)
	}
	if data, _ := os.ReadFile(p); string(data) != "1\ntwo\ntwo\n" {
		t.Errorf("file = %q", data)
	}
}

func TestDeclinedWriteLeavesDiskAlone(t *testing.T) {
	box := newBox(t, false)
	got := box.run(action{Tool: "write_file", Path: "new.txt", Content: "hi"})
	if !strings.Contains(got, "declined") {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(box.root, "new.txt")); err == nil {
		t.Error("file was written despite decline")
	}
}

func TestPathsCannotEscapeRoot(t *testing.T) {
	box := newBox(t, true)
	for _, p := range []string{"../x", "/etc/passwd", "a/../../x"} {
		if got := box.run(action{Tool: "read_file", Path: p}); !strings.Contains(got, "outside") {
			t.Errorf("read %q: got %q", p, got)
		}
	}
}

func TestReadFileLineRange(t *testing.T) {
	box := newBox(t, true)
	os.WriteFile(filepath.Join(box.root, "f"), []byte("a\nb\nc\nd"), 0o644)
	got := box.run(action{Tool: "read_file", Path: "f", StartLine: 2, EndLine: 3})
	if !strings.HasPrefix(got, "2\tb\n3\tc\n") {
		t.Errorf("got %q", got)
	}
}

func TestGrep(t *testing.T) {
	box := newBox(t, true)
	os.MkdirAll(filepath.Join(box.root, "sub"), 0o755)
	os.WriteFile(filepath.Join(box.root, "sub", "f.go"), []byte("package x\nfunc Foo() {}\n"), 0o644)
	if got := box.run(action{Tool: "grep", Pattern: `func \w+`}); !strings.Contains(got, "sub/f.go:2: func Foo() {}") {
		t.Errorf("got %q", got)
	}
}

func TestWindowKeepsLatestAndStartsOnUser(t *testing.T) {
	a := &agent{}
	big := strings.Repeat("x", historyBudget/2)
	a.history = []message{{"user", "old"}, {"assistant", big}, {"user", big}, {"assistant", big}, {"user", "latest"}}
	w := a.window()
	if w[0].Role != "system" || w[1].Role != "user" || w[len(w)-1].Content != "latest" {
		t.Errorf("bad window: %v", w)
	}
	if len(w) >= len(a.history)+1 {
		t.Error("expected old messages to be dropped")
	}
}
