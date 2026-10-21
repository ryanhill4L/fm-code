package main

import (
	"bufio"
	"context"
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
	maxReadLines   = 150 // a longer file is shown as its start plus an outline
	headLines      = 60
	maxResultChars = 6000
	bashTimeout    = 30 * time.Second
)

// toolbox runs tools confined to root. Mutations go through approve.
type toolbox struct {
	root    string
	approve func(a action, summary string) bool
	read    map[string]bool // files read this session; write_file may only replace those
	// lockTests refuses changes to test files, for requests that didn't ask
	// for test changes: told "the tests fail", the model tends to edit the test.
	lockTests bool
	// runTests reports whether checks may run the project's tests after an edit.
	runTests func() bool
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
	if !within(t.root, abs) {
		return "", fmt.Errorf("path %q is outside the working directory", p)
	}
	// A symlink inside the project can point anywhere: check where it really leads.
	if real := realPath(abs); !within(t.root, real) {
		return "", fmt.Errorf("path %q leads outside the working directory", p)
	}
	rel, _ := filepath.Rel(t.root, abs)
	if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is inside .git, which fm-code doesn't touch", p)
	}
	return abs, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// realPath resolves symlinks in p, or in its deepest existing parent when p
// doesn't exist yet (a file about to be created).
func realPath(p string) string {
	for dir, rest := p, ""; ; {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

func (t *toolbox) run(a action) string {
	if mutates(a.Tool) && a.Tool != "bash" && t.lockTests && isTestFile(a.Path) {
		if p, err := t.resolve(a.Path); err == nil && exists(p) {
			return "error: " + a.Path + " is a test file and the user didn't ask to change tests. Fix the code under test instead."
		}
	}
	var out string
	var err error
	switch a.Tool {
	case "list_dir":
		out, err = t.listDir(a)
	case "read_file", "read_lines":
		out, err = t.readFile(a)
	case "glob":
		out, err = t.glob(a)
	case "grep":
		out, err = t.grep(a)
	case "bash":
		out, err = t.bash(a)
	case "edit_file":
		out, err = t.editFile(a)
	case "write_file":
		out, err = t.writeFile(a)
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
var testFile = regexp.MustCompile(`(_test\.go|^test_.*\.py|_test\.py|\.test\.[jt]s|\.spec\.[jt]s)$`)

func isTestFile(p string) bool { return testFile.MatchString(filepath.Base(p)) }

var asksForTests = regexp.MustCompile(`\b(add|write|create|new|update|change|edit|extend)\b[\w\s]{0,20}\btests?\b|\btests? for\b`)

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
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
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
	whole := a.StartLine == 0 && a.EndLine == 0
	if whole && len(lines) > maxReadLines {
		end = headLines
	} else if end-start+1 > maxReadLines {
		end = start + maxReadLines - 1
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i, lines[i-1])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "[showing lines %d-%d of %d. Use read_lines to see more.]\n", start, end, len(lines))
		if whole {
			b.WriteString("Outline of the rest:\n" + outline(lines, end))
		}
	}
	return b.String(), nil
}

var declaration = regexp.MustCompile(`^(func |type |const |var |class |def |async def |export |function |async function )`)

// outline lists the top-level declarations after line `after`, with line
// numbers, so the model can find its way around a file too big to show.
func outline(lines []string, after int) string {
	var b strings.Builder
	for i := after; i < len(lines); i++ {
		if declaration.MatchString(lines[i]) {
			fmt.Fprintf(&b, "%d\t%s\n", i+1, clip(strings.TrimRight(lines[i], " {"), 100))
		}
	}
	if b.Len() == 0 {
		return "(no top-level declarations)\n"
	}
	return b.String()
}

func (t *toolbox) grep(a action) (string, error) {
	pattern := a.Pattern
	if strings.ToLower(pattern) == pattern {
		pattern = "(?i)" + pattern // smart case: the model guesses "port" for PORT
	}
	re, err := regexp.Compile(pattern)
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

// pythonAlias makes `python` mean python3 on systems that only have python3,
// because the model reaches for `python` first and gives up when it's missing.
func pythonAlias() string {
	if _, err := exec.LookPath("python"); err == nil {
		return ""
	}
	if _, err := exec.LookPath("python3"); err != nil {
		return ""
	}
	return `python() { python3 "$@"; }; `
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
	out, err := runLimited(ctx, t.root, "bash", "-c", pythonAlias()+a.Command)
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
