// Package cli implements the sentinel command line. Everything it touches
// (output, external programs, build information) is passed in, so each command
// is tested without a terminal or a network.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/doctor"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// Exit statuses. A script can tell "a check failed" from "the command line
// was wrong".
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

// Env is what a command may touch.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Runner execx.Runner
	Build  buildinfo.Info
}

const usage = `sentinel is the Sentinel command-line client.

This is a pre-release. Only the commands below exist.

Usage:
  sentinel <command> [flags]

Commands:
  doctor [destination]   Check this machine, or a server before installing on it
  version                Print the version
  help                   Show this help

Run "sentinel <command> -h" for the flags of a command.
`

// Run executes one command line and returns the process exit status.
func Run(ctx context.Context, args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	}
	switch name, rest := args[0], args[1:]; name {
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	case "version", "--version":
		return runVersion(rest, env)
	case "doctor":
		return runDoctor(ctx, rest, env)
	default:
		fmt.Fprintf(env.Stderr, "sentinel: unknown command %q\n\n%s", name, usage)
		return ExitUsage
	}
}

// parse reads the flags of one command. It returns the exit status to stop
// with, or -1 to continue.
func parse(fs *flag.FlagSet, args []string, env Env, synopsis string) int {
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(env.Stderr, "Usage: sentinel %s\n", synopsis)
		fs.PrintDefaults()
	}
	switch err := fs.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		return ExitOK
	case err != nil:
		return ExitUsage
	}
	return -1
}

func runVersion(args []string, env Env) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the build information as JSON")
	if code := parse(fs, args, env, "version [--json]"); code >= 0 {
		return code
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(env.Stderr, "sentinel version takes no arguments")
		return ExitUsage
	}
	if *asJSON {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(env.Build); err != nil {
			fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
			return ExitFailed
		}
		return ExitOK
	}
	fmt.Fprintf(env.Stdout, "sentinel %s (%s, %s/%s)\n", env.Build.Version, env.Build.Go, env.Build.OS, env.Build.Arch)
	return ExitOK
}

func runDoctor(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if code := parse(fs, args, env, "doctor [--json] [destination]"); code >= 0 {
		return code
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(env.Stderr, "sentinel doctor takes at most one destination")
		return ExitUsage
	}

	var report doctor.Report
	if dest := fs.Arg(0); dest != "" {
		report = doctor.Host(ctx, env.Runner, dest)
	} else {
		report = doctor.Local(ctx, env.Runner)
	}

	if *asJSON {
		if err := report.WriteJSON(env.Stdout); err != nil {
			fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
			return ExitFailed
		}
	} else {
		report.WriteText(env.Stdout)
	}
	if report.Failed() {
		return ExitFailed
	}
	return ExitOK
}
