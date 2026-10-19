//go:build unix

package toolrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCancellationKillsStartedProcessGroup(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & echo $! > child.pid; wait")
	cmd.Dir = dir
	controlProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopAndWait := func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(3 * time.Second):
			return errors.New("fixture parent did not exit after cancellation")
		}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	var child int
	for child == 0 {
		select {
		case <-ticker.C:
			raw, err := os.ReadFile(filepath.Join(dir, "child.pid"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				t.Fatal(errors.Join(err, stopAndWait()))
			}
			if !strings.HasSuffix(string(raw), "\n") {
				continue // Wait for the fixture's complete PID line.
			}
			child, err = strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(errors.Join(err, stopAndWait()))
			}
			if child <= 0 {
				t.Fatal(errors.Join(fmt.Errorf("invalid child PID %d", child), stopAndWait()))
			}
		case <-deadline.C:
			t.Fatal(errors.Join(errors.New("child process did not start before fixture deadline"), stopAndWait()))
		}
	}
	// Cancellation is triggered after observing the running child's identity,
	// so this cannot pass merely by timing out before the shell was started.
	if err := syscall.Kill(child, 0); err != nil {
		t.Fatal(errors.Join(err, stopAndWait()))
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled shell succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parent process did not exit")
	}
	exitDeadline := time.NewTimer(3 * time.Second)
	defer exitDeadline.Stop()
	for {
		err := syscall.Kill(child, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-exitDeadline.C:
			t.Fatal("child process survived cancellation")
		}
	}
}
