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
	if data, err := os.ReadFile(path); err == nil && path != "" {
		h.lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	return h
}

func (h *history) add(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\n") || (len(h.lines) > 0 && h.lines[len(h.lines)-1] == line) {
		return
	}
	h.lines = append(h.lines, line)
	if len(h.lines) > maxHistory {
		h.lines = h.lines[len(h.lines)-maxHistory:]
	}
	if h.path != "" {
		os.WriteFile(h.path, []byte(strings.Join(h.lines, "\n")+"\n"), 0o600)
	}
}
