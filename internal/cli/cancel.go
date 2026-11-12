package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

func runCancel(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	if code := parse(fs, args, env, "cancel <run id>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel cancel takes the run id as one argument, for example: sentinel cancel run-7")
		return ExitUsage
	}
	id := fs.Arg(0)
	if !server.ValidRunID(id) {
		// The argument is not repeated: it may be something that was pasted by
		// mistake.
		fmt.Fprintln(env.Stderr, "sentinel cancel: that is not a run id")
		return ExitUsage
	}
	client, code := newServerClient(env, "cancel")
	if code >= 0 {
		return code
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	// The run is read before it is stopped. A run that already ended is left
	// alone, and a server that accepts the key here and refuses it for the stop
	// is one that stops runs only from its panel: without this read that
	// refusal would look like a wrong key.
	run, err := client.Run(ctx, id)
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return fail("Run %s was not found on this server.", id)
		}
		return fail("%v", err)
	}
	// A status is printed only when IsTerminal holds, which is true for the
	// statuses this client knows and for nothing else the server may send.
	if server.IsTerminal(run.Status) {
		fmt.Fprintf(env.Stdout, "Run %s had already ended: %s. Nothing was stopped.\n", id, run.Status)
		return ExitOK
	}

	run, err = client.Stop(ctx, id)
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			return fail("This server read run %s with the API key and refused the key for stopping it. It stops runs only from its panel; a newer Sentinel server accepts the key.", id)
		}
		return fail("%v", err)
	}
	switch {
	case run.Status == server.StatusStopped:
		fmt.Fprintf(env.Stdout, "Run %s: stopped\n", id)
	case server.IsTerminal(run.Status):
		fmt.Fprintf(env.Stdout, "Run %s ended before it was stopped: %s.\n", id, run.Status)
	default:
		// The server handed the stop to the machine of the run, which has not
		// applied it yet.
		fmt.Fprintf(env.Stdout, "Run %s: the server accepted the stop. The run ends when its machine applies it; follow it with: sentinel watch %s\n", id, id)
	}
	return ExitOK
}
