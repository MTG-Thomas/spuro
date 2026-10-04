// Package process runs bounded argv-only subprocesses without a shell.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Stdout []byte
	Stderr string
	Code   int
	Err    error
}
type limitedBuffer struct {
	bytes.Buffer
	Limit    int
	Overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.Limit - b.Len()
	if remaining < len(p) {
		b.Overflow = true
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
		return n, nil
	}
	return b.Buffer.Write(p)
}
func Environment() []string {
	env := []string{}
	for _, s := range os.Environ() {
		k, _, _ := strings.Cut(s, "=")
		if !strings.HasPrefix(k, "GIT_") && k != "LC_ALL" && k != "LANG" {
			env = append(env, s)
		}
	}
	return append(env, "LC_ALL=C", "LANG=C", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_COUNT=6", "GIT_CONFIG_KEY_0=core.fsmonitor", "GIT_CONFIG_VALUE_0=false", "GIT_CONFIG_KEY_1=core.untrackedCache", "GIT_CONFIG_VALUE_1=false", "GIT_CONFIG_KEY_2=gc.auto", "GIT_CONFIG_VALUE_2=0", "GIT_CONFIG_KEY_3=maintenance.auto", "GIT_CONFIG_VALUE_3=false", "GIT_CONFIG_KEY_4=diff.external", "GIT_CONFIG_VALUE_4=", "GIT_CONFIG_KEY_5=core.pager", "GIT_CONFIG_VALUE_5=cat")
}
func Run(ctx context.Context, timeout time.Duration, limit int, executable string, args []string, input io.Reader) Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = Environment()
	cmd.Stdin = input
	cmd.WaitDelay = time.Second
	isolate(cmd)
	out := &limitedBuffer{Limit: limit}
	errout := &limitedBuffer{Limit: 64 * 1024}
	cmd.Stdout = out
	cmd.Stderr = errout
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		var e *exec.ExitError
		if errors.As(err, &e) {
			code = e.ExitCode()
		}
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("command timeout or cancellation: %w", ctx.Err())
		code = -1
	}
	if out.Overflow {
		err = fmt.Errorf("command output exceeds %d bytes", limit)
		code = -1
	}
	return Result{Stdout: out.Bytes(), Stderr: errout.String(), Code: code, Err: err}
}

// Pipe streams potentially large patches directly into native patch-id. No patch
// contents are retained in scan JSON or logs.
func Pipe(ctx context.Context, timeout time.Duration, limit int, exe string, left, right []string, input io.Reader) Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	a := exec.CommandContext(ctx, exe, left...)
	b := exec.CommandContext(ctx, exe, right...)
	a.Env = Environment()
	b.Env = Environment()
	a.Stdin = input
	a.WaitDelay = time.Second
	b.WaitDelay = time.Second
	isolate(a)
	isolate(b)
	pipe, err := a.StdoutPipe()
	if err != nil {
		return Result{Err: err, Code: -1}
	}
	b.Stdin = pipe
	out := &limitedBuffer{Limit: limit}
	errA := &limitedBuffer{Limit: 64 * 1024}
	errB := &limitedBuffer{Limit: 64 * 1024}
	a.Stderr = errA
	b.Stderr = errB
	b.Stdout = out
	if err = b.Start(); err != nil {
		_ = pipe.Close()
		return Result{Err: err, Code: -1}
	}
	if err = a.Start(); err != nil {
		_ = pipe.Close()
		_ = b.Process.Kill()
		_ = b.Wait()
		return Result{Err: err, Code: -1}
	}
	be := b.Wait()
	ae := a.Wait()
	_ = pipe.Close()
	code := 0
	if ae != nil || be != nil {
		code = -1
		err = fmt.Errorf("patch pipeline failed: source=%v patch-id=%v", ae, be)
	}
	if ctx.Err() != nil {
		code = -1
		err = ctx.Err()
	}
	if out.Overflow {
		code = -1
		err = fmt.Errorf("patch-id output exceeds limit")
	}
	return Result{Stdout: out.Bytes(), Stderr: errA.String() + errB.String(), Code: code, Err: err}
}
