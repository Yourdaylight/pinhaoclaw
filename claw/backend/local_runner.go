package backend

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// LocalRunner 在当前机器上执行命令，供 local 节点复用现有 runner 流程。
type LocalRunner struct {
	pathEntries []string
}

func NewLocalRunner(picoclawBin string) *LocalRunner {
	entries := []string{}
	if picoclawBin != "" {
		entries = append(entries, filepath.Dir(picoclawBin))
	}
	return &LocalRunner{pathEntries: entries}
}

func (r *LocalRunner) Run(ctx context.Context, cmd string) (*SSHPayload, error) {
	command := exec.CommandContext(ctx, "bash", "-lc", cmd)
	command.Env = r.commandEnv()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	result := &SSHPayload{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: 0,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, fmt.Errorf("local command failed: %s (exit=%d)", result.Stderr, result.ExitCode)
	}
	return result, nil

}

func (r *LocalRunner) StreamRun(ctx context.Context, cmd string) (<-chan string, <-chan error) {
	outCh := make(chan string, 64)
	errCh := make(chan error, 1)

	command := exec.CommandContext(ctx, "bash", "-lc", cmd)
	command.Env = r.commandEnv()

	stdout, err := command.StdoutPipe()
	if err != nil {
		errCh <- err
		close(outCh)
		close(errCh)
		return outCh, errCh
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		errCh <- err
		close(outCh)
		close(errCh)
		return outCh, errCh
	}

	if err := command.Start(); err != nil {
		errCh <- err
		close(outCh)
		close(errCh)
		return outCh, errCh
	}

	go func() {
		defer close(outCh)
		defer close(errCh)

		var wg sync.WaitGroup
		forward := func(reader io.Reader) {
			defer wg.Done()
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				select {
				case outCh <- scanner.Text():
				case <-ctx.Done():
					return
				}
			}
		}

		wg.Add(2)
		go forward(stdout)
		go forward(stderr)
		wg.Wait()

		if err := command.Wait(); err != nil {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}
			errCh <- err
		}
	}()

	return outCh, errCh
}

func (r *LocalRunner) commandEnv() []string {
	pathValue := os.Getenv("PATH")
	for i := len(r.pathEntries) - 1; i >= 0; i-- {
		entry := strings.TrimSpace(r.pathEntries[i])
		if entry == "" {
			continue
		}
		pathValue = entry + string(os.PathListSeparator) + pathValue
	}
	env := os.Environ()
	env = append(env, "PATH="+pathValue)
	return env
}

var _ CommandRunner = (*LocalRunner)(nil)
