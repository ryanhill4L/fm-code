package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// tracer wraps a completer and appends every request and response to a JSONL
// file, so a session can be replayed and studied after the fact.
type tracer struct {
	next completer
	mu   sync.Mutex
	f    *os.File
}

func newTracer(next completer, path string) (*tracer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &tracer{next: next, f: f}, nil
}

type traceEntry struct {
	Time     time.Time `json:"time"`
	Schema   string    `json:"schema"`
	Messages []message `json:"messages"`
	Response string    `json:"response"`
	Prompt   int       `json:"prompt_tokens"`
	Reply    int       `json:"reply_tokens"`
	Error    string    `json:"error,omitempty"`
	Millis   int64     `json:"ms"`
}

func (t *tracer) complete(ctx context.Context, msgs []message, schema any) (completion, error) {
	start := time.Now()
	out, err := t.next.complete(ctx, msgs, schema)
	e := traceEntry{Time: start, Messages: msgs, Response: out.Text, Prompt: out.PromptTokens, Reply: out.CompletionTokens, Millis: time.Since(start).Milliseconds()}
	if m, ok := schema.(map[string]any); ok {
		e.Schema, _ = m["title"].(string)
	}
	if err != nil {
		e.Error = err.Error()
	}
	line, _ := json.Marshal(e)
	t.mu.Lock()
	t.f.Write(append(line, '\n'))
	t.mu.Unlock()
	return out, err
}
