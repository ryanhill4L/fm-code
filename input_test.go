package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditHistory(t *testing.T) {
	opts := editOpts{history: []string{"first", "second"}}
	up, down := "\x1b[A", "\x1b[B"
	cases := map[string]string{
		up + "\r":                  "second",
		up + up + "\r":             "first",
		up + up + up + "\r":        "first", // stops at the oldest
		"dra" + up + down + "ft\r": "draft", // down past the newest restores the draft
		up + "\x15new\r":           "new",
		up + up + down + "\x7f!\r": "secon!",
		down + "x\r":               "x", // down with nothing browsed is a no-op
	}
	for keys, want := range cases {
		if got, _ := edit(keysOf(keys), io.Discard, "> ", nil, 80, opts); got != want {
			t.Errorf("keys %q: got %q, want %q", keys, got, want)
		}
	}
}

func TestEditShiftTabCyclesLabel(t *testing.T) {
	labels := []string{"[auto] ", "[autopilot] "}
	calls := 0
	opts := editOpts{label: "", cycleMode: func() string { calls++; return labels[calls-1] }}
	var out strings.Builder
	got, _ := edit(keysOf("a\x1b[Zb\x1b[Z\r"), &out, "> ", nil, 80, opts)
	if got != "ab" || calls != 2 {
		t.Fatalf("got %q after %d cycles", got, calls)
	}
	if !strings.HasSuffix(out.String(), "\r\x1b[J[autopilot] > ab\r\n") {
		t.Errorf("final render should show the new label: %q", out.String())
	}
}

func TestHistoryPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := loadHistory(path)
	for _, l := range []string{"one", "two", "two", "  ", "three"} {
		h.add(l)
	}
	if got := strings.Join(loadHistory(path).lines, ","); got != "one,two,three" {
		t.Errorf("reloaded history = %q", got)
	}
}

func TestEmptyHistoryFileHasNoBlankEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	os.WriteFile(path, nil, 0o600)
	if h := loadHistory(path); len(h.lines) != 0 {
		t.Errorf("got %q", h.lines)
	}
}

func TestTwoSessionsKeepEachOthersHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	a, b := loadHistory(path), loadHistory(path)
	a.add("from a")
	b.add("from b")
	if got := strings.Join(loadHistory(path).lines, ","); got != "from a,from b" {
		t.Errorf("got %q", got)
	}
}
