// fm-code is a small coding assistant powered by the on-device Apple Foundation Model (`fm`).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	yes := flag.Bool("yes", false, "same as -mode autopilot")
	modeFlag := flag.String("mode", "ask", "permissions: ask (confirm every change), auto (ask only for risky commands) or autopilot (never ask); Shift+Tab cycles them")
	showVersion := flag.Bool("version", false, "print version and exit")
	noSuggest := flag.Bool("no-suggest", false, "don't suggest the next command (also falls back to plain line input)")
	verbose := flag.Bool("v", false, "print raw model actions")
	tracePath := flag.String("trace", "", "append every model request and response to this JSONL file")
	dir := flag.String("C", ".", "project directory the assistant may read and edit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: fm-code [-C dir] [-mode ask|auto|autopilot] [-yes] [-no-suggest] [-trace file] [-v] [-version] [prompt]\n\nWithout a prompt, starts an interactive session.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("fm-code", version)
		return
	}

	mode, err := parseMode(*modeFlag)
	if err != nil {
		fatal(err)
	}
	if *yes {
		mode = modeAutopilot
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
	var model completer = srv
	if *tracePath != "" {
		tr, err := newTracer(srv, *tracePath)
		if err != nil {
			fatal(err)
		}
		model = tr
	}

	rd := newReader(!*noSuggest)
	if tr, ok := rd.(*ttyReader); ok {
		tr.label = func() string { return mode.label() }
		tr.cycleMode = func() string { mode = mode.next(); return mode.label() }
	}
	tools := &toolbox{root: root, approve: func(a action, summary string) bool {
		fmt.Printf("\n%s\n", summary)
		switch mode {
		case modeAutopilot:
			return true
		case modeAuto:
			if a.Tool != "bash" {
				return true // edits stay inside the project root
			}
			v := judgeCommand(context.Background(), model, a.Command)
			if v.safe {
				fmt.Printf("%s(auto: %s)%s\n", dim, v.reason, reset)
				return true
			}
			fmt.Printf("%s⚠ auto mode held this command: %s%s\n", yellow, v.reason, reset)
		}
		line, _ := rd.ReadLine("apply? [y/N] ", nil)
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
	}}
	ag := &agent{model: model, tools: tools, out: os.Stdout, verbose: *verbose}

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
			fmt.Println("  /clear  reset the conversation (do this often: the model's memory is tiny)\n  /help   show this help\n  exit    quit\n  →       accept the suggested next command shown in grey\n  ↑ ↓     browse earlier prompts\n  ⇧Tab    cycle permission mode: ask → auto (only risky commands ask) → autopilot (nothing asks)")
		case strings.HasPrefix(text, "/"):
			fmt.Printf("%s⏺ unknown command %s (try /help)%s\n", red, text, reset)
		case text != "":
			rd.Remember(text)
			turn(ag, text)
		}
		if err != nil {
			return
		}
		pending = nil
		if rd.SupportsSuggestions() && len(ag.history) > 0 {
			pending, cancelSuggestion = startSuggestion(model, ag.history)
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
