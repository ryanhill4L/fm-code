package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// action is one model turn: a tool name plus whichever arguments that tool uses.
type action struct {
	Tool      string `json:"tool"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	OldText   string `json:"old_text"`
	NewText   string `json:"new_text"`
	Pattern   string `json:"pattern"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Message   string `json:"message"`
	Command   string `json:"command"`
}

const (
	maxReadLines   = 200
	maxResultChars = 6000
	bashTimeout    = 30 * time.Second
)

// toolbox runs tools confined to root. Mutations go through approve.
type toolbox struct {
	root    string
	approve func(a action, summary string) bool
	read    map[string]bool // files read this session; write_file may only replace those
}

func (t *toolbox) resolve(p string) (string, error) {
	if p == "" {
		p = "."
	}
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(t.root, p)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(t.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the working directory", p)
	}
	return abs, nil
}

func (t *toolbox) run(a action) string {
	var out string
	var err error
	switch a.Tool {
	case "list_dir":
		out, err = t.listDir(a)
	case "read_file":
		out, err = t.readFile(a)
	case "glob":
		out, err = t.glob(a)
	case "grep":
		out, err = t.grep(a)
	case "bash":
		out, err = t.bash(a)
	case "write_file":
		out, err = t.writeFile(a)
	case "edit_file":
		out, err = t.editFile(a)
	case "append_file":
		out, err = t.appendFile(a)
	default:
		err = fmt.Errorf("unknown tool %q", a.Tool)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%s does not exist. %s", a.Path, t.nearMiss(a.Path))
		}
		return "error: " + t.scrub(err.Error())
	}
	return truncate(t.scrub(out), maxResultChars)
}

// scrub rewrites absolute project paths as relative ones. The model copies
// whatever it sees, and temp-dir paths send it off on tangents.
func (t *toolbox) scrub(s string) string {
	s = strings.ReplaceAll(s, t.root+string(filepath.Separator), "")
	return strings.ReplaceAll(s, t.root, ".")
}

// nearMiss suggests real files for a path that doesn't exist: same name in another
// directory first, then names that contain the stem, else a sample of the project.
func (t *toolbox) nearMiss(p string) string {
	base := filepath.Base(p)
	stem := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	var same, similar, all []string
	filepath.WalkDir(t.root, func(f string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(t.root, f)
		name := strings.ToLower(d.Name())
		switch {
		case name == strings.ToLower(base):
			same = append(same, rel)
		case stem != "" && strings.Contains(name, stem):
			similar = append(similar, rel)
		}
		all = append(all, rel)
		return nil
	})
	if c := append(same, similar...); len(c) > 0 {
		return "Did you mean " + strings.Join(c[:min(len(c), 5)], " or ") + "?"
	}
	if len(all) == 0 {
		return "The project is empty."
	}
	more := ""
	if len(all) > 15 {
		all, more = all[:15], " ..."
	}
	return "Files in the project: " + strings.Join(all, ", ") + more
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

func (t *toolbox) listDir(a action) (string, error) {
	dir, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		b.WriteString(e.Name())
		if e.IsDir() {
			b.WriteString("/")
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "(empty)", nil
	}
	return b.String(), nil
}

func (t *toolbox) readFile(a action) (string, error) {
	p, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	t.markRead(p)
	lines := strings.Split(string(data), "\n")
	start, end := 1, len(lines)
	if a.StartLine > 0 {
		start = a.StartLine
	}
	if a.EndLine > 0 && a.EndLine < end {
		end = a.EndLine
	}
	if start > len(lines) {
		return "", fmt.Errorf("start_line %d is past end of file (%d lines)", start, len(lines))
	}
	capped := false
	if end-start+1 > maxReadLines {
		end, capped = start+maxReadLines-1, true
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i, lines[i-1])
	}
	if capped || end < len(lines) {
		fmt.Fprintf(&b, "[showing lines %d-%d of %d; use start_line/end_line for more]\n", start, end, len(lines))
	}
	return b.String(), nil
}

func (t *toolbox) grep(a action) (string, error) {
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return "", fmt.Errorf("bad pattern: %v", err)
	}
	root, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	var hits []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || len(hits) >= 50 {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		rel, _ := filepath.Rel(t.root, p)
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan() && len(hits) < 50; n++ {
			line := sc.Text()
			if strings.IndexByte(line, 0) >= 0 {
				return nil // binary
			}
			if re.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, n, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if len(hits) == 0 {
		return "no matches", nil
	}
	sort.Strings(hits)
	return strings.Join(hits, "\n"), nil
}

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

func absDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", p)
	}
	return abs, nil
}

// globRegexp turns a glob with ** support into an anchored regexp.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func (t *toolbox) glob(a action) (string, error) {
	re, err := globRegexp(a.Pattern)
	if err != nil {
		return "", fmt.Errorf("bad pattern: %v", err)
	}
	base, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	var matches []string
	filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || len(matches) >= 100 {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		// A bare pattern like "*.go" matches at any depth, as people expect.
		if re.MatchString(rel) || (!strings.Contains(a.Pattern, "/") && re.MatchString(d.Name())) {
			shown, _ := filepath.Rel(t.root, p)
			matches = append(matches, shown)
		}
		return nil
	})
	if len(matches) == 0 {
		return "no matches", nil
	}
	sort.Strings(matches)
	return strings.Join(matches, "\n"), nil
}

func (t *toolbox) bash(a action) (string, error) {
	if strings.TrimSpace(a.Command) == "" {
		return "", fmt.Errorf("command is required")
	}
	if !t.approve(a, "run: "+a.Command) {
		return "", fmt.Errorf("user declined this command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", a.Command)
	cmd.Dir = t.root
	out, err := cmd.CombinedOutput()
	res := string(out)
	switch {
	case ctx.Err() != nil:
		res += fmt.Sprintf("\n[timed out after %s]", bashTimeout)
	case err != nil:
		res += fmt.Sprintf("\n[%v]", err)
	}
	if strings.TrimSpace(res) == "" {
		return "(no output)", nil
	}
	return res, nil
}
