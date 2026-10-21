package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func wordRe(name string) *regexp.Regexp { return regexp.MustCompile(`\b` + name + `\b`) }

// rename replaces every whole-word use of an identifier in one file, which is
// what a small model means when it edits a bare name that appears many times.
func (t *toolbox) rename(a action, p, src string) (string, error) {
	re := wordRe(a.OldText)
	n := len(re.FindAllStringIndex(src, -1))
	if !t.approve(a, fmt.Sprintf("rename %s to %s in %s (%d places)", a.OldText, a.NewText, a.Path, n)) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.WriteFile(p, []byte(re.ReplaceAllLiteralString(src, a.NewText)), 0o644); err != nil {
		return "", err
	}
	t.markRead(p)
	return fmt.Sprintf("renamed %s to %s in %d places in %s", a.OldText, a.NewText, n, a.Path) + t.check(p), nil
}

func (t *toolbox) appendFile(a action) (string, error) {
	p, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	old, err := os.ReadFile(p)
	if err != nil {
		return "", err // appending is for existing files; write_file creates them
	}
	content := strings.TrimRight(a.Content, "\n") + "\n"
	if len(old) > 0 && !strings.HasSuffix(string(old), "\n") {
		content = "\n" + content
	}
	if !t.approve(a, fmt.Sprintf("append to %s\n%s", a.Path, preview(content))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.WriteFile(p, append(old, content...), 0o644); err != nil {
		return "", err
	}
	t.markRead(p)
	return fmt.Sprintf("appended %d lines to %s", strings.Count(content, "\n"), a.Path) + t.check(p), nil
}

func (t *toolbox) markRead(p string) {
	if t.read == nil {
		t.read = map[string]bool{}
	}
	t.read[p] = true
}

func (t *toolbox) writeFile(a action) (string, error) {
	p, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if old, err := os.ReadFile(p); err == nil {
		// The model's favourite mistake is rewriting a whole file with just the new part.
		if !t.read[p] {
			return "", fmt.Errorf("%s already exists and you haven't read it. To change part of it use edit_file; to add to it use edit_file on its last lines", a.Path)
		}
		if lost := lostLines(string(old), a.Content); lost > 0 {
			return "", fmt.Errorf("write_file would delete %d existing lines of %s. Use edit_file to change only the part that needs changing", lost, a.Path)
		}
	}
	if !t.approve(a, fmt.Sprintf("write %s (%d bytes)\n%s", a.Path, len(a.Content), preview(a.Content))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	t.markRead(p)
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path) + t.check(p), nil
}

func (t *toolbox) editFile(a action) (string, error) {
	p, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if a.OldText == a.NewText {
		return "", fmt.Errorf("old_text and new_text are the same, so there is nothing to change. If the task is done, reply")
	}
	src := string(data)
	if ident.MatchString(a.OldText) && ident.MatchString(a.NewText) && len(wordRe(a.OldText).FindAllStringIndex(src, 2)) > 1 {
		return t.rename(a, p, src)
	}
	start, end, newText, err := locate(src, a.OldText, a.NewText)
	if err != nil {
		// Show the file so the next attempt can copy real lines instead of guessing again.
		shown, _ := t.readFile(action{Path: a.Path})
		return "", fmt.Errorf("%v in %s. Here is the file:\n%s", err, a.Path, shown)
	}
	if !t.approve(a, fmt.Sprintf("edit %s\n%s", a.Path, diff(src[start:end], newText))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.WriteFile(p, []byte(src[:start]+newText+src[end:]), 0o644); err != nil {
		return "", err
	}
	t.markRead(p)
	return "edited " + a.Path + t.check(p), nil
}

// lostLines counts the non-blank lines of old that are missing from new,
// once more than a third of them are; fewer is treated as a deliberate rewrite.
func lostLines(old, new string) int {
	have := map[string]int{}
	for _, l := range strings.Split(new, "\n") {
		have[strings.TrimSpace(l)]++
	}
	total, lost := 0, 0
	for _, l := range strings.Split(old, "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		total++
		if have[l] > 0 {
			have[l]--
		} else {
			lost++
		}
	}
	if lost*3 <= total {
		return 0
	}
	return lost
}

// locate finds the one place old occurs in src and returns its byte range and
// the replacement, fixing up the indentation mistakes a small model makes:
// old_text copied without its leading indent, or new_text written as if it
// started at column zero. If old doesn't occur exactly, whole lines that
// match ignoring indentation are accepted.
func locate(src, old, new string) (start, end int, replacement string, err error) {
	if strings.TrimSpace(old) == "" {
		return 0, 0, "", fmt.Errorf("old_text is required")
	}
	switch n := strings.Count(src, old); {
	case n > 1:
		return 0, 0, "", fmt.Errorf("old_text matches %d places; include more surrounding lines to make it unique", n)
	case n == 1:
		start = strings.Index(src, old)
		end = start + len(old)
		lineStart := strings.LastIndex(src[:start], "\n") + 1
		before := src[lineStart:start]
		if before == "" || strings.TrimSpace(before) != "" {
			return start, end, new, nil
		}
		// old_text starts inside the line's indentation: take the whole line and
		// indent new_text to match it.
		line := src[lineStart:]
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		oldLines := strings.Split(old, "\n")
		if len(oldLines) > 1 && strings.HasPrefix(oldLines[1], indent) && !startsIndented(new) {
			return lineStart, end, indent + new, nil // later lines already carry their indent
		}
		return lineStart, end, indentLines(dedent(new), indent), nil
	}
	// Fall back to whole lines that match once leading and trailing space is ignored.
	want := strings.Split(strings.Trim(old, "\n"), "\n")
	lines := strings.SplitAfter(src, "\n")
	found := -1
	for i := 0; i+len(want) <= len(lines); i++ {
		match := true
		for j, w := range want {
			if strings.TrimSpace(lines[i+j]) != strings.TrimSpace(w) {
				match = false
				break
			}
		}
		if match {
			if found >= 0 {
				return 0, 0, "", fmt.Errorf("old_text matches more than one place; include more surrounding lines to make it unique")
			}
			found = i
		}
	}
	if found < 0 {
		return 0, 0, "", fmt.Errorf("old_text not found; read the file again and copy the lines exactly")
	}
	for _, l := range lines[:found] {
		start += len(l)
	}
	end = start
	for _, l := range lines[found : found+len(want)] {
		end += len(l)
	}
	if strings.HasSuffix(src[start:end], "\n") {
		end--
	}
	first := lines[found]
	indent := first[:len(first)-len(strings.TrimLeft(first, " \t"))]
	return start, end, indentLines(dedent(new), indent), nil
}

func startsIndented(s string) bool { return strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\t") }

// indentLines prefixes each non-blank line of s with indent.
func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n")
}

// dedent removes the indentation common to every non-blank line of s.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	common, first := "", true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			common, first = ind, false
		}
		for !strings.HasPrefix(ind, common) {
			common = common[:len(common)-1]
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, common)
	}
	return strings.Join(lines, "\n")
}

func preview(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 15 {
		lines = append(lines[:15], fmt.Sprintf("... (%d more lines)", len(lines)-15))
	}
	return "  " + strings.Join(lines, "\n  ")
}

func diff(old, new string) string {
	var b strings.Builder
	for _, l := range strings.Split(old, "\n") {
		b.WriteString("\x1b[31m- " + l + "\x1b[0m\n")
	}
	for _, l := range strings.Split(new, "\n") {
		b.WriteString("\x1b[32m+ " + l + "\x1b[0m\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
