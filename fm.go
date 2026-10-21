package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// maxReplyTokens bounds one model turn. Without it a degenerate reply can loop
// for minutes; this still leaves room to write a whole small file.
const maxReplyTokens = 1500

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// fmServer runs `fm serve` on a private unix socket and talks Chat Completions to it.
type fmServer struct {
	cmd    *exec.Cmd
	dir    string
	client *http.Client
}

// checkLicense refuses to run until the user has agreed to fm's terms themselves.
func checkLicense(bin string) error {
	out, err := exec.Command(bin, "license", "--status").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Agreed") {
		return fmt.Errorf("you haven't agreed to the Apple Foundation Models CLI license yet.\nRun `fm license` in your terminal, read the terms, and agree, then try again")
	}
	return nil
}

func startFM() (*fmServer, error) {
	bin, err := exec.LookPath("fm")
	if err != nil {
		return nil, fmt.Errorf("`fm` not found on PATH (needs macOS with Apple Foundation Models)")
	}
	if err := checkLicense(bin); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "fm-code-")
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(dir, "fm.sock")
	cmd := exec.Command(bin, "serve", "--socket", sock)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	s := &fmServer{cmd: cmd, dir: dir, client: &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		}},
	}}
	for i := 0; i < 100; i++ {
		if resp, err := s.client.Get("http://fm/health"); err == nil {
			resp.Body.Close()
			return s, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.Close()
	return nil, fmt.Errorf("timed out waiting for `fm serve`")
}

func (s *fmServer) Close() {
	s.cmd.Process.Kill()
	s.cmd.Wait()
	os.RemoveAll(s.dir)
}

// complete asks the model for a reply constrained to the given JSON schema.
func (s *fmServer) complete(ctx context.Context, msgs []message, schema any) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      "system",
		"stream":     false,
		"messages":   msgs,
		"max_tokens": maxReplyTokens,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "Action", "schema": schema},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fm/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Choices []struct {
			Message message `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("bad response from fm: %s", raw)
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("fm: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("fm returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
