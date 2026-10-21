package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocateFixesIndentation(t *testing.T) {
	src := "def f(n):\n    result = 1\n    for i in range(2, n + 1):\n        result *= i\n    return result\n"
	cases := []struct{ name, old, new, want string }{
		{"exact", "result = 1", "result = 2",
			"def f(n):\n    result = 2\n    for i in range(2, n + 1):\n        result *= i\n    return result\n"},
		{"new text written from column zero", "result = 1", "if n < 0:\n    raise ValueError()\nresult = 1",
			"def f(n):\n    if n < 0:\n        raise ValueError()\n    result = 1\n    for i in range(2, n + 1):\n        result *= i\n    return result\n"},
		{"old text with wrong indentation", "for i in range(2, n + 1):\n  result *= i", "for i in range(1, n + 1):\n    result *= i",
			"def f(n):\n    result = 1\n    for i in range(1, n + 1):\n        result *= i\n    return result\n"},
		{"new text over-indented", "  return result", "        return result + 0",
			"def f(n):\n    result = 1\n    for i in range(2, n + 1):\n        result *= i\n    return result + 0\n"},
	}
	for _, c := range cases {
		start, end, repl, err := locate(src, c.old, c.new)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := src[:start] + repl + src[end:]; got != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
	if _, _, _, err := locate(src, "while True:", "x"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing text should fail, got %v", err)
	}
}

func TestWriteFileWontClobber(t *testing.T) {
	box := newBox(t, true)
	p := filepath.Join(box.root, "m.py")
	orig := "def add(a, b):\n    return a + b\n\n\ndef sub(a, b):\n    return a - b\n"
	os.WriteFile(p, []byte(orig), 0o644)

	clobber := action{Tool: "write_file", Path: "m.py", Content: "def mul(a, b):\n    return a * b\n"}
	if got := box.run(clobber); !strings.Contains(got, "haven't read it") {
		t.Errorf("unread overwrite should fail, got %q", got)
	}
	box.run(action{Tool: "read_file", Path: "m.py"})
	if got := box.run(clobber); !strings.Contains(got, "would delete 4 existing lines") {
		t.Errorf("lossy overwrite should fail, got %q", got)
	}
	if data, _ := os.ReadFile(p); string(data) != orig {
		t.Errorf("file changed: %q", data)
	}
	if got := box.run(action{Tool: "write_file", Path: "m.py", Content: orig + "\n\ndef mul(a, b):\n    return a * b\n"}); !strings.HasPrefix(got, "wrote") {
		t.Errorf("a superset rewrite of a read file should work, got %q", got)
	}
}

func TestMissingFileSuggestsRealOnes(t *testing.T) {
	box := newBox(t, true)
	os.MkdirAll(filepath.Join(box.root, "pkg"), 0o755)
	os.WriteFile(filepath.Join(box.root, "pkg", "bank.py"), nil, 0o644)
	got := box.run(action{Tool: "read_file", Path: "src/bank.py"})
	if got != "error: src/bank.py does not exist. Did you mean pkg/bank.py?" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(box.run(action{Tool: "list_dir", Path: "nope"}), box.root) {
		t.Error("errors should not leak the absolute root")
	}
}

func TestEditsAreChecked(t *testing.T) {
	box := newBox(t, true)
	if got := box.run(action{Tool: "write_file", Path: "ok.py", Content: "print(1)\n"}); !strings.Contains(got, "check: OK") {
		t.Errorf("got %q", got)
	}
	if got := box.run(action{Tool: "write_file", Path: "bad.py", Content: "if x:\nprint(1)\n"}); !strings.Contains(got, "check FAILED") || !strings.Contains(got, "IndentationError") {
		t.Errorf("got %q", got)
	}
	if got := box.run(action{Tool: "write_file", Path: "c.json", Content: "{"}); !strings.Contains(got, "not valid JSON") {
		t.Errorf("got %q", got)
	}
	if got := box.run(action{Tool: "write_file", Path: "notes.txt", Content: "hi"}); strings.Contains(got, "check") || !strings.Contains(got, "reply now") {
		t.Errorf("plain text has no checker, got %q", got)
	}
}

func TestAppendFile(t *testing.T) {
	box := newBox(t, true)
	p := filepath.Join(box.root, "m.py")
	os.WriteFile(p, []byte("def add(a, b):\n    return a + b"), 0o644)
	got := box.run(action{Tool: "append_file", Path: "m.py", Content: "\n\ndef mul(a, b):\n    return a * b"})
	if !strings.HasPrefix(got, "appended") || !strings.Contains(got, "check: OK") {
		t.Errorf("got %q", got)
	}
	if data, _ := os.ReadFile(p); string(data) != "def add(a, b):\n    return a + b\n\n\ndef mul(a, b):\n    return a * b\n" {
		t.Errorf("file = %q", data)
	}
	if got := box.run(action{Tool: "append_file", Path: "new.py", Content: "x"}); !strings.Contains(got, "does not exist") {
		t.Errorf("append to a missing file should fail, got %q", got)
	}
}

func TestFailedEditShowsTheFile(t *testing.T) {
	box := newBox(t, true)
	os.WriteFile(filepath.Join(box.root, "c.go"), []byte("package c\n\nconst MaxRetries = 3\n"), 0o644)
	got := box.run(action{Tool: "edit_file", Path: "c.go", OldText: "MaxRetries = 10", NewText: "MaxRetries = 5"})
	if !strings.Contains(got, "not found") || !strings.Contains(got, "3\tconst MaxRetries = 3") {
		t.Errorf("got %q", got)
	}
	if got := box.run(action{Tool: "edit_file", Path: "c.go", OldText: "x", NewText: "x"}); !strings.Contains(got, "nothing to change") {
		t.Errorf("got %q", got)
	}
}

func TestLongFileShowsHeadAndOutline(t *testing.T) {
	box := newBox(t, true)
	var src strings.Builder
	src.WriteString("package big\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&src, "\n// F%d does things.\nfunc F%d() {\n\tprintln(%d)\n}\n", i, i, i)
	}
	os.WriteFile(filepath.Join(box.root, "big.go"), []byte(src.String()), 0o644)
	got := box.run(action{Tool: "read_file", Path: "big.go"})
	if !strings.Contains(got, "[showing lines 1-60 of 201. Use read_lines to see more.]") || !strings.Contains(got, "199\tfunc F39()") {
		t.Errorf("got %q", got)
	}
	got = box.run(action{Tool: "read_lines", Path: "big.go", StartLine: 199, EndLine: 201})
	if got != "199\tfunc F39() {\n200\t\tprintln(39)\n201\t}\n" {
		t.Errorf("got %q", got)
	}
}

func TestRenameReportsOtherFiles(t *testing.T) {
	box := newBox(t, true)
	os.WriteFile(filepath.Join(box.root, "a.py"), []byte("def calc():\n    return 1\n\nprint(calc())\n"), 0o644)
	os.WriteFile(filepath.Join(box.root, "b.py"), []byte("from a import calc\n"), 0o644)
	got := box.run(action{Tool: "edit_file", Path: "a.py", OldText: "calc", NewText: "total"})
	if !strings.Contains(got, "rename it in these files too:\nb.py:1: from a import calc") {
		t.Errorf("got %q", got)
	}
}

func TestBashHasPython(t *testing.T) {
	box := newBox(t, true)
	if got := box.run(action{Tool: "bash", Command: "python -c 'print(6*7)'"}); !strings.HasPrefix(got, "42") {
		t.Errorf("got %q", got)
	}
}

// Each case is a real failing edit from the v2 eval traces.
func TestEditFallbacksFromTraces(t *testing.T) {
	cases := []struct {
		name, file, src, old, new, want string
	}{
		{"misquoted declaration deletes the whole function", "u.py",
			"def keep():\n    return 1\n\n\ndef old_helper(x):\n    y = x * 2\n    return y\n\n\ndef other():\n    return 3\n",
			"def old_helper():", "",
			"def keep():\n    return 1\n\n\n\ndef other():\n    return 3\n"},
		{"brace block deleted with its header", "f.go",
			"package f\n\nfunc legacyFormat(s string) string {\n\tif s == \"\" {\n\t\treturn \"-\"\n\t}\n\treturn s\n}\n\nfunc Keep() {}\n",
			"func legacyFormat(name string) string {", "",
			"package f\n\n\n\nfunc Keep() {}\n"},
		{"non-adjacent lines edited in place", "config.py",
			"DEBUG = True\nHOST = \"x\"\nTIMEOUT = 30\n",
			"DEBUG = True\nTIMEOUT = 30", "DEBUG = False\nTIMEOUT = 60",
			"DEBUG = False\nHOST = \"x\"\nTIMEOUT = 60\n"},
		{"line numbers copied from read_file", "n.txt",
			"apple\nmango\n", "2\tmango", "kiwi",
			"apple\nkiwi\n"},
		{"empty old_text appends", ".gitignore",
			"node_modules/\n", "", ".env",
			"node_modules/\n.env\n"},
	}
	for _, c := range cases {
		box := newBox(t, true)
		p := filepath.Join(box.root, c.file)
		os.WriteFile(p, []byte(c.src), 0o644)
		got := box.run(action{Tool: "edit_file", Path: c.file, OldText: c.old, NewText: c.new})
		if data, _ := os.ReadFile(p); string(data) != c.want {
			t.Errorf("%s: result %q\nfile %q\nwant %q", c.name, got, data, c.want)
		}
	}
}

func TestMissedEditNamesClosestLine(t *testing.T) {
	box := newBox(t, true)
	os.WriteFile(filepath.Join(box.root, "m.go"), []byte("package m\n\nconst greeting = \"hello, \" + name\n"), 0o644)
	got := box.run(action{Tool: "edit_file", Path: "m.go", OldText: "const greeting = \"hello \" + name", NewText: "x"})
	if !strings.Contains(got, "the closest line is 3: const greeting = \"hello, \" + name") {
		t.Errorf("got %q", got)
	}
}

func TestTestFilesLockedUnlessAsked(t *testing.T) {
	box := newBox(t, true)
	os.WriteFile(filepath.Join(box.root, "calc_test.go"), []byte("package calc\n"), 0o644)
	box.lockTests = !asksForTests.MatchString("run the tests and fix the failing one")
	if got := box.run(action{Tool: "append_file", Path: "calc_test.go", Content: "// x"}); !strings.Contains(got, "Fix the code under test") {
		t.Errorf("got %q", got)
	}
	box.lockTests = !asksForTests.MatchString("add a test for reverse()")
	if got := box.run(action{Tool: "append_file", Path: "calc_test.go", Content: "// x"}); !strings.HasPrefix(got, "appended") {
		t.Errorf("asking for a test should allow editing tests, got %q", got)
	}
}
