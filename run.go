package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// runLimited runs a command in its own process group and kills the whole
// group when ctx ends. Killing only the shell would leave a server or a hung
// test it started holding the output pipe open, and the agent would wait forever.
func runLimited(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1") // don't litter the repo with __pycache__
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}
