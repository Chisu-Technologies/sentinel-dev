package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// defaultWatchTimeout is longer than verify's: a run that writes code takes
// longer than one that only checks a pull request.
const defaultWatchTimeout = 2 * time.Hour

func runWatch(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	timeout := fs.Duration("timeout", defaultWatchTimeout, "how long to follow the run, such as 30m")
	raw := fs.Bool("raw", false, "print the log as the server gives it, one JSON event per line")
	if code := parse(fs, args, env, "watch [--timeout <duration>] [--raw] <run id>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel watch takes the run id as one argument, for example: sentinel watch run-7")
		return ExitUsage
	}
	if *timeout <= 0 {
		fmt.Fprintln(env.Stderr, "sentinel watch: --timeout must be longer than zero")
		return ExitUsage
	}
	id := fs.Arg(0)
	if !server.ValidRunID(id) {
		// The argument is not repeated: it may be something that was pasted by
		// mistake.
		fmt.Fprintln(env.Stderr, "sentinel watch: that is not a run id")
		return ExitUsage
	}
	client, code := newServerClient(env, "watch")
	if code >= 0 {
		return code
	}

	w := newWaiter(ctx, env, client, *timeout)
	defer w.cancel()
	logs := &logFormatter{clean: func(s string) string { return client.Clean(s, server.MaxDetail) }}
	show := func(text string) {
		if *raw {
			fmt.Fprint(env.Stdout, text)
		} else {
			fmt.Fprint(env.Stdout, logs.write(text))
		}
	}
	// However watching ends, the last line of the log is not held back.
	defer func() {
		if !*raw {
			fmt.Fprint(env.Stdout, logs.flush())
		}
	}()
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}
	stillRunning := func(why string) int {
		return fail("%s Run %s continues on the server.", why, id)
	}

	var lastStatus, lastLog string
	announced := false
	// observe runs after every read of the run, the first one included: the
	// status when it changed, then what the log has that was not shown yet.
	observe := func(run server.Run) error {
		if status := client.Clean(run.Status, 64); !announced || status != lastStatus {
			fmt.Fprintf(env.Stderr, "Run %s: %s\n", id, status)
			announced, lastStatus = true, status
		}
		text, found, err := client.Log(w.ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		text = client.CleanText(text)
		show(newLogText(lastLog, text))
		lastLog = text
		return nil
	}

	run, err := client.Run(w.ctx, id)
	if err != nil {
		var apiErr *server.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.Status == 404:
			return fail("Run %s was not found on this server.", id)
		case w.ctx.Err() != nil:
			return stillRunning(w.classify(err, false))
		}
		return fail("%s", w.classify(err, false))
	}
	if err := observe(run); err != nil {
		return stillRunning(w.classify(err, false))
	}
	final, why := w.until(id, run.Status, observe)
	if why != "" {
		return stillRunning(why)
	}
	if final == server.StatusCompleted {
		return ExitOK
	}
	return ExitFailed
}

// newLogText is what of cur was not shown yet. The server gives the most
// recent lines of the log, so each answer is a window that moves: either it
// grew, or its start slid forward and the end of the previous window is the
// start of this one. With no overlap at all the whole window is new.
func newLogText(prev, cur string) string {
	if rest, ok := strings.CutPrefix(cur, prev); ok {
		return rest
	}
	for k := min(len(prev), len(cur)); k > 0; k-- {
		if prev[len(prev)-k:] == cur[:k] {
			return cur[k:]
		}
	}
	return cur
}
