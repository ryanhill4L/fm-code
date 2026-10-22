package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func wordRe(name string) *regexp.Regexp { return regexp.MustCompile(`\b` + name + `\b`) }

// notRenamable are words a bare old_text names too loosely to rename everywhere.
var notRenamable = map[string]bool{}

func init() {
	for _, w := range strings.Fields("True False None nil true false null undefined self this return if else for while def func class import from and or not in is") {
		notRenamable[w] = true
	}
}

// isRename reports whether old and new are bare identifiers and old is used
// more than once as code in src, so the edit means renaming it.
func isRename(src, old, new, comment string) bool {
	return ident.MatchString(old) && ident.MatchString(new) && len(old) >= 3 &&
		!notRenamable[old] && len(codeUses(src, old, comment)) > 1
}

// codeUses finds the whole-word uses of name in src outside strings and comments.
func codeUses(src, name, comment string) [][]int {
	code := codeMask(src, comment)
	var uses [][]int
	for _, m := range wordRe(name).FindAllStringIndex(src, -1) {
		if code[m[0]] {
			uses = append(uses, m)
		}
	}
	return uses
}

// lineComment is the line comment marker for a file's language.
func lineComment(path string) string {
	switch filepath.Ext(path) {
	case ".py", ".sh", ".bash", ".rb", ".yaml", ".yml", ".toml", ".r", ".pl":
		return "#"
	}
	return "//"
}

// codeMask marks the bytes of src that are code rather than string literals
// or comments. Quoted strings end at their line; backtick strings may span lines.
func codeMask(src, comment string) []bool {
	code := make([]bool, len(src))
	for i := 0; i < len(src); {
		switch c := src[i]; {
		case strings.HasPrefix(src[i:], comment):
			i = lineEnd(src, i)
		case comment == "//" && strings.HasPrefix(src[i:], "/*"):
			if j := strings.Index(src[i+2:], "*/"); j >= 0 {
				i += j + 4
			} else {
				i = len(src)
			}
		case c == '"' || c == '\'':
			for i++; i < len(src) && src[i] != c && src[i] != '\n'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
			if i < len(src) && src[i] == c {
				i++
			}
		case c == '`':
			if j := strings.IndexByte(src[i+1:], '`'); j >= 0 {
				i += j + 2
			} else {
				i = len(src)
			}
		default:
			code[i] = true
			i++
		}
	}
	return code
}

// lineEnd is the index of the newline at or after i, or len(s).
func lineEnd(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(s)
}

