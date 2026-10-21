package main

import (
	"bufio"
	"context"
	"fmt"
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

var toolNames = []string{"list_dir", "read_file", "glob", "grep", "write_file", "edit_file", "bash", "reply"}

const (
	maxReadLines   = 200
	maxResultChars = 2500
	bashTimeout    = 30 * time.Second
)

// toolbox runs tools confined to root. Mutations go through approve.
type toolbox struct {
	root    string
	approve func(summary string) bool
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
	default:
		err = fmt.Errorf("unknown tool %q; use one of %s", a.Tool, strings.Join(toolNames, ", "))
	}
	if err != nil {
		return "error: " + err.Error()
	}
	return truncate(out, maxResultChars)
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

func (t *toolbox) writeFile(a action) (string, error) {
	p, err := t.resolve(a.Path)
	if err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !t.approve(fmt.Sprintf("write %s (%d bytes)\n%s", a.Path, len(a.Content), preview(a.Content))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), nil
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
	if a.OldText == "" {
		return "", fmt.Errorf("old_text is required")
	}
	src := string(data)
	switch n := strings.Count(src, a.OldText); {
	case n == 0:
		return "", fmt.Errorf("old_text not found in %s; read the file and copy the text exactly", a.Path)
	case n > 1:
		return "", fmt.Errorf("old_text matches %d places in %s; include more surrounding lines to make it unique", n, a.Path)
	}
	if !t.approve(fmt.Sprintf("edit %s\n%s", a.Path, diff(a.OldText, a.NewText))) {
		return "", fmt.Errorf("user declined this change")
	}
	if err := os.WriteFile(p, []byte(strings.Replace(src, a.OldText, a.NewText, 1)), 0o644); err != nil {
		return "", err
	}
	return "edited " + a.Path, nil
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
	if !t.approve("run: " + a.Command) {
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
