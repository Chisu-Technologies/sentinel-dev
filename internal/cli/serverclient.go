package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// newServerClient reads the two environment variables every command that
// talks to a server needs, and builds the client. It returns the exit status
// to stop with, or -1 to continue. The key comes only from the environment: a
// flag would end up in the shell history.
func newServerClient(env Env, command string) (*server.Client, int) {
	rawURL := env.getenv("SENTINEL_URL")
	if rawURL == "" {
		fmt.Fprintf(env.Stderr, "sentinel %s needs SENTINEL_URL, the address of your Sentinel server.\n", command)
		return nil, ExitFailed
	}
	key := env.getenv("SENTINEL_API_KEY")
	if key == "" {
		fmt.Fprintf(env.Stderr, "sentinel %s needs SENTINEL_API_KEY, an API key of your Sentinel server. It is read only from the environment.\n", command)
		return nil, ExitFailed
	}
	httpClient := env.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client, err := server.New(rawURL, key, httpClient, "sentinel/"+env.Build.Version)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel %s: SENTINEL_URL or SENTINEL_API_KEY: %v\n", command, err)
		return nil, ExitFailed
	}
	return client, -1
}

// waiter waits for a run to reach a terminal status, within one overall
// deadline. It is shared by the commands that follow a run.
type waiter struct {
	// ctx ends at the deadline or when the user interrupts; later requests of
	// the same command use it too, so the deadline covers all of them.
	ctx     context.Context
	cancel  context.CancelFunc
	parent  context.Context
	timeout time.Duration
	sleep   func(context.Context, time.Duration) error
	client  *server.Client
}

func newWaiter(ctx context.Context, env Env, client *server.Client, timeout time.Duration) *waiter {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	sleep := env.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &waiter{ctx: waitCtx, cancel: cancel, parent: ctx, timeout: timeout, sleep: sleep, client: client}
}

// classify tells the three ways waiting stops apart. A deadline error that
// did not come from the overall wait (the 30 second limit of one request)
// is an ordinary failure.
func (w *waiter) classify(err error, fromSleep bool) string {
	switch {
	case w.parent.Err() != nil:
		return "Interrupted."
	case w.ctx.Err() != nil || (fromSleep && errors.Is(err, context.DeadlineExceeded)):
		return "Gave up waiting after " + w.timeout.String() + "."
	}
	return w.client.Clean(err.Error(), server.MaxDetail) + "."
}

// until polls the run every pollInterval while status is not terminal.
// onPoll sees each answer and may stop the wait with an error. It returns the
// last status, and, when waiting stopped for another reason than a terminal
// status, a sentence that says why.
func (w *waiter) until(id, status string, onPoll func(server.Run) error) (last, reason string) {
	for !server.IsTerminal(status) {
		if err := w.sleep(w.ctx, pollInterval); err != nil {
			return status, w.classify(err, true)
		}
		next, err := w.client.Run(w.ctx, id)
		if err != nil {
			return status, w.classify(err, false)
		}
		status = next.Status
		if onPoll != nil {
			if err := onPoll(next); err != nil {
				return status, w.classify(err, false)
			}
		}
	}
	return status, ""
}
