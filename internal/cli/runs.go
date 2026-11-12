package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

const (
	defaultRunsLimit = 20
	maxRunsLimit     = 100
	// storyWidth keeps one run on one line of a normal terminal.
	storyWidth = 60
	// minuteWidth is the length of "2006-01-02T15:04": a timestamp cut to
	// minutes, whatever follows the minutes.
	minuteWidth = 16
)

// runSummary is one run in `sentinel runs --json`: only the fields the client
// has a use for, so a field the server adds later does not leak into scripts.
type runSummary struct {
	ID           string  `json:"id"`
	Repo         string  `json:"repo"`
	Status       string  `json:"status"`
	Mode         string  `json:"mode,omitempty"`
	Story        *string `json:"story"`
	PRNumber     *string `json:"pr_number"`
	PRURL        *string `json:"pr_url"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	StartedAt    *string `json:"started_at"`
	FinishedAt   *string `json:"finished_at"`
}

func runRuns(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the runs as JSON")
	limit := fs.Int("limit", defaultRunsLimit, "how many runs to list, from 1 to 100")
	if code := parse(fs, args, env, "runs [--json] [--limit <n>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel runs takes no arguments")
		return ExitUsage
	}
	if *limit < 1 || *limit > maxRunsLimit {
		fmt.Fprintf(env.Stderr, "sentinel runs: --limit must be between 1 and %d\n", maxRunsLimit)
		return ExitUsage
	}
	client, code := newServerClient(env, "runs")
	if code >= 0 {
		return code
	}
	runs, err := client.Runs(ctx, *limit, 0)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}

	if *asJSON {
		// Never nil: an empty list is [], not null.
		out := make([]runSummary, 0, len(runs))
		for _, r := range runs {
			out = append(out, summarize(client, r))
		}
		return writeJSON(env, out)
	}
	if len(runs) == 0 {
		fmt.Fprintln(env.Stdout, "No runs yet.")
		return ExitOK
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tREPOSITORY\tPR\tCOST\tSTARTED\tSTORY")
	for _, r := range runs {
		pr := ""
		if r.PRNumber != nil && *r.PRNumber != "" {
			pr = "#" + client.Clean(*r.PRNumber, 16)
		}
		started := ""
		if r.StartedAt != nil {
			started = cutRunes(client.Clean(*r.StartedAt, 64), minuteWidth)
		}
		story := ""
		if r.Story != nil {
			story = storyLine(client.Clean(*r.Story, 1<<20))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t$%.2f\t%s\t%s\n",
			client.Clean(r.ID, 64), client.Clean(r.Status, 64), client.Clean(r.Repo, 100),
			pr, r.TotalCostUSD, started, story)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	return ExitOK
}

func summarize(client *server.Client, r server.Run) runSummary {
	clean := func(s *string, limit int) *string {
		if s == nil {
			return nil
		}
		c := client.Clean(*s, limit)
		return &c
	}
	return runSummary{
		ID:           client.Clean(r.ID, 64),
		Repo:         client.Clean(r.Repo, 100),
		Status:       client.Clean(r.Status, 64),
		Mode:         client.Clean(r.Mode, 64),
		Story:        clean(r.Story, server.MaxDetail),
		PRNumber:     clean(r.PRNumber, 16),
		PRURL:        clean(r.PRURL, server.MaxDetail),
		TotalCostUSD: r.TotalCostUSD,
		StartedAt:    clean(r.StartedAt, 64),
		FinishedAt:   clean(r.FinishedAt, 64),
	}
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// storyLine fits a title in storyWidth characters, ellipsis included.
func storyLine(s string) string {
	if utf8.RuneCountInString(s) <= storyWidth {
		return s
	}
	return strings.TrimRight(cutRunes(s, storyWidth-3), " ") + "..."
}
