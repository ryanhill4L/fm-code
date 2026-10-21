// fm-code is a small coding assistant powered by the on-device Apple Foundation Model (`fm`).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	yes := flag.Bool("yes", false, "apply edits without asking for confirmation")
	verbose := flag.Bool("v", false, "print raw model actions")
	dir := flag.String("C", ".", "project directory the assistant may read and edit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: fm-code [-C dir] [-yes] [prompt]\n\nWithout a prompt, starts an interactive session.")
		flag.PrintDefaults()
	}
	flag.Parse()

	root, err := absDir(*dir)
	if err != nil {
		fatal(err)
	}
	srv, err := startFM()
	if err != nil {
		fatal(err)
	}
	defer srv.Close()

	in := bufio.NewReader(os.Stdin)
	tools := &toolbox{root: root, approve: func(summary string) bool {
		fmt.Printf("\n%s\n", summary)
		if *yes {
			return true
		}
		fmt.Print("apply? [y/N] ")
		line, _ := in.ReadString('\n')
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
	for {
		fmt.Printf("\n%s%s❯%s ", bold, cyan, reset)
		line, err := in.ReadString('\n')
		switch text := strings.TrimSpace(line); {
		case text == "exit" || text == "/exit":
			return
		case text == "/clear":
			ag.history = nil
			fmt.Printf("%s⏺ Conversation cleared%s\n", green, reset)
		case text == "/help":
			fmt.Println("  /clear  reset the conversation (do this often: the model's memory is tiny)\n  /help   show this help\n  exit    quit")
		case strings.HasPrefix(text, "/"):
			fmt.Printf("%s⏺ unknown command %s (try /help)%s\n", red, text, reset)
		case text != "":
			turn(ag, text)
		}
		if err != nil {
			return
		}
	}
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
