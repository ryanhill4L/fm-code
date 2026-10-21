package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeModel struct {
	reply string
	got   []message
}

func (f *fakeModel) complete(_ context.Context, msgs []message, _ any) (string, error) {
	f.got = msgs
	return f.reply, nil
}

func TestSuggestNextForksHistory(t *testing.T) {
	history := []message{{"user", "fix main.go"}, {"assistant", `{"tool":"reply","message":"done"}`}}
	m := &fakeModel{reply: `{"next": "Run the tests"}`}

	got, err := suggestNext(context.Background(), m, history)
	if err != nil || got != "Run the tests" {
		t.Fatalf("got %q, %v", got, err)
	}
	if len(history) != 2 {
		t.Errorf("suggestNext mutated the agent's history: %v", history)
	}
	if m.got[0].Content != suggestPrompt || m.got[len(m.got)-1].Content != suggestInstruction {
		t.Errorf("fork should use the suggester prompt and end with the instruction: %v", m.got)
	}
	if m.got[1].Content != "fix main.go" {
		t.Errorf("fork should carry the conversation: %v", m.got)
	}
}

func TestCleanSuggestion(t *testing.T) {
	cases := map[string]string{
		`  "Run the tests"  `:    "Run the tests",
		"Run tests\nbecause why": "Run tests",
		"/clear":                 "",
		"exit":                   "",
		"":                       "",
		strings.Repeat("a", 200): strings.Repeat("a", maxSuggestionRunes),
	}
	for in, want := range cases {
		if got := cleanSuggestion(in); got != want {
			t.Errorf("cleanSuggestion(%q) = %q, want %q", in, got, want)
		}
	}
}

func keysOf(s string) chan byte {
	ch := make(chan byte, len(s)+1)
	for i := 0; i < len(s); i++ {
		ch <- s[i]
	}
	return ch
}

func TestEditRightArrowAcceptsSuggestion(t *testing.T) {
	sugg := make(chan string, 1)
	sugg <- "Run the tests"
	// Let the suggestion arrive before the keys: right arrow, then Enter.
	keys := make(chan byte, 8)
	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, b := range []byte("\x1b[C\r") {
			keys <- b
		}
	}()
	var out bytes.Buffer
	got, err := edit(keys, &out, "> ", sugg, 80, editOpts{})
	if err != nil || got != "Run the tests" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !strings.Contains(out.String(), "Run the tests") {
		t.Error("ghost text was never rendered")
	}
}

func TestEditRightArrowIgnoredWhenTypingDiverges(t *testing.T) {
	sugg := make(chan string, 1)
	sugg <- "Run the tests"
	keys := make(chan byte, 16)
	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, b := range []byte("hi\x1b[C\r") {
			keys <- b
		}
	}()
	got, _ := edit(keys, io.Discard, "> ", sugg, 80, editOpts{})
	if got != "hi" {
		t.Errorf("got %q, want the typed text untouched", got)
	}
}

func TestEditTypingBackspaceAndCompletion(t *testing.T) {
	sugg := make(chan string, 1)
	sugg <- "Run the tests"
	keys := make(chan byte, 32)
	go func() {
		time.Sleep(50 * time.Millisecond)
		// Type "Rux", fix to "Ru", then complete the rest with the right arrow.
		for _, b := range []byte("Rux\x7f\x1b[C\r") {
			keys <- b
		}
	}()
	if got, _ := edit(keys, io.Discard, "> ", sugg, 80, editOpts{}); got != "Run the tests" {
		t.Errorf("got %q", got)
	}
}

func TestEditControlKeys(t *testing.T) {
	if got, err := edit(keysOf("abc\x15x\r"), io.Discard, "> ", nil, 80, editOpts{}); got != "x" || err != nil {
		t.Errorf("Ctrl-U: got %q, %v", got, err)
	}
	if _, err := edit(keysOf("\x04"), io.Discard, "> ", nil, 80, editOpts{}); err != io.EOF {
		t.Errorf("Ctrl-D on empty line: err = %v", err)
	}
	if _, err := edit(keysOf("a\x03"), io.Discard, "> ", nil, 80, editOpts{}); err != errInterrupt {
		t.Errorf("Ctrl-C: err = %v", err)
	}
	// A bare Escape press (nothing follows it) must not swallow the next real key.
	keys := make(chan byte, 4)
	keys <- 27
	go func() {
		time.Sleep(100 * time.Millisecond)
		keys <- 'q'
		keys <- '\r'
	}()
	if got, _ := edit(keys, io.Discard, "> ", nil, 80, editOpts{}); got != "q" {
		t.Errorf("bare escape: got %q", got)
	}
}

func TestEditAcceptsSS3RightArrow(t *testing.T) {
	sugg := make(chan string, 1)
	sugg <- "Run the tests"
	keys := make(chan byte, 8)
	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, b := range []byte("\x1bOC\r") {
			keys <- b
		}
	}()
	if got, _ := edit(keys, io.Discard, "> ", sugg, 80, editOpts{}); got != "Run the tests" {
		t.Errorf("got %q", got)
	}
}

func TestEditCtrlWDeletesWord(t *testing.T) {
	if got, _ := edit(keysOf("fix the bug  \x17x\r"), io.Discard, "> ", nil, 80, editOpts{}); got != "fix the x" {
		t.Errorf("got %q", got)
	}
}

func TestEditRedrawsWrappedInput(t *testing.T) {
	var out bytes.Buffer
	// "> " plus 20 characters fills two 10-column rows and starts a third.
	got, _ := edit(keysOf(strings.Repeat("a", 20)+"\x7f\r"), &out, "\x1b[1m> \x1b[0m", nil, 10, editOpts{})
	if got != strings.Repeat("a", 19) {
		t.Fatalf("got %q", got)
	}
	// The redraw after the backspace must climb back to the prompt's row first.
	frames := strings.Split(out.String(), "\r\x1b[J")
	if last := frames[len(frames)-2]; !strings.HasSuffix(last, "\x1b[2A") {
		t.Errorf("redraw after wrapping didn't move up two rows: %q", last)
	}
}

func TestEditClipsGhostTextToRow(t *testing.T) {
	sugg := make(chan string, 1)
	sugg <- "Run the tests"
	keys := make(chan byte, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		keys <- '\r'
	}()
	var out bytes.Buffer
	edit(keys, &out, "> ", sugg, 10, editOpts{})
	if s := out.String(); strings.Contains(s, "Run the ") || !strings.Contains(s, "Run the") {
		t.Errorf("ghost text should stop at the row's last free column: %q", s)
	}
}
