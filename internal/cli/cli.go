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
	"io/fs"
	"net/http"
	"strings"
	"time"

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
	// Stdin is what the command reads, for `up --github-token-stdin`. Nil
	// means empty.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Runner execx.Runner
	Build  buildinfo.Info
	// Dir is the directory the command was started in. Empty means the
	// current directory of the process.
	Dir string
	// Getenv reads an environment variable. Nil means every variable is
	// empty, so a test never depends on the machine it runs on.
	Getenv func(string) string
	// HTTP is the client for requests to a Sentinel server. Nil means one with
	// a 30 second timeout. Redirects are never followed, whatever it is.
	HTTP *http.Client
	// Sleep waits for d, or returns early with the error of ctx. Nil means a
	// real timer. It exists so a test does not wait for the clock.
	Sleep func(ctx context.Context, d time.Duration) error
	// Bundle is what `up` brings up. Nil means the bundle built into the command.
	Bundle fs.FS
	// Rand is the source of the secrets `up` creates. Nil means crypto/rand.
	Rand io.Reader
}

func (e Env) getenv(name string) string {
	if e.Getenv == nil {
		return ""
	}
	return strings.TrimSpace(e.Getenv(name))
}

const usage = `sentinel is the Sentinel command-line client.

This is a pre-release. Only the commands below exist.

Usage:
  sentinel <command> [flags]

Commands:
  cancel <run id>           Stop a run of a Sentinel server
  doctor [destination]      Check this machine, or a server before installing on it
  down                      Stop Sentinel on this machine; its data stays
  run "<task>"              Send a task and this working tree to a Sentinel server
  runs                      List the runs of a Sentinel server
  setup <destination>       Run Sentinel on your server, over ssh
  up                        Run Sentinel on this machine, with Docker
  verify <pull request>     Ask a Sentinel server to verify a pull request
  watch <run id>            Follow a run until it ends
  version                   Print the version
  help                      Show this help

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
	case "cancel":
		return runCancel(ctx, rest, env)
	case "doctor":
		return runDoctor(ctx, rest, env)
	case "down":
		return runDown(ctx, rest, env)
	case "up":
		return runUp(ctx, rest, env)
	case "run":
		return runRun(ctx, rest, env)
	case "runs":
		return runRuns(ctx, rest, env)
	case "setup":
		return runSetup(ctx, rest, env)
	case "verify":
		return runVerify(ctx, rest, env)
	case "watch":
		return runWatch(ctx, rest, env)
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

func writeJSON(env Env, value any) int {
	enc := json.NewEncoder(env.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	return ExitOK
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
		return writeJSON(env, env.Build)
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
		if code := writeJSON(env, report); code != ExitOK {
			return code
		}
	} else {
		report.WriteText(env.Stdout)
	}
	if report.Failed() {
		return ExitFailed
	}
	return ExitOK
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func size(bytes int) string {
	switch {
	case bytes < 1<<10:
		return fmt.Sprintf("%d bytes", bytes)
	case bytes < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
	}
}
