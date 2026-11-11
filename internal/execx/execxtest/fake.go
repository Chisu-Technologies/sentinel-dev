// Package execxtest provides a Runner for tests: no program is started.
package execxtest

import (
	"context"
	"os/exec"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// Call records one program a test subject tried to run.
type Call struct {
	Stdin string
	Name  string
	Args  []string
}

// Fake is a scripted execx.Runner.
type Fake struct {
	// Missing lists the programs LookPath does not find.
	Missing map[string]bool
	// Results holds the outcome of a run, keyed by the full command line
	// ("gh auth status") or, failing that, by the program name ("gh").
	Results map[string]execx.Result
	// Errors makes a program fail to start, keyed like Results.
	Errors map[string]error
	// Calls is every Run in order.
	Calls []Call
}

// LookPath implements execx.Runner.
func (f *Fake) LookPath(name string) (string, error) {
	if f.Missing[name] {
		return "", exec.ErrNotFound
	}
	return "/usr/bin/" + name, nil
}

// Run implements execx.Runner.
func (f *Fake) Run(_ context.Context, stdin string, name string, args ...string) (execx.Result, error) {
	f.Calls = append(f.Calls, Call{Stdin: stdin, Name: name, Args: args})
	line := strings.Join(append([]string{name}, args...), " ")
	if err, ok := f.Errors[line]; ok {
		return execx.Result{}, err
	}
	if err, ok := f.Errors[name]; ok {
		return execx.Result{}, err
	}
	if res, ok := f.Results[line]; ok {
		return res, nil
	}
	return f.Results[name], nil
}
