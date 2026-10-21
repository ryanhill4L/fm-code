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
	msg := fmt.Sprintf("renamed %s to %s in %d places in %s", a.OldText, a.NewText, n, a.Path)
	if hits, _ := t.grep(action{Pattern: `\b` + a.OldText + `\b`}); hits != "no matches" {
		msg += "\n" + a.OldText + " is still used here; rename it in these files too:\n" + hits
	}
	return msg + t.check(p), nil
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

// blockEnd extends a match that is a single block-opening line (ending in ":"
// or "{") to the end of that block: by indentation for ":" and by brace depth
// for "{". Other matches are returned unchanged.
func blockEnd(src string, start, end int) int {
	header := strings.TrimSpace(src[start:end])
	if strings.Contains(header, "\n") || !(strings.HasSuffix(header, ":") || strings.HasSuffix(header, "{")) {
		return end
	}
	lineStart := strings.LastIndex(src[:start], "\n") + 1
	indent := indentOf(src[lineStart:])
	pos, depth := end, strings.Count(header, "{")-strings.Count(header, "}")
	for pos < len(src) {
		next := strings.IndexByte(src[pos+1:], '\n')
		if next < 0 {
			next = len(src) - pos - 1
		}
		line := src[pos+1 : pos+1+next]
		if strings.HasSuffix(header, "{") {
			if depth <= 0 {
				break
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
		} else if strings.TrimSpace(line) != "" && len(indentOf(line)) <= len(indent) {
			break
		}
		pos += 1 + next
	}
	return pos
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
	if strings.TrimSpace(a.OldText) == "" && strings.TrimSpace(a.NewText) != "" {
		return t.appendFile(action{Tool: "append_file", Path: a.Path, Content: a.NewText}) // what an empty old_text means
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
	if strings.TrimSpace(newText) == "" {
		end = blockEnd(src, start, end) // deleting a def/func header means deleting its body too
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
	old = stripLineNumbers(old)
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
		if start, end, repl, ok := matchDeclaration(lines, want, new); ok {
			return start, end, repl, nil
		}
		if start, end, repl, ok := matchLineSet(lines, want, new); ok {
			return start, end, repl, nil
		}
		return 0, 0, "", fmt.Errorf("old_text not found%s; read the file again and copy the lines exactly", closestLine(lines, want[0]))
	}
	start, end = lineSpan(lines, found, found+len(want))
	return start, end, indentLines(dedent(new), indentOf(lines[found])), nil
}

// lineSpan is the byte range of lines[from:to], without the final newline.
func lineSpan(lines []string, from, to int) (start, end int) {
	for _, l := range lines[:from] {
		start += len(l)
	}
	end = start
	for _, l := range lines[from:to] {
		end += len(l)
	}
	if to > from && strings.HasSuffix(lines[to-1], "\n") {
		end--
	}
	return start, end
}

func indentOf(line string) string { return line[:len(line)-len(strings.TrimLeft(line, " \t"))] }

var lineNumber = regexp.MustCompile(`^\s*\d+(\t|\\t)`)

// stripLineNumbers removes "12\t" prefixes copied from read_file output, but
// only when every line has one.
func stripLineNumbers(s string) string {
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) != "" && !lineNumber.MatchString(l) {
			return s
		}
	}
	for i, l := range lines {
		lines[i] = lineNumber.ReplaceAllString(l, "")
	}
	return strings.Join(lines, "\n")
}

var declName = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?(?:def|func|function|class|type)\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)

// matchDeclaration handles a one-line old_text that misquotes a declaration,
// e.g. "def old_helper():" for "def old_helper(x):": the name picks the line.
func matchDeclaration(lines, want []string, new string) (start, end int, repl string, ok bool) {
	m := declName.FindStringSubmatch(want[0])
	if len(want) != 1 || m == nil {
		return 0, 0, "", false
	}
	found := -1
	for i, l := range lines {
		if d := declName.FindStringSubmatch(l); d != nil && d[1] == m[1] {
			if found >= 0 {
				return 0, 0, "", false
			}
			found = i
		}
	}
	if found < 0 {
		return 0, 0, "", false
	}
	start, end = lineSpan(lines, found, found+1)
	return start, end, indentLines(dedent(new), indentOf(lines[found])), true
}

// matchLineSet handles old_text whose lines each appear once in the file but
// not next to each other, e.g. two settings with another between them. With
// as many new lines as old ones, each line is replaced in place.
func matchLineSet(lines, want []string, new string) (start, end int, repl string, ok bool) {
	newLines := strings.Split(strings.Trim(new, "\n"), "\n")
	if len(want) < 2 || len(newLines) != len(want) {
		return 0, 0, "", false
	}
	at := make([]int, len(want))
	for j, w := range want {
		at[j] = -1
		for i, l := range lines {
			if strings.TrimSpace(l) == strings.TrimSpace(w) && strings.TrimSpace(w) != "" {
				if at[j] >= 0 {
					return 0, 0, "", false
				}
				at[j] = i
			}
		}
		if at[j] < 0 || (j > 0 && at[j] <= at[j-1]) {
			return 0, 0, "", false
		}
	}
	span := append([]string(nil), lines[at[0]:at[len(at)-1]+1]...)
	for j, i := range at {
		l := span[i-at[0]]
		nl := ""
		if strings.HasSuffix(l, "\n") {
			nl = "\n"
		}
		span[i-at[0]] = indentOf(l) + strings.TrimSpace(newLines[j]) + nl
	}
	start, end = lineSpan(lines, at[0], at[len(at)-1]+1)
	return start, end, strings.TrimSuffix(strings.Join(span, ""), "\n"), true
}

// closestLine names the file line most like want, as a hint after a miss.
func closestLine(lines []string, want string) string {
	want = strings.TrimSpace(want)
	best, bestScore := -1, 0.6
	for i, l := range lines {
		if s := similarity(strings.TrimSpace(l), want); s > bestScore {
			best, bestScore = i, s
		}
	}
	if best < 0 {
		return ""
	}
	return fmt.Sprintf(" (the closest line is %d: %s)", best+1, strings.TrimSpace(lines[best]))
}

// similarity is 1 minus the edit distance over the longer length.
func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return 1 - float64(prev[len(b)])/float64(max(len(a), len(b)))
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
