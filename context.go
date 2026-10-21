package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// contextBudget is how many characters of file contents a turn starts with.
const contextBudget = 6000

var wordPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)

// stopWords are request words too common to say which file matters.
var stopWords = map[string]bool{
	"what": true, "which": true, "where": true, "does": true, "this": true, "that": true, "with": true,
	"from": true, "into": true, "file": true, "files": true, "function": true, "code": true, "project": true,
	"make": true, "should": true, "would": true, "there": true, "have": true, "when": true, "then": true,
	"tell": true, "show": true, "list": true, "change": true, "add": true, "fix": true, "the": true,
	"print": true, "prints": true, "instead": true, "every": true, "used": true, "uses": true,
}

// relevantFiles picks the files a request is most likely about, by name or by
// the request's words appearing in them, and returns their numbered contents
// up to contextBudget characters. A small model is poor at searching but good
// at reading what is in front of it, so the harness does the searching.
func (t *toolbox) relevantFiles(input string) string {
	lower := strings.ToLower(input)
	var words []string
	for _, w := range wordPattern.FindAllString(input, -1) {
		if w = strings.ToLower(w); !stopWords[w] {
			words = append(words, w)
		}
	}
	type scored struct {
		path  string
		data  string
		score int
	}
	var files []scored
	for _, rel := range projectFiles(t.root, 400) {
		data, err := os.ReadFile(filepath.Join(t.root, rel))
		if err != nil || strings.IndexByte(string(data), 0) >= 0 {
			continue
		}
		s := scored{path: rel, data: string(data)}
		if strings.Contains(lower, strings.ToLower(rel)) || strings.Contains(lower, strings.ToLower(filepath.Base(rel))) {
			s.score += 100
		}
		content := strings.ToLower(s.data)
		for _, w := range words {
			if strings.Contains(content, w) {
				s.score += 10
			}
		}
		if s.score > 0 {
			files = append(files, s)
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].score > files[j].score })
	var b strings.Builder
	left := contextBudget
	for _, f := range files {
		if len(f.data) > left {
			continue
		}
		left -= len(f.data)
		p := filepath.Join(t.root, f.path)
		t.markRead(p)
		shown, _ := t.readFile(action{Path: f.path})
		fmt.Fprintf(&b, "\n== %s ==\n%s", f.path, shown)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n\nFiles that look relevant (current contents, so you don't need to read them again):" + b.String()
}

// projectFiles lists the project's files, skipping hidden, vendored and
// generated directories, up to limit entries.
func projectFiles(root string, limit int) []string {
	var files []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || len(files) >= limit {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); p != root && (strings.HasPrefix(n, ".") || skipDirs[n]) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, rel)
		return nil
	})
	return files
}

var skipDirs = map[string]bool{"node_modules": true, "__pycache__": true, "vendor": true, "target": true, "dist": true, "build": true}
