// fm-code is a small coding assistant powered by the on-device Apple Foundation Model (`fm`).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	yes := flag.Bool("yes", false, "apply edits without asking for confirmation")
	showVersion := flag.Bool("version", false, "print version and exit")
	noSuggest := flag.Bool("no-suggest", false, "don't suggest the next command (also falls back to plain line input)")
	verbose := flag.Bool("v", false, "print raw model actions")
	dir := flag.String("C", ".", "project directory the assistant may read and edit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: fm-code [-C dir] [-yes] [-no-suggest] [-v] [-version] [prompt]\n\nWithout a prompt, starts an interactive session.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("fm-code", version)
		return
	}

	root, err := absDir(*dir)
	if err != nil {
		fatal(err)
	}
	srv, err := startFM()
	if err != nil {
		fatal(err)
	}
	defer srv.Close()

	rd := newReader(!*noSuggest)
	tools := &toolbox{root: root, approve: func(summary string) bool {
		fmt.Printf("\n%s\n", summary)
		if *yes {
			return true
		}
		line, _ := rd.ReadLine("apply? [y/N] ", nil)
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
	}}
	ag := &agent{model: srv, tools: tools, out: os.Stdout, verbose: *verbose}

	if prompt := strings.Join(flag.Args(), " "); prompt != "" {
		if !turn(ag, prompt) {
			srv.Close()
			os.Exit(1)
		}
		return
	}
	fmt.Printf("%s%s⏺ fm-code%s in %s. /help for commands, Ctrl-D to quit.\n", bold, green, reset, root)
	var pending <-chan string // suggestion for the next prompt
	cancelSuggestion := func() {}
	for {
		fmt.Println()
		line, err := rd.ReadLine(fmt.Sprintf("%s%s❯%s ", bold, cyan, reset), pending)
		cancelSuggestion() // stop the forked request before the real one starts
		if errors.Is(err, errInterrupt) {
			return
		}
		switch text := strings.TrimSpace(line); {
		case text == "exit" || text == "/exit":
			return
		case text == "/clear":
			ag.history = nil
			fmt.Printf("%s⏺ Conversation cleared%s\n", green, reset)
		case text == "/help":
			fmt.Println("  /clear  reset the conversation (do this often: the model's memory is tiny)\n  /help   show this help\n  exit    quit\n  →       accept the suggested next command shown in grey")
		case strings.HasPrefix(text, "/"):
			fmt.Printf("%s⏺ unknown command %s (try /help)%s\n", red, text, reset)
		case text != "":
			turn(ag, text)
		}
		if err != nil {
			return
		}
		pending = nil
		if rd.SupportsSuggestions() && len(ag.history) > 0 {
			pending, cancelSuggestion = startSuggestion(srv, ag.history)
		}
	}
}

// newReader returns a raw-mode reader on an interactive terminal, else a plain line reader.
func newReader(suggest bool) lineReader {
	if fi, err := os.Stdin.Stat(); suggest && err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		if r, err := newTTYReader(os.Stdin, os.Stdout); err == nil {
			return r
		}
	}
	return plainReader{in: bufio.NewReader(os.Stdin), out: os.Stdout}
}

func turn(ag *agent, text string) bool {
	reply, err := ag.Send(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s⏺ Error: %v%s\n", red, err, reset)
		return false
	}
	fmt.Printf("\n%s⏺%s %s\n", cyan, reset, reply)
	return true
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fm-code:", err)
	os.Exit(1)
}
