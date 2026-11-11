// Package execx runs external programs behind an interface, so the commands that
// depend on git, gh and ssh can be tested without them.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command describes one program to run.
type Command struct {
	Name string
	Args []string
	// Stdin is fed to the program.
	Stdin string
	// Dir is the working directory. Empty means the current one.
	Dir string
	// Env holds KEY=VALUE pairs added to the environment of this one program.
	Env []string
	// StdoutLimit, when positive, is the most standard output the caller
	// accepts, in bytes. A program that prints more is stopped, and Run
	// returns ErrOutputTooLarge.
	StdoutLimit int64
}

// Result is what a finished program left behind.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ErrOutputTooLarge means a program printed more than Command.StdoutLimit.
var ErrOutputTooLarge = errors.New("the program printed more output than accepted")

// Runner finds and runs external programs.
type Runner interface {
	// LookPath reports where a program is, or an error when it is not installed.
	LookPath(name string) (string, error)
	// Run starts a program and waits for it. A program that exits with a
	// non-zero status is not an error: the status is in the result. The error
	// is non-nil only when the program could not be started, the context
	// ended first, or the program went over StdoutLimit.
	Run(ctx context.Context, cmd Command) (Result, error)
}

// OS is the Runner that uses the operating system.
type OS struct{}

// LookPath implements Runner.
func (OS) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Run implements Runner.
func (OS) Run(ctx context.Context, command Command) (Result, error) {
	// Going over the output limit must stop the program: left alone, it would
	// block forever writing to a pipe nobody reads.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	cmd := exec.CommandContext(runCtx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	if len(command.Env) > 0 {
		cmd.Env = append(os.Environ(), command.Env...)
	}
	cmd.Stdin = strings.NewReader(command.Stdin)
	stdout := &capped{limit: command.StdoutLimit, stop: stop}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	// A child that ignores the kill, or a grandchild that keeps the pipes open,
	// must not hang the command forever.
	cmd.WaitDelay = 5 * time.Second

	err := cmd.Run()
	res := Result{Stdout: stdout.buf.String(), Stderr: stderr.String()}
	if stdout.exceeded {
		return Result{Stderr: res.Stderr}, ErrOutputTooLarge
	}
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

// capped collects output up to a limit. Zero means no limit.
type capped struct {
	buf      bytes.Buffer
	limit    int64
	exceeded bool
	stop     context.CancelFunc
}

func (c *capped) Write(p []byte) (int, error) {
	if c.limit > 0 && int64(c.buf.Len())+int64(len(p)) > c.limit {
		c.exceeded = true
		c.stop()
		return 0, ErrOutputTooLarge
	}
	return c.buf.Write(p)
}
