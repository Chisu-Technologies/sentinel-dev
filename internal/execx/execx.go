// Package execx runs external programs behind an interface, so the commands that
// depend on git, gh and ssh can be tested without them.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Result is what a finished program left behind.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner finds and runs external programs.
type Runner interface {
	// LookPath reports where a program is, or an error when it is not installed.
	LookPath(name string) (string, error)
	// Run starts a program, feeds it stdin and waits for it. A program that
	// exits with a non-zero status is not an error: the status is in the
	// result. The error is non-nil only when the program could not be started
	// or the context ended first.
	Run(ctx context.Context, stdin string, name string, args ...string) (Result, error)
}

// OS is the Runner that uses the operating system.
type OS struct{}

// LookPath implements Runner.
func (OS) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Run implements Runner.
func (OS) Run(ctx context.Context, stdin string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// A child that ignores the kill, or a grandchild that keeps the pipes open,
	// must not hang the command forever.
	cmd.WaitDelay = 5 * time.Second

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}
