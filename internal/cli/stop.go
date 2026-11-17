package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

func runStop(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "stop every run without asking again")
	if code := parse(fs, args, env, "stop --yes"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel stop takes no arguments. To stop one run: sentinel cancel <run id>")
		return ExitUsage
	}
	// Only the owner, in the panel, lets runs start again, so a stop typed by
	// mistake is not undone from here. The flag is the confirmation.
	if !*yes {
		fmt.Fprintln(env.Stderr, "sentinel stop stops every run of your organization and refuses new ones until the owner allows them again in the panel.")
		fmt.Fprintln(env.Stderr, "To go ahead: sentinel stop --yes. To stop one run: sentinel cancel <run id>")
		return ExitUsage
	}
	client, code := newServerClient(env, "stop")
	if code >= 0 {
		return code
	}

	halt, err := client.StopAll(ctx)
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusMethodNotAllowed) {
			fmt.Fprintln(env.Stderr, "sentinel: This server cannot stop every run from the command line; stop them from its panel. A newer Sentinel server accepts it.")
			return ExitFailed
		}
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	if halt.Queued {
		fmt.Fprintln(env.Stdout, "Every run is stopping: the server handed the stop to the machine of the runs. No new run starts until the owner allows runs again in the panel.")
		return ExitOK
	}
	noun := "runs"
	if halt.Stopped == 1 {
		noun = "run"
	}
	fmt.Fprintf(env.Stdout, "Stopped %d %s. No new run starts until the owner allows runs again in the panel.\n", halt.Stopped, noun)
	return ExitOK
}
