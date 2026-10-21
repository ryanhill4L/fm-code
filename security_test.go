package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSymlinksCannotEscapeRoot(t *testing.T) {
	box := newBox(t, true)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("keep"), 0o644)
	os.Symlink(outside, filepath.Join(box.root, "link"))
	for _, a := range []action{
		{Tool: "edit_file", Path: "link/secret.txt", OldText: "keep", NewText: "gone"},
		{Tool: "write_file", Path: "link/new.txt", Content: "x"},
		{Tool: "read_file", Path: "link/secret.txt"},
	} {
		if got := box.run(a); !strings.Contains(got, "outside") {
			t.Errorf("%s %s: got %q", a.Tool, a.Path, got)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(outside, "secret.txt")); string(data) != "keep" {
		t.Error("a file outside the project was changed through a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Error("a file was created outside the project through a symlink")
	}
}

func TestGitDirIsOffLimits(t *testing.T) {
	box := newBox(t, true)
	os.MkdirAll(filepath.Join(box.root, ".git"), 0o755)
	os.WriteFile(filepath.Join(box.root, ".git", "config"), []byte("[core]\n"), 0o644)
	got := box.run(action{Tool: "append_file", Path: ".git/config", Content: "\tfsmonitor = touch pwned"})
	if !strings.Contains(got, "inside .git") {
		t.Errorf("got %q", got)
	}
}

func TestAbsolutePathCannotDodgeTestLock(t *testing.T) {
	box := newBox(t, true)
	p := filepath.Join(box.root, "x_test.go")
	os.WriteFile(p, []byte("package x\n"), 0o644)
	box.lockTests = true
	if got := box.run(action{Tool: "append_file", Path: p, Content: "// x"}); !strings.Contains(got, "test file") {
		t.Errorf("got %q", got)
	}
}

func TestTimeoutKillsChildProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	runLimited(ctx, t.TempDir(), "bash", "-c", "sleep 5; echo hi")
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("a forked child kept the command alive for %s", d)
	}
}
