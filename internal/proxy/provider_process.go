package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pulumi/providertest/providers"
)

const stderrCaptureLimit = 4096

type providerProcess struct {
	name string
	cmd  *exec.Cmd
}

type providerProcessSet struct {
	mu        sync.Mutex
	processes []providerProcess
}

func (p *providerProcessSet) add(proc providerProcess) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if proc.cmd == nil {
		return
	}
	p.processes = append(p.processes, proc)
}

func (p *providerProcessSet) wait(ctx context.Context, logger *log.Logger) {
	p.mu.Lock()
	processes := make([]providerProcess, len(p.processes))
	copy(processes, p.processes)
	p.mu.Unlock()

	for _, proc := range processes {
		done := make(chan struct{})
		go func(pr providerProcess) {
			_ = pr.cmd.Wait()
			close(done)
		}(proc)

		select {
		case <-done:
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			logger.Printf("provider %s did not exit within timeout", proc.name)
		}
	}
}

// newProviderFactory starts a provider binary with stdio detached from the parent process and tracks it for cleanup.
func newProviderFactory(name, version string, processes *providerProcessSet) providers.ProviderFactory {
	return func(ctx context.Context, pt providers.PulumiTest) (providers.Port, error) {
		binaryPath, err := providers.DownloadPluginBinary(name, version)
		if err != nil {
			return 0, err
		}

		port, cmd, err := startProviderProcess(ctx, binaryPath, name, pt.Source())
		if err != nil {
			return 0, err
		}

		processes.add(providerProcess{name: name, cmd: cmd})
		return port, nil
	}
}

func startProviderProcess(ctx context.Context, binaryPath, name, cwd string) (providers.Port, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, nil, err
	}

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	stderrBuf := cappedBuffer{cap: stderrCaptureLimit}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(io.MultiWriter(&stderrBuf, io.Discard), stderr)
	}()

	reader := bufio.NewReader(stdout)
	port, err := readProviderPort(reader)
	if err != nil {
		_ = cmd.Process.Kill()
		// Ensure the killed provider process is reaped to avoid zombies on startup failure.
		go cmd.Wait()
		select {
		case <-stderrDone:
		case <-time.After(100 * time.Millisecond):
		}
		if stderrText := strings.TrimSpace(stderrBuf.String()); stderrText != "" {
			err = fmt.Errorf("%w; stderr: %s", err, stderrText)
		}
		return 0, nil, fmt.Errorf("failed to read port number from provider %s: %w", name, err)
	}

	// Drain any remaining stdout output so the provider can't block on a full pipe.
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		if closer, ok := stdout.(io.Closer); ok {
			_ = closer.Close()
		}
	}()

	return providers.Port(port), cmd, nil
}

func readProviderPort(reader *bufio.Reader) (int, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("failed to read provider port: %w", err)
	}
	line = strings.TrimSpace(line)

	port, err := strconv.Atoi(line)
	if err != nil {
		return 0, fmt.Errorf("failed to parse port number from provider: %w", err)
	}
	return port, nil
}

type cappedBuffer struct {
	buf bytes.Buffer
	cap int64
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	remaining := c.cap - int64(c.buf.Len())
	if remaining > 0 {
		if int64(len(p)) <= remaining {
			_, _ = c.buf.Write(p)
		} else {
			_, _ = c.buf.Write(p[:remaining])
		}
	}
	// Always report full write to avoid blocking the writer; excess bytes are discarded.
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	return c.buf.String()
}
