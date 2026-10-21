package main

import (
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
