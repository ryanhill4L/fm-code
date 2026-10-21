package main

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const suggestPrompt = `You predict what the user of a coding assistant will type next.
Look at the conversation and the assistant's latest output. If something failed or looks unfinished, suggest the fix. If the task looks done, suggest a sensible follow-up such as running the tests.
Write exactly what the user would type: one short imperative sentence, no quotes, no explanation.`

const suggestInstruction = "What will the user most likely ask next? Answer with just that instruction."

const maxSuggestionRunes = 80

var suggestSchema = map[string]any{
	"title":                "Suggestion",
	"type":                 "object",
	"properties":           map[string]any{"next": map[string]any{"type": "string"}},
	"required":             []string{"next"},
	"x-order":              []string{"next"},
	"additionalProperties": false,
}

// suggestNext asks a fresh model request, forked from the conversation so far,
// for the user's likely next command. It never touches the agent's own history.
func suggestNext(ctx context.Context, model completer, history []message) (string, error) {
	fork := append(append([]message(nil), history...), message{"user", suggestInstruction})
	raw, err := model.complete(ctx, windowed(suggestPrompt, fork, historyBudget), suggestSchema)
	if err != nil {
		return "", err
	}
	var out struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", err
	}
	return cleanSuggestion(out.Next), nil
}

// cleanSuggestion reduces model output to something safe to show as ghost text,
// or "" if it isn't usable.
func cleanSuggestion(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	s = strings.Trim(strings.TrimSpace(s), "\"'`")
	if s == "" || strings.HasPrefix(s, "/") || strings.EqualFold(s, "exit") {
		return ""
	}
	if utf8.RuneCountInString(s) > maxSuggestionRunes {
		return string([]rune(s)[:maxSuggestionRunes])
	}
	return s
}

// startSuggestion forks the conversation and computes a suggestion in the background.
// The result arrives on the channel, which stays empty if the model fails.
// Call the returned cancel func when the suggestion is no longer wanted.
// The channel is never closed: edit selects on it, and a closed channel would spin.
func startSuggestion(model completer, history []message) (<-chan string, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	snapshot := append([]message(nil), history...)
	ch := make(chan string, 1)
	go func() {
		if s, err := suggestNext(ctx, model, snapshot); err == nil && s != "" {
			ch <- s
		}
	}()
	return ch, cancel
}