// rename replaces every whole-word use of an identifier in one file's code,
// which is what a small model means when it edits a bare name that appears
// many times. Strings and comments are left alone.
func (t *toolbox) rename(a action, p, src string, crlf bool) (string, error) {
	uses := codeUses(src, a.OldText, lineComment(p))
	n := len(uses)
	if !t.approve(a, fmt.Sprintf("rename %s to %s in %s (%d places)", a.OldText, a.NewText, a.Path, n)) {
		return "", fmt.Errorf("user declined this change")
	}
	var b strings.Builder
	last := 0
	for _, m := range uses {
		b.WriteString(src[last:m[0]] + a.NewText)
		last = m[1]
	}
	b.WriteString(src[last:])
	if err := writeText(p, b.String(), crlf); err != nil {
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
// for "{", ignoring braces in strings and comments. Other matches are returned
// unchanged. It reports false when the braces never balance.
func blockEnd(src string, start, end int) (int, bool) {
	header := strings.TrimSpace(src[start:end])
	if strings.Contains(header, "\n") || !(strings.HasSuffix(header, ":") || strings.HasSuffix(header, "{")) {
		return end, true
	}
	lineStart := strings.LastIndex(src[:start], "\n") + 1
	headerEnd := lineEnd(src, start+len(strings.TrimRight(src[start:end], " \t\r\n")))
	pos := headerEnd
	if strings.HasSuffix(header, "{") {
		code, depth, closed := codeMask(src, "//"), 0, false
		for i := lineStart; i < len(src) && !closed; i++ {
			if !code[i] || (src[i] != '{' && src[i] != '}') {
				continue
			}
			if src[i] == '{' {
				depth++
			} else if depth--; depth < 0 {
				return end, false
			} else if depth == 0 && i >= headerEnd {
				pos, closed = lineEnd(src, i), true
			}
		}
		if !closed {
			return end, false
		}
	} else {
		indent := len(indentOf(src[lineStart:]))
		for pos < len(src) {
			next := lineEnd(src, pos+1)
			if line := src[pos+1 : next]; strings.TrimSpace(line) != "" && len(indentOf(line)) <= indent {
				break
			}
			pos = next
		}
	}
	// old_text that took the header's newline takes the block's too.
	if strings.HasSuffix(src[start:end], "\n") && pos < len(src) {
		pos++
	}
	return pos, true
}

// locateJSON accepts an old_text that is the whole JSON document, or an
// object within it, written differently (the model tends to quote a
// pretty-printed file on one line), and writes the result in the file's
// indentation.
func locateJSON(src, old, new string, notFound error) (start, end int, replacement string, err error) {
	var file, quoted any
	if json.Unmarshal([]byte(src), &file) != nil || json.Unmarshal([]byte(old), &quoted) != nil || !reflect.DeepEqual(file, quoted) {
		if out, ok := patchJSON(src, old, new); ok {
			return 0, len(strings.TrimRight(src, "\n")), out, nil
		}
		return 0, 0, "", notFound
	}
	indent := "  "
	if lines := strings.Split(src, "\n"); len(lines) > 1 && indentOf(lines[1]) != "" {
		indent = indentOf(lines[1])
	}
	var out bytes.Buffer // json.Indent keeps the keys in the order written
	if json.Indent(&out, []byte(new), "", indent) != nil {
		return 0, 0, "", fmt.Errorf("new_text is not valid JSON")
	}
	return 0, len(strings.TrimRight(src, "\n")), out.String(), nil
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
	// Match and edit with "\n" line endings; writeText restores "\r\n".
	src := string(data)
	crlf := strings.Contains(src, "\r\n")
	if crlf {
		src = strings.ReplaceAll(src, "\r\n", "\n")
		a.OldText = strings.ReplaceAll(a.OldText, "\r\n", "\n")
		a.NewText = strings.ReplaceAll(a.NewText, "\r\n", "\n")
	}
	if isRename(src, a.OldText, a.NewText, lineComment(p)) {
		return t.rename(a, p, src, crlf)
	}
	start, end, newText, err := locate(src, a.OldText, a.NewText)
	if err != nil && filepath.Ext(p) == ".json" {
		start, end, newText, err = locateJSON(src, a.OldText, a.NewText, err)
	}
	if err != nil {
		// Show the file so the next attempt can copy real lines instead of guessing again.
		shown, _ := t.readFile(action{Path: a.Path})
		return "", fmt.Errorf("%v in %s. Here is the file:\n%s", err, a.Path, shown)
	}
	if strings.TrimSpace(newText) == "" {
		// Deleting a def/func header means deleting its body too.
		var ok bool
		if end, ok = blockEnd(src, start, end); !ok {
			return "", fmt.Errorf("old_text opens a block whose end can't be found, so nothing was deleted. Put the whole block, through its closing line, in old_text")
		}
	}
	if !t.approve(a, fmt.Sprintf("edit %s\n%s", a.Path, diff(src[start:end], newText))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := writeText(p, src[:start]+newText+src[end:], crlf); err != nil {
		return "", err
	}
	t.markRead(p)
	return "edited " + a.Path + t.check(p), nil
}

// writeText writes s to p, with "\r\n" line endings if crlf.
func writeText(p, s string, crlf bool) error {
	if crlf {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return os.WriteFile(p, []byte(s), 0o644)
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
	if !numberedLine.MatchString(src) {
		new = stripLineNumbers(new)
	}
	if strings.Contains(old, `\n`) && !strings.Contains(src, `\n`) {
		// The model escaped its newlines twice, so they arrived as a literal \n.
		unescape := strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\t`, "\t")
		old, new = unescape.Replace(old), unescape.Replace(new)
	}
	if numberOnlyLine.MatchString(old) && !numberOnlyLine.MatchString(src) {
		copied := map[string]bool{}
		for _, m := range numberOnlyLine.FindAllString(old, -1) {
			copied[strings.TrimSpace(m)] = true
		}
		old = numberOnlyLine.ReplaceAllString(old, "")
		new = numberOnlyLine.ReplaceAllStringFunc(new, func(m string) string {
			if copied[strings.TrimSpace(m)] {
				return ""
			}
			return m
		})
	}
	switch at := wordMatches(src, old); {
	case len(at) > 1:
		return 0, 0, "", fmt.Errorf("old_text matches %d places; include more surrounding lines to make it unique", len(at))
	case len(at) == 1:
		start = at[0]
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
		indented := len(oldLines) > 1 && strings.HasPrefix(oldLines[1], indent) || len(oldLines) == 1 && laterLinesIndented(new, indent)
		if indented && !startsIndented(new) {
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
		if start, end, repl, ok := matchDeclaration(src, lines, want, new); ok {
			return start, end, repl, nil
		}
		if start, end, repl, ok := matchLineSet(lines, want, new); ok {
			return start, end, repl, nil
		}
		if start, end, repl, ok := matchTokens(lines, want, new); ok {
			return start, end, repl, nil
		}
		return 0, 0, "", fmt.Errorf("old_text not found%s; read the file again and copy the lines exactly", closestLine(lines, want[0]))
	}
	start, end = lineSpan(lines, found, found+len(want))
	return start, end, indentLines(dedent(tabsLike(lines[found:found+len(want)], want, new)), indentOf(lines[found])), nil
}

// tabsLike rewrites new's space indentation as tabs when old_text used spaces
// where the file has tabs, as a Makefile requires.
func tabsLike(have, want []string, new string) string {
	for j, w := range want {
		fi, wi := indentOf(have[j]), indentOf(w)
		if fi == "" || strings.Trim(fi, "\t") != "" || wi == "" || strings.Trim(wi, " ") != "" || len(wi)%len(fi) != 0 {
			continue
		}
		unit := len(wi) / len(fi)
		lines := strings.Split(new, "\n")
		for i, l := range lines {
			n := len(l) - len(strings.TrimLeft(l, " "))
			lines[i] = strings.Repeat("\t", (n+unit-1)/unit) + l[n:] // a partial indent rounds up to a tab
		}
		return strings.Join(lines, "\n")
	}
	return new
}

var token = regexp.MustCompile(`[A-Za-z0-9_.\-]+`)

// matchTokens handles a one-line old_text with the right words but the wrong
// punctuation, e.g. version "1.2.0" for "version": "1.2.0",. If exactly one
// line holds old_text's words in order, the words that new_text changes are
// swapped in place and the line's own punctuation is kept.
func matchTokens(lines, want []string, new string) (start, end int, repl string, ok bool) {
	oldT, newT := token.FindAllString(want[0], -1), token.FindAllString(new, -1)
	if len(want) != 1 || strings.Contains(new, "\n") || len(oldT) == 0 || len(oldT) != len(newT) || punctuation(want[0]) != punctuation(new) {
		return 0, 0, "", false // only words may change, or the edit would be lost
	}
	// Outside JSON, words inside a string literal don't count: x = 1 is not "x 1".
	data := json.Valid([]byte(strings.Join(lines, "")))
	found, at := -1, 0
	for i, l := range lines {
		var lt []string
		code := codeMask(l, "\x00")
		for _, sp := range token.FindAllStringIndex(l, -1) {
			if data || code[sp[0]] {
				lt = append(lt, l[sp[0]:sp[1]])
			} else {
				lt = append(lt, "\x00") // a placeholder no word equals
			}
		}
		for k := 0; k+len(oldT) <= len(lt); k++ {
			if slices.Equal(lt[k:k+len(oldT)], oldT) {
				if found >= 0 {
					return 0, 0, "", false
				}
				found, at = i, k
				break
			}
		}
	}
	if found < 0 {
		return 0, 0, "", false
	}
	line := strings.TrimSuffix(lines[found], "\n")
	spans := token.FindAllStringIndex(line, -1)[at : at+len(oldT)]
	for k := len(spans) - 1; k >= 0; k-- {
		line = line[:spans[k][0]] + newT[k] + line[spans[k][1]:]
	}
	if line == strings.TrimSuffix(lines[found], "\n") {
		return 0, 0, "", false
	}
	start, end = lineSpan(lines, found, found+1)
	return start, end, line, true
}

// punctuation is s without its words or spaces.
func punctuation(s string) string {
	return strings.Join(strings.Fields(token.ReplaceAllString(s, " ")), "")
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

// wordMatches finds where old occurs in src, skipping matches that start or
// end in the middle of a word ("x = 1" inside "max = 1").
func wordMatches(src, old string) []int {
	var at []int
	for i := 0; ; {
		j := strings.Index(src[i:], old)
		if j < 0 {
			return at
		}
		s, e := i+j, i+j+len(old)
		if !(isWord(old[0]) && s > 0 && isWord(src[s-1])) && !(isWord(old[len(old)-1]) && e < len(src) && isWord(src[e])) {
			at = append(at, s)
			i = e
		} else {
			i = s + 1
		}
	}
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// laterLinesIndented reports whether a multi-line new_text for a one-line
// old_text was written at file level: its later lines already carry indent.
// After a block opener they must be deeper than indent, or they could just be
// the block's body written from column zero.
func laterLinesIndented(new, indent string) bool {
	lines := strings.Split(strings.TrimRight(new, "\n"), "\n")
	least := -1
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !strings.HasPrefix(l, indent) {
			return false
		}
		if n := len(indentOf(l)); least < 0 || n < least {
			least = n
		}
	}
	first := strings.TrimSpace(lines[0])
	opens := strings.HasSuffix(first, ":") || strings.HasSuffix(first, "{")
	return least >= 0 && (!opens || least > len(indent))
}

var (
	lineNumber   = regexp.MustCompile(`^\s*\d+(\t|\\t)`)
	numberedLine = regexp.MustCompile(`(?m)^\s*\d+\t`)
)

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

// numberOnlyLine is a read_file line number the model copied onto a line of
// its own ("21\nfoo()" for "21\tfoo()").
var numberOnlyLine = regexp.MustCompile(`(?m)^[ \t]*\d+[ \t]*(\n|$)`)

var declName = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?(?:def|func|function|class|type)\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)

// matchDeclaration handles a one-line old_text that misquotes a declaration,
// e.g. "def old_helper():" for "def old_helper(x):": the name picks the line.
// A new_text that brings its own body replaces the old body too.
func matchDeclaration(src string, lines, want []string, new string) (start, end int, repl string, ok bool) {
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
	if hasBody(new, m[1]) {
		if e, closed := blockEnd(src, start, end); closed {
			end = start + len(strings.TrimRight(src[start:e], " \t\r\n"))
		}
	}
	return start, end, indentLines(dedent(new), indentOf(lines[found])), true
}

// hasBody reports whether s declares name and has lines after that declaration.
func hasBody(s, name string) bool {
	lines := strings.Split(strings.TrimRight(s, " \t\n"), "\n")
	for _, l := range lines[:len(lines)-1] {
		if d := declName.FindStringSubmatch(l); d != nil && d[1] == name {
			return true
		}
	}
	return false
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
