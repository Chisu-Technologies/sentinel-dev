package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

const (
	// pollInterval is how often a running run is asked for its status. It is
	// always waited through Env.Sleep.
	pollInterval = 5 * time.Second
	// maxPullNumber keeps the number inside what a JSON number carries safely
	// and what any server stores.
	maxPullNumber = 1<<31 - 1
	maxBaseLen    = 255
)

// pullRequestURL is the address of a pull request as GitHub shows it. A path
// after the number (/files, /commits) and a query or fragment are accepted, as
// they come from the browser.
var pullRequestURL = regexp.MustCompile(`^/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)/pull/([0-9]+)(?:/.*)?$`)

// pullRequest is the pull request the user named. Repository is empty when the
// user gave only a number: it then comes from the checkout.
type pullRequest struct {
	Repository string
	Number     int
}

func parsePullRequest(arg string) (pullRequest, error) {
	arg = strings.TrimSpace(arg)
	if !strings.Contains(arg, "/") {
		n, err := parsePullNumber(arg)
		if err != nil {
			return pullRequest{}, errors.New("the pull request is a number such as 12, or a URL such as https://github.com/owner/name/pull/12")
		}
		return pullRequest{Number: n}, nil
	}
	u, err := url.Parse(arg)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") || u.Port() != "" || u.User != nil {
		return pullRequest{}, errors.New("a pull request URL looks like https://github.com/owner/name/pull/12; only github.com is supported")
	}
	m := pullRequestURL.FindStringSubmatch(u.EscapedPath())
	if m == nil || m[1] == "." || m[1] == ".." || m[2] == "." || m[2] == ".." {
		return pullRequest{}, errors.New("a pull request URL looks like https://github.com/owner/name/pull/12")
	}
	n, err := parsePullNumber(m[3])
	if err != nil {
		return pullRequest{}, errors.New("the pull request number in the URL is not valid")
	}
	return pullRequest{Repository: m[1] + "/" + m[2], Number: n}, nil
}

func parsePullNumber(s string) (int, error) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maxPullNumber {
		return 0, errors.New("not a pull request number")
	}
	return n, nil
}

// verifyResult is what `sentinel verify --json` prints. Fields after run_id
// and status are absent until the run has an outcome.
type verifyResult struct {
	Repository     string   `json:"repository"`
	PullRequest    int      `json:"pull_request"`
	PullRequestURL string   `json:"pull_request_url"`
	RunID          string   `json:"run_id"`
	Status         string   `json:"status"`
	Verdict        string   `json:"verdict,omitempty"`
	CostUSD        *float64 `json:"cost_usd,omitempty"`
	TestsPassed    *int     `json:"tests_passed,omitempty"`
	TestsTotal     *int     `json:"tests_total,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type verifyDryRun struct {
	Repository  string `json:"repository"`
	PullRequest int    `json:"pull_request"`
	Base        string `json:"base,omitempty"`
	ServerHost  string `json:"server_host,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

func runVerify(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "show what would be sent, and send nothing")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	noWait := fs.Bool("no-wait", false, "print the run id and stop, without waiting for the verdict")
	timeout := fs.Duration("timeout", 45*time.Minute, "how long to wait for the verdict, such as 30m")
	base := fs.String("base", "", "the branch the pull request targets, when it is not the default one")
	if code := parse(fs, args, env, "verify [--dry-run] [--json] [--no-wait] [--timeout <duration>] [--base <branch>] <pull request>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel verify takes the pull request as one argument, for example: sentinel verify 12")
		return ExitUsage
	}
	if *timeout <= 0 {
		fmt.Fprintln(env.Stderr, "sentinel verify: --timeout must be longer than zero")
		return ExitUsage
	}
	baseGiven := false
	fs.Visit(func(f *flag.Flag) { baseGiven = baseGiven || f.Name == "base" })
	if baseGiven && !validBase(*base) {
		fmt.Fprintln(env.Stderr, "sentinel verify: --base needs a branch name")
		return ExitUsage
	}
	pr, err := parsePullRequest(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel verify: %v\n", err)
		return ExitUsage
	}

	if pr.Repository == "" {
		pr.Repository, err = checkout.Repository(ctx, env.Runner, env.Dir)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
			return ExitFailed
		}
	}

	rawURL := env.getenv("SENTINEL_URL")
	if *dry {
		return verifyDryRunReport(env, pr, *base, rawURL, *asJSON)
	}
	if rawURL == "" {
		fmt.Fprintln(env.Stderr, "sentinel verify needs SENTINEL_URL, the address of your Sentinel server.")
		return ExitFailed
	}
	key := env.getenv("SENTINEL_API_KEY")
	if key == "" {
		fmt.Fprintln(env.Stderr, "sentinel verify needs SENTINEL_API_KEY, an API key of your Sentinel server. It is read only from the environment.")
		return ExitFailed
	}
	httpClient := env.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client, err := server.New(rawURL, key, httpClient, "sentinel/"+env.Build.Version)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel verify: SENTINEL_URL or SENTINEL_API_KEY: %v\n", err)
		return ExitFailed
	}
	return verify(ctx, env, client, pr, *base, *noWait, *timeout, *asJSON)
}

func validBase(s string) bool {
	return s != "" && len(s) <= maxBaseLen && strings.IndexFunc(s, unicode.IsControl) < 0 && strings.TrimSpace(s) == s
}

