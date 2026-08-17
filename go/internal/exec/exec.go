// Package exec is mudev's single gateway for running external processes.
//
// It is the ONLY package that imports os/exec. Every other package that needs
// to run a subprocess (git, workspace, …) builds a domain wrapper on top of
// this one — mirroring mpd's single-choke-point rule (VM/Exec.swift).
// Keeping process execution in one place makes command running uniformly
// testable, auditable, and consistent (working dir, env, exit-code handling).
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
)

// Cmd describes a single external command to run.
type Cmd struct {
	Name  string    // executable name or path; a bare name (no path separator) is resolved via PATH
	Args  []string  // arguments (excluding Name)
	Dir   string    // working directory; empty means the current directory
	Env   []string  // extra "KEY=VALUE" entries appended to os.Environ()
	Stdin io.Reader // optional standard input
}

// Result is the outcome of a captured command.
type Result struct {
	Code   int    // process exit code (0 on success; -1 if killed by signal)
	Stdout string // captured standard output, trailing newlines trimmed
	Stderr string // captured standard error, trailing newlines trimmed
}

// Failed reports whether the command exited non-zero.
func (r Result) Failed() bool {
	return r.Code != 0
}

// Err returns a non-nil error if the command exited non-zero, folding in any
// captured stderr. Callers that treat a non-zero exit as failure can write
// `if err := res.Err(); err != nil { … }`; callers that inspect specific exit
// codes (e.g. `git diff --quiet`) can read res.Code directly.
func (r Result) Err() error {
	if r.Code == 0 {
		return nil
	}
	if r.Stderr != "" {
		return fmt.Errorf("exit status %d: %s", r.Code, r.Stderr)
	}
	return fmt.Errorf("exit status %d", r.Code)
}

// Available reports whether name can be resolved on PATH.
func Available(name string) bool {
	_, err := osexec.LookPath(name)
	return err == nil
}

// Run executes cmd, streaming its stdout and stderr to this process's stdout
// and stderr. It returns the exit code. The returned error is non-nil only when
// the process could not be started (e.g. binary not found); a non-zero exit is
// reported through the returned code, not as an error.
func Run(ctx context.Context, cmd Cmd) (int, error) {
	c := build(ctx, cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return wait(c)
}

// Capture executes cmd and captures stdout and stderr separately. As with Run,
// a non-zero exit is not an error — it is reported via Result.Code (use
// Result.Err to convert). The error is non-nil only when the process could not
// be started.
func Capture(ctx context.Context, cmd Cmd) (Result, error) {
	c := build(ctx, cmd)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	code, err := wait(c)
	return Result{
		Code:   code,
		Stdout: strings.TrimRight(stdout.String(), "\n"),
		Stderr: strings.TrimRight(stderr.String(), "\n"),
	}, err
}

func build(ctx context.Context, cmd Cmd) *osexec.Cmd {
	c := osexec.CommandContext(ctx, cmd.Name, cmd.Args...)
	c.Dir = cmd.Dir
	if len(cmd.Env) > 0 {
		c.Env = append(os.Environ(), cmd.Env...)
	}
	c.Stdin = cmd.Stdin
	return c
}

// wait runs c and separates "process ran" from "process could not start".
func wait(c *osexec.Cmd) (int, error) {
	err := c.Run()
	if err == nil {
		return 0, nil
	}
	// The process ran and exited non-zero (or was signaled): a valid outcome.
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	// The process could not be started at all.
	return -1, fmt.Errorf("exec %s: %w", c.Path, err)
}
