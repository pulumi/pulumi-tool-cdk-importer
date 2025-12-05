package proxy

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCappedBufferRespectsCap(t *testing.T) {
	t.Parallel()

	cb := cappedBuffer{cap: 5}
	_, _ = cb.Write([]byte("hello"))
	_, _ = cb.Write([]byte("world"))

	if got := cb.String(); got != "hello" {
		t.Fatalf("expected capped buffer to keep first 5 bytes, got %q", got)
	}
}

func TestProviderProcessSetWaitCompletes(t *testing.T) {
	t.Parallel()

	cmd := helperCommand(t, "sleep", "20ms")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper: %v", err)
	}

	var processes providerProcessSet
	processes.add(providerProcess{name: "helper", cmd: cmd})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	processes.wait(ctx, discardLogger(t))
	if time.Since(start) > time.Second {
		t.Fatalf("wait exceeded context timeout")
	}
}

func TestProviderProcessSetWaitRespectsContext(t *testing.T) {
	t.Parallel()

	cmd := helperCommand(t, "sleep", "2s")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper: %v", err)
	}
	defer cmd.Process.Kill() // ensure cleanup if it outlives the test

	var processes providerProcessSet
	processes.add(providerProcess{name: "helper", cmd: cmd})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	processes.wait(ctx, discardLogger(t))
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("wait did not respect context deadline")
	}
}

func TestStartProviderProcessNonExecutable(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "dummy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho noop\n"), 0o644); err != nil {
		t.Fatalf("write dummy: %v", err)
	}

	_, _, err := startProviderProcess(context.Background(), path, "aws", tmpDir)
	if err == nil {
		t.Fatalf("expected startProviderProcess to fail for non-executable path")
	}
}

func helperCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "HELPER_PROCESS=1")
	return cmd
}

func discardLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}

// TestHelperProcess is executed as a subprocess to simulate controllable command behavior.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	sep := 0
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep == 0 || sep+1 >= len(args) {
		os.Exit(1)
	}
	cmd := args[sep+1]
	switch cmd {
	case "sleep":
		dur := args[sep+2]
		if d, err := time.ParseDuration(dur); err == nil {
			time.Sleep(d)
		}
	case "exit":
		code := 0
		if len(args) > sep+2 {
			if parsed, err := strconv.Atoi(args[sep+2]); err == nil {
				code = parsed
			}
		}
		os.Exit(code)
	}
	os.Exit(0)
}