func verifyDryRunReport(env Env, pr pullRequest, base, rawURL string, asJSON bool) int {
	host := ""
	if rawURL != "" {
		_, h, err := server.ParseURL(rawURL)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel verify: SENTINEL_URL: %v\n", err)
			return ExitFailed
		}
		host = h
	}
	if asJSON {
		return writeJSON(env, verifyDryRun{Repository: pr.Repository, PullRequest: pr.Number, Base: base, ServerHost: host, DryRun: true})
	}
	table(env, "Repository", pr.Repository)
	table(env, "Pull request", fmt.Sprintf("#%d", pr.Number))
	if base != "" {
		table(env, "Base", base)
	}
	if host == "" {
		table(env, "Server", "SENTINEL_URL is not set")
	} else {
		table(env, "Server", host)
	}
	fmt.Fprintln(env.Stdout, "\nNothing was sent: this was a dry run.")
	return ExitOK
}

// table prints one line of the aligned `Label   value` style.
func table(env Env, label, value string) {
	fmt.Fprintf(env.Stdout, "%-14s%s\n", label, value)
}

func verify(ctx context.Context, env Env, client *server.Client, pr pullRequest, base string, noWait bool, timeout time.Duration, asJSON bool) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	run, err := client.StartVerify(ctx, server.StartRequest{Repository: pr.Repository, PullNumber: pr.Number, BaseRef: base})
	if err != nil {
		return fail("%v", err)
	}
	status := client.Clean(run.Status, 64)
	fmt.Fprintf(env.Stderr, "Run %s created (%s).\n", run.ID, status)

	result := verifyResult{
		Repository:     pr.Repository,
		PullRequest:    pr.Number,
		PullRequestURL: fmt.Sprintf("https://github.com/%s/pull/%d", pr.Repository, pr.Number),
		RunID:          run.ID,
		Status:         status,
	}
	if noWait {
		return printResult(env, result, server.Story{}, asJSON, ExitOK)
	}

	// From here a failure leaves a run behind on the server; every message says
	// so, with the id, so the user can look at it there.
	stillRunning := func(why string) int {
		return fail("%s Run %s continues on the server.", why, run.ID)
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sleep := env.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	// classify tells the three ways waiting stops apart. A deadline error that
	// did not come from the overall wait (the 30 second limit of one request)
	// is an ordinary failure.
	classify := func(err error, fromSleep bool) string {
		switch {
		case ctx.Err() != nil:
			return "Interrupted."
		case waitCtx.Err() != nil || (fromSleep && errors.Is(err, context.DeadlineExceeded)):
			return "Gave up waiting after " + timeout.String() + "."
		}
		return client.Clean(err.Error(), server.MaxDetail) + "."
	}

	for !server.IsTerminal(run.Status) {
		if err := sleep(waitCtx, pollInterval); err != nil {
			return stillRunning(classify(err, true))
		}
		next, err := client.Run(waitCtx, run.ID)
		if err != nil {
			return stillRunning(classify(err, false))
		}
		if next.Status != run.Status {
			fmt.Fprintf(env.Stderr, "Run %s: %s\n", run.ID, client.Clean(next.Status, 64))
		}
		run.Status = next.Status
	}
	result.Status = client.Clean(run.Status, 64)

	stories, err := client.Stories(waitCtx, run.ID)
	endedWithoutVerdict := run.Status != server.StatusCompleted && run.Status != server.StatusBlocked
	if err != nil && !endedWithoutVerdict {
		return stillRunning(classify(err, false))
	}
	var story server.Story
	if err == nil && len(stories) > 0 {
		story = stories[0]
	}
	verdict := story.Verdict()
	if endedWithoutVerdict && verdict == "APPROVED" {
		// A run that failed, was stopped or was abandoned verified nothing,
		// whatever its story says.
		verdict = "UNKNOWN"
	}
	result.Verdict = client.Clean(verdict, 64)
	cost := story.CostUSD
	result.CostUSD = &cost
	if story.PRURL != nil && strings.HasPrefix(*story.PRURL, "https://github.com/") {
		result.PullRequestURL = client.Clean(*story.PRURL, server.MaxDetail)
	}
	if ev := story.Evidence; ev != nil {
		if ev.TestsTotal != nil && *ev.TestsTotal > 0 {
			total := int(*ev.TestsTotal)
			result.TestsTotal = &total
			if ev.TestsPassed != nil {
				passed := int(*ev.TestsPassed)
				result.TestsPassed = &passed
			}
		}
	}
	if result.Verdict != "APPROVED" && story.BlockedDetail != nil {
		result.Reason = client.Clean(*story.BlockedDetail, server.MaxDetail)
	}

	code := ExitFailed
	if result.Verdict == "APPROVED" {
		code = ExitOK
	}
	return printResult(env, result, story, asJSON, code)
}

func printResult(env Env, result verifyResult, story server.Story, asJSON bool, code int) int {
	if asJSON {
		if out := writeJSON(env, result); out != ExitOK {
			return out
		}
		return code
	}
	table(env, "Repository", result.Repository)
	table(env, "Pull request", fmt.Sprintf("#%d %s", result.PullRequest, result.PullRequestURL))
	table(env, "Run", fmt.Sprintf("%s (%s)", result.RunID, result.Status))
	if result.Verdict != "" {
		table(env, "Verdict", result.Verdict)
	}
	if result.TestsTotal != nil {
		passed := 0
		if result.TestsPassed != nil {
			passed = *result.TestsPassed
		}
		table(env, "Tests", fmt.Sprintf("%d of %d passed", passed, *result.TestsTotal))
	}
	if story.DiffFiles > 0 {
		table(env, "Changes", fmt.Sprintf("%s, +%d -%d", count(int(story.DiffFiles), "file"), int(story.DiffAdded), int(story.DiffRemoved)))
	}
	if result.CostUSD != nil {
		table(env, "Cost", fmt.Sprintf("$%.2f", *result.CostUSD))
	}
	if result.Reason != "" {
		table(env, "Reason", result.Reason)
	}
	return code
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
