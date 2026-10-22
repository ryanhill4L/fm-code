package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelevantFilesPicksByNameAndWords(t *testing.T) {
	box := newBox(t, true)
	files := map[string]string{
		"server.js":   "const PORT = 8123;\nlisten(PORT);\n",
		"utils.js":    "function slug(s) { return s; }\n",
		"README.md":   "docs\n",
		"big/data.js": strings.Repeat("// filler line mentioning port\n", 400),
	}
	for name, body := range files {
		p := filepath.Join(box.root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	got := box.relevantFiles("what port does the server listen on?")
	if !strings.Contains(got, "== server.js ==\n1\tconst PORT = 8123;") {
		t.Errorf("server.js should be included:\n%s", got)
	}
	if strings.Contains(got, "utils.js") || strings.Contains(got, "data.js") {
		t.Errorf("unrelated or oversized files should be left out:\n%s", got)
	}
	if !box.read[filepath.Join(box.root, "server.js")] {
		t.Error("files shown up front count as read")
	}
	if box.relevantFiles("hi") != "" {
		t.Error("a request with no matching words should add nothing")
	}
}

func TestRelevantFilesOnARealishRepo(t *testing.T) {
	box := newBox(t, true)
	big := strings.Repeat("// unrelated filler\n", 300)
	files := map[string]string{
		"README.md":           "history entries, network access, version, permission modes, everything\n",
		"history.go":          "package main\n\nconst maxHistory = 500\n",
		"permissions.go":      big + "var riskyCommand = regexp.MustCompile(`curl|wget`)\n" + big,
		"permissions_test.go": "package main\n// riskyCommand is tested here\n",
		"main.go":             "package main\n// network version entries\n",
	}
	for name, body := range files {
		os.WriteFile(filepath.Join(box.root, name), []byte(body), 0o644)
	}
	got := box.relevantFiles("Change the maximum number of history entries kept from 500 to 1000.")
	if !strings.HasPrefix(got[strings.Index(got, "=="):], "== history.go ==") {
		t.Errorf("history.go should come first:\n%s", got)
	}
	got = box.relevantFiles("Which commands does the riskyCommand regex treat as network access?")
	if !strings.HasPrefix(got[strings.Index(got, "=="):], "== permissions.go ==\n(excerpts; the file has 601 lines") ||
		!strings.Contains(got, "301\tvar riskyCommand") {
		t.Errorf("the big file defining riskyCommand should be excerpted first:\n%s", got)
	}
}

func TestProjectTreeFoldsCrowdedDirectories(t *testing.T) {
	var files []string
	for i := 0; i < 50; i++ {
		files = append(files, fmt.Sprintf("evals/cases/c%d/prompt.txt", i), fmt.Sprintf("evals/cases/c%d/check.sh", i))
	}
	files = append(files, "evals/run.sh", "main.go", "agent.go", "docs/a.md", "docs/b.md")
	got := strings.Join(foldTree(files, 20), "\n")
	for _, want := range []string{"main.go", "agent.go", "evals/run.sh", "evals/cases/ (100 files)", "docs/a.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("tree should show %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "c7/") {
		t.Errorf("crowded directory should be folded:\n%s", got)
	}
}
