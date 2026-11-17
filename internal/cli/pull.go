package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

func runPull(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	if code := parse(fs, args, env, "pull <run id>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel pull takes the run id as one argument, for example: sentinel pull run-7")
		return ExitUsage
	}
	id := fs.Arg(0)
	if !server.ValidRunID(id) {
		fmt.Fprintln(env.Stderr, "sentinel pull: that is not a run id")
		return ExitUsage
	}
	client, code := newServerClient(env, "pull")
	if code >= 0 {
		return code
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	run, err := client.Run(ctx, id)
	if err != nil {
		var apiErr *server.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return fail("Run %s was not found on this server.", id)
		}
		return fail("%v", err)
	}
	url := ""
	if run.PRURL != nil {
		url = *run.PRURL
	} else if stories, err := client.Stories(ctx, id); err == nil {
		for _, story := range stories {
			if story.PRURL != nil {
				url = *story.PRURL
				break
			}
		}
	}
	if url == "" {
		if server.IsTerminal(run.Status) {
			return fail("Run %s ended without a pull request: %s.", id, run.Status)
		}
		return fail("Run %s has no pull request yet. Follow it with: sentinel watch %s", id, id)
	}
	// The address is the word of the server: it is parsed like one a user typed,
	// so only a pull request on github.com is fetched.
	pr, err := parsePullRequest(url)
	if err != nil || pr.Repository == "" {
		return fail("Run %s has a pull request this command cannot fetch: %s", id, client.Clean(url, server.MaxDetail))
	}

	got, err := checkout.Pull(ctx, env.Runner, env.Dir, pr.Repository, pr.Number, "sentinel/"+id)
	if err != nil {
		if errors.Is(err, checkout.ErrUncommitted) {
			return fail("%v. Commit or stash them first: sentinel pull switches branches and never touches your changes.", err)
		}
		return fail("%v", err)
	}
	verb := "Updated"
	if got.Created {
		verb = "Created"
	}
	fmt.Fprintf(env.Stdout, "%s branch %s at %s, the pull request %s, and switched to it.\n", verb, got.Branch, shortCommit(got.Commit), client.Clean(url, server.MaxDetail))
	return ExitOK
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
