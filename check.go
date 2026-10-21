package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// check runs a quick compile or syntax check on a file the model just changed,
// so a broken edit is reported straight away instead of claimed as done.
// Either way it ends by telling the model to reply if it's done, because a
// small model otherwise keeps "improving" work that is already finished.
func (t *toolbox) check(p string) string {
	var cmds [][]string
	hasTests := func(pattern string) bool {
		m, _ := filepath.Glob(filepath.Join(t.root, pattern))
		return len(m) > 0
	}
	switch filepath.Ext(p) {
	case ".go":
		if _, err := os.Stat(filepath.Join(t.root, "go.mod")); err != nil {
			cmds = [][]string{{"gofmt", "-e", "-l", p}}
			break
		}
		cmds = [][]string{{"go", "vet", "./..."}}
		if hasTests("*_test.go") {
			cmds = append(cmds, []string{"go", "test", "./..."})
		}
	case ".py":
		cmds = [][]string{{"python3", "-c", "import ast,sys; ast.parse(open(sys.argv[1]).read(), sys.argv[1])", p}}
		if hasTests("test*.py") {
			cmds = append(cmds, []string{"python3", "-m", "unittest", "-q"})
		}
	case ".js", ".mjs", ".cjs":
		cmds = [][]string{{"node", "--check", p}}
	case ".json":
		data, _ := os.ReadFile(p)
		if !json.Valid(data) {
			return "\ncheck FAILED: " + filepath.Base(p) + " is not valid JSON. Fix it before replying."
		}
		return "\ncheck: valid JSON. If the task is done, reply now."
	default:
		return "\nIf the task is done, reply now."
	}
	var ran []string
	for _, cmd := range cmds {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			break
		}
		name := strings.Join(cmd[:2], " ")
		ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		c.Dir = t.root
		c.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1") // checks shouldn't litter the repo
		out, err := c.CombinedOutput()
		cancel()
		if err != nil {
			return "\ncheck FAILED (" + name + "), fix this before replying:\n" + truncate(strings.TrimSpace(string(out)), 1500)
		}
		ran = append(ran, name)
	}
	if len(ran) == 0 {
		return "\nIf the task is done, reply now."
	}
	return "\ncheck: OK (" + strings.Join(ran, ", ") + "). If the task is done, reply now."
}
