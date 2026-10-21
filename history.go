package main

import (
	"os"
	"path/filepath"
	"strings"
)

const maxHistory = 500

// history is the list of submitted prompts, persisted one per line so
// up-arrow reaches prompts from earlier sessions too.
type history struct {
	path  string // "" keeps history in memory only
	lines []string
}

func historyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".fm-code_history")
}

func loadHistory(path string) *history {
	h := &history{path: path}
	data, err := os.ReadFile(path)
	if err != nil || path == "" {
		return h
	}
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			h.lines = append(h.lines, l)
		}
	}
	if len(h.lines) > maxHistory {
		h.lines = h.lines[len(h.lines)-maxHistory:]
		os.WriteFile(path, []byte(strings.Join(h.lines, "\n")+"\n"), 0o600) // trim on load
	}
	return h
}

func (h *history) add(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\n") || (len(h.lines) > 0 && h.lines[len(h.lines)-1] == line) {
		return
	}
	h.lines = append(h.lines, line)
	if h.path == "" {
		return
	}
	// Append rather than rewrite, so two sessions don't erase each other's lines.
	if f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		f.WriteString(line + "\n")
		f.Close()
	}
}
