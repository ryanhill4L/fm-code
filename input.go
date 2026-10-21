package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var errInterrupt = errors.New("interrupted")

// lineReader reads one line of user input. If suggestions arrive on sugg while
// the user is typing, a reader that supports it shows them as ghost text.
type lineReader interface {
	ReadLine(prompt string, sugg <-chan string) (string, error)
	// SupportsSuggestions reports whether ReadLine can display suggestions.
	SupportsSuggestions() bool
}

// plainReader reads cooked lines from a pipe or a terminal without raw mode.
type plainReader struct {
	in  *bufio.Reader
	out io.Writer
}

func (r plainReader) SupportsSuggestions() bool { return false }

func (r plainReader) ReadLine(prompt string, _ <-chan string) (string, error) {
	fmt.Fprint(r.out, prompt)
	line, err := r.in.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

// ttyReader puts the terminal in raw mode while reading, so it can handle the
// right arrow key.
type ttyReader struct {
	keys chan byte
	out  io.Writer
}

func newTTYReader(in *os.File, out io.Writer) (*ttyReader, error) {
	restore, err := rawMode()
	if err != nil {
		return nil, err
	}
	restore()
	r := &ttyReader{keys: make(chan byte, 256), out: out}
	go func() {
		defer close(r.keys)
		b := make([]byte, 1)
		for {
			if n, err := in.Read(b); n > 0 {
				r.keys <- b[0]
			} else if err != nil {
				return
			}
		}
	}()
	return r, nil
}

func (r *ttyReader) SupportsSuggestions() bool { return true }

func (r *ttyReader) ReadLine(prompt string, sugg <-chan string) (string, error) {
	restore, err := rawMode()
	if err != nil {
		return "", err
	}
	defer restore()
	// Drop keys typed while the model was working so a stray Enter can't answer a prompt.
	for drained := false; !drained; {
		select {
		case <-r.keys:
		default:
			drained = true
		}
	}
	return edit(r.keys, r.out, prompt, sugg, termWidth())
}

// termWidth returns the terminal's width in columns, or 80 if it can't tell.
func termWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 80
	}
	var rows, cols int
	if _, err := fmt.Sscan(string(out), &rows, &cols); err != nil || cols <= 0 {
		return 80
	}
	return cols
}

// rawMode disables line buffering, echo and signal keys, and returns a func that restores the terminal.
func rawMode() (restore func(), err error) {
	stty := func(args ...string) ([]byte, error) {
		cmd := exec.Command("stty", args...)
		cmd.Stdin = os.Stdin
		return cmd.Output()
	}
	saved, err := stty("-g")
	if err != nil {
		return nil, err
	}
	if _, err := stty("-icanon", "-echo", "-isig"); err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(saved))
	return func() { stty(state) }, nil
}

// edit is a minimal line editor: typing, backspace, Ctrl-U, Ctrl-W, Enter, Ctrl-D,
// Ctrl-C, and right arrow to accept the current suggestion. width is the terminal's
// width in columns, used to redraw input that wraps onto several rows.
func edit(keys <-chan byte, out io.Writer, prompt string, sugg <-chan string, width int) (string, error) {
	var buf []byte
	var suggestion string
	promptWidth := visibleWidth(prompt)
	row := 0 // rows the cursor sits below the prompt's first row

	render := func(final bool) {
		if row > 0 {
			fmt.Fprintf(out, "\x1b[%dA", row)
		}
		fmt.Fprintf(out, "\r\x1b[J%s%s", prompt, buf)
		pos := promptWidth + utf8.RuneCount(buf)
		if pos > 0 && pos%width == 0 {
			// The terminal holds the cursor at the last column until the next
			// character; move it to the next row so row counting stays exact.
			fmt.Fprint(out, "\r\n")
		}
		row = pos / width
		if final {
			return
		}
		// Only show as much ghost text as fits on the cursor's row, so moving
		// back over it never has to cross a line.
		if rest, ok := strings.CutPrefix(suggestion, string(buf)); ok && rest != "" {
			if room := width - 1 - pos%width; utf8.RuneCountInString(rest) > room {
				rest = string([]rune(rest)[:max(room, 0)])
			}
			if n := utf8.RuneCountInString(rest); n > 0 {
				fmt.Fprintf(out, "%s%s%s\x1b[%dD", dim, rest, reset, n)
			}
		}
	}
	render(false)

	for {
		select {
		case s := <-sugg: // a nil channel never fires, which is what we want
			suggestion = s
			render(false)
		case k, ok := <-keys:
			if !ok {
				return string(buf), io.EOF
			}
			switch {
			case k == '\r' || k == '\n':
				render(true)
				fmt.Fprint(out, "\r\n")
				return string(buf), nil
			case k == 3:
				render(true)
				fmt.Fprint(out, "\r\n")
				return "", errInterrupt
			case k == 4 && len(buf) == 0:
				fmt.Fprint(out, "\r\n")
				return "", io.EOF
			case k == 21:
				buf = buf[:0]
			case k == 23:
				buf = bytes.TrimRight(buf, " ")
				buf = buf[:bytes.LastIndexByte(buf, ' ')+1]
			case k == 127 || k == 8:
				_, n := utf8.DecodeLastRune(buf)
				buf = buf[:len(buf)-n]
			case k == 27:
				if readArrow(keys) == 'C' {
					if rest, ok := strings.CutPrefix(suggestion, string(buf)); ok && rest != "" {
						buf = []byte(suggestion)
					}
				}
			case k >= 32 && k != 127:
				buf = append(buf, k)
			}
			render(false)
		}
	}
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// visibleWidth is the number of columns s takes up once its color codes are removed.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansiEscape.ReplaceAllString(s, ""))
}

// readArrow consumes the rest of an escape sequence and returns its final byte,
// or 0 if it isn't a CSI or SS3 one (terminals in application cursor mode send
// the right arrow as ESC O C). A bare Escape press times out.
func readArrow(keys <-chan byte) byte {
	next := func() (byte, bool) {
		select {
		case k, ok := <-keys:
			return k, ok
		case <-time.After(30 * time.Millisecond):
			return 0, false
		}
	}
	if k, ok := next(); !ok || (k != '[' && k != 'O') {
		return 0
	}
	for {
		k, ok := next()
		if !ok {
			return 0
		}
		if k >= 0x40 && k <= 0x7e {
			return k
		}
	}
}
