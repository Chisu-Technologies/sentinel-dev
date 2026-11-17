package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

func runResume(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	budget := fs.Float64("budget", 0, "the new limit per task in USD; the organization's when not given")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	noWait := fs.Bool("no-wait", false, "print the run id and stop, without waiting for the pull request")
	timeout := fs.Duration("timeout", 2*time.Hour, "how long to wait for the run to end, such as 90m")
	base := fs.String("base", "", "the branch the pull request targets, when it is not the default one")
	if code := parse(fs, args, env, "resume [--budget <usd>] [--json] [--no-wait] [--timeout <duration>] [--base <branch>] <run id>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel resume takes the run id as one argument, for example: sentinel resume run-7")
		return ExitUsage
	}
	id := fs.Arg(0)
	if !server.ValidRunID(id) {
		fmt.Fprintln(env.Stderr, "sentinel resume: that is not a run id")
		return ExitUsage
	}
	if *budget < 0 || *timeout <= 0 {
		fmt.Fprintln(env.Stderr, "sentinel resume: --budget cannot be negative and --timeout must be longer than zero")
		return ExitUsage
	}
	baseGiven := false
	fs.Visit(func(f *flag.Flag) { baseGiven = baseGiven || f.Name == "base" })
	if baseGiven && !validBase(*base) {
		fmt.Fprintln(env.Stderr, "sentinel resume: --base needs a branch name")
		return ExitUsage
	}
	client, code := newServerClient(env, "resume")
	if code >= 0 {
		return code
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	paused, err := client.Run(ctx, id)
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return fail("Run %s was not found on this server.", id)
		}
		return fail("%v", err)
	}
	if !server.IsTerminal(paused.Status) {
		return fail("Run %s has not ended, so there is nothing to resume yet. Follow it with: sentinel watch %s", id, id)
	}

	run, err := client.StartResume(ctx, server.ResumeRequest{Paused: paused, BudgetUSD: *budget, BaseRef: *base})
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "resume_run_not_paused" {
			return fail("Run %s did not pause at its budget, so there is nothing to resume. To do the task again: sentinel run", id)
		}
		return fail("%v", err)
	}
	return followRun(ctx, env, client, run, runResult{
		Task:       client.Clean(taskTitle(paused), server.MaxDetail),
		Repository: client.Clean(paused.Repo, server.MaxDetail),
	}, *noWait, *timeout, *asJSON)
}

// taskTitle is the title of the one task of a run, or its id when the server
// sent none.
func taskTitle(run server.Run) string {
	if len(run.Findings) == 1 {
		var finding struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(run.Findings[0], &finding) == nil && finding.Title != "" {
			return finding.Title
		}
	}
	return "the task of run " + run.ID
}
