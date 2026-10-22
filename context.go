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

var (
	wordPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}|\b\d{2,}\b`)
	// identPattern finds code identifiers in a request: camelCase, snake_case or a call.
	identPattern = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9]*([A-Z_][A-Za-z0-9_]*)+\b|\b[A-Za-z_]\w*\(`)
)

// stopWords are request words too common to say which file matters.
var stopWords = map[string]bool{
	"what": true, "which": true, "where": true, "does": true, "this": true, "that": true, "with": true,
	"from": true, "into": true, "file": true, "files": true, "function": true, "code": true, "project": true,
	"make": true, "should": true, "would": true, "there": true, "have": true, "when": true, "then": true,
	"tell": true, "show": true, "list": true, "change": true, "add": true, "fix": true, "the": true,
	"print": true, "prints": true, "instead": true, "every": true, "used": true, "uses": true,
}

const (
	maxContextFiles = 4  // files shown at the start of a turn
	excerptRadius   = 6  // lines kept around each match in a file too big to show whole
	maxExcerptLines = 45 // lines kept from one big file
)

type candidate struct {
	path, data string
	score      float64
	hits       []string // terms to center excerpts on
}

// relevantFiles picks the files a request is most likely about and returns
// their numbered contents, or excerpts around the matches for big files, up to
// contextBudget characters. A small model is poor at searching but good at
// reading what is in front of it, so the harness does the searching.
//
// Scoring favours what singles a file out: a file named after a request word,
// an identifier from the request (most of all where it is defined), and rare
// words. A word found in most files, as a README matches everything, counts
// for almost nothing.
func (t *toolbox) relevantFiles(input string) string {
	lower := strings.ToLower(input)
	words := map[string]bool{}
	for _, w := range wordPattern.FindAllString(input, -1) {
		if w = strings.ToLower(w); !stopWords[w] {
			words[w] = true
		}
	}
	idents := map[string]bool{}
	for _, id := range identPattern.FindAllString(input, -1) {
		idents[strings.TrimSuffix(id, "(")] = true
	}
	var files []candidate
	df := map[string]int{}
	for _, rel := range shallowFirst(projectFiles(t.root, 5000), 400) {
		data, err := os.ReadFile(filepath.Join(t.root, rel))
		if err != nil || strings.IndexByte(string(data), 0) >= 0 {
			continue
		}
		c := candidate{path: rel, data: string(data)}
		content := strings.ToLower(c.data)
		for w := range words {
			if strings.Contains(content, w) {
				df[w]++
			}
		}
		files = append(files, c)
	}
	for i := range files {
		c := &files[i]
		base := strings.ToLower(filepath.Base(c.path))
		if strings.Contains(lower, strings.ToLower(c.path)) || strings.Contains(lower, base) {
			c.score += 100
		} else if stem := strings.TrimSuffix(base, filepath.Ext(base)); words[stem] {
			c.score += 60
		}
		content := strings.ToLower(c.data)
		for w := range words {
			if strings.Contains(content, w) && df[w] <= len(files)/2+1 {
				c.score += 20 / float64(df[w])
				c.hits = append(c.hits, w)
			}
		}
		for id := range idents {
			if !strings.Contains(c.data, id) {
				continue
			}
			c.hits = append(c.hits, strings.ToLower(id))
			c.score += 40
			if definesIdent(c.data, id) {
				c.score += 80
			}
		}
		if isTestFile(c.path) && !asksForTests.MatchString(lower) && !strings.Contains(lower, "test") {
			c.score /= 2
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].score > files[j].score })
	var b strings.Builder
	left, shown := contextBudget, 0
	for _, f := range files {
		if f.score < 5 || f.score < files[0].score/3 || shown == maxContextFiles {
			break
		}
		var text string
		if numberedSize(f.data) <= left {
			text, _ = t.readFile(action{Path: f.path}) // marks it read, so only once it will be shown
		} else {
			text = excerpt(f.data, f.hits, left)
		}
		if text == "" || len(text) > left {
			continue
		}
		left -= len(text)
		shown++
		fmt.Fprintf(&b, "\n== %s ==\n%s", f.path, text)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n\nFiles that look relevant (current contents, so you don't need to read them again):" + b.String()
}

// numberedSize is an upper bound on how long data is once read_file numbers its lines.
func numberedSize(data string) int {
	n := strings.Count(data, "\n") + 1
	return len(data) + n*(len(fmt.Sprint(n))+1)
}

// definesIdent reports whether src declares id rather than just using it.
func definesIdent(src, id string) bool {
	q := regexp.QuoteMeta(id)
	return regexp.MustCompile(`(?m)(\b(func|var|const|type|def|class|let|function|struct|enum)\s+(\([^)]*\)\s*)?` + q + `\b|^\s*` + q + `\s*(:=|=[^=]))`).MatchString(src)
}

// excerpt returns numbered windows of src around lines containing any of the
// terms, labelled with the file's length, within limit characters.
func excerpt(src string, terms []string, limit int) string {
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	keep := make([]bool, len(lines))
	kept := 0
	for i, l := range lines {
		ll := strings.ToLower(l)
		for _, term := range terms {
			if !strings.Contains(ll, term) {
				continue
			}
			for j := max(0, i-excerptRadius); j <= min(len(lines)-1, i+excerptRadius) && kept < maxExcerptLines; j++ {
				if !keep[j] {
					keep[j] = true
					kept++
				}
			}
			break
		}
	}
	if kept == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(excerpts; the file has %d lines, use read_lines for more)\n", len(lines))
	gap := false
	for i, l := range lines {
		if !keep[i] {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteString("...\n")
		}
		gap = false
		line := fmt.Sprintf("%d\t%s\n", i+1, l)
		if b.Len()+len(line) > limit {
			break
		}
		b.WriteString(line)
	}
	return b.String()
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

// shallowFirst keeps the n least nested files, so a deep pile of fixtures
// can't crowd the top-level sources out of the search.
func shallowFirst(files []string, n int) []string {
	files = append([]string(nil), files...)
	sort.SliceStable(files, func(i, j int) bool {
		return strings.Count(files[i], string(filepath.Separator)) < strings.Count(files[j], string(filepath.Separator))
	})
	return files[:min(n, len(files))]
}

// projectTree lists the project's files so the model never has to guess a
// path. A directory with many files is folded into one line with its count,
// so a pile of fixtures or assets can't push the source files out of view.
func projectTree(root string, limit int) string {
	files := projectFiles(root, 5000)
	if len(files) == 0 {
		return "(the project is empty)"
	}
	return strings.Join(foldTree(files, limit), "\n")
}

// foldTree folds the deepest crowded directories first until the listing fits.
func foldTree(files []string, limit int) []string {
	entries := append([]string(nil), files...)
	for depth := maxDepth(files); len(entries) > limit && depth > 0; depth-- {
		count := map[string]int{}
		for _, f := range files {
			if d := dirAt(f, depth); d != "" {
				count[d]++
			}
		}
		seen := map[string]bool{}
		var next []string
		for _, e := range entries {
			d := dirAt(strings.SplitN(e, " (", 2)[0], depth)
			if d == "" || count[d] < 4 {
				next = append(next, e)
			} else if !seen[d] {
				seen[d] = true
				next = append(next, fmt.Sprintf("%s/ (%d files)", d, count[d]))
			}
		}
		entries = next
	}
	if len(entries) > limit {
		entries = append(entries[:limit], fmt.Sprintf("... and %d more (use glob to find them)", len(entries)-limit))
	}
	return entries
}

// dirAt returns the directory of path at the given depth (1 = top level), or
// "" if the path is not that deep.
func dirAt(path string, depth int) string {
	parts := strings.Split(filepath.ToSlash(strings.TrimSuffix(path, "/")), "/")
	if len(parts) <= depth {
		return ""
	}
	return strings.Join(parts[:depth], "/")
}

func maxDepth(files []string) int {
	d := 0
	for _, f := range files {
		d = max(d, strings.Count(filepath.ToSlash(f), "/"))
	}
	return d
}
