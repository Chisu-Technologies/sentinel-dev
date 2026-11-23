package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// dryRun is what `sentinel run --dry-run` reports. The patch itself is not
// printed: it is the content of the working tree.
type dryRun struct {
	Task string `json:"task"`
	checkout.Package
	PatchBytes int `json:"patch_bytes"`
}

// runResult is what `sentinel run --json` prints. Fields after status are
// absent until the run has an outcome.
type runResult struct {
	Task           string   `json:"task"`
	Repository     string   `json:"repository"`
	BaseCommit     string   `json:"base_commit"`
	PatchFiles     int      `json:"patch_files"`
	RunID          string   `json:"run_id"`
	Status         string   `json:"status"`
	PullRequestURL string   `json:"pull_request_url,omitempty"`
	Verdict        string   `json:"verdict,omitempty"`
	CostUSD        *float64 `json:"cost_usd,omitempty"`
	TestsPassed    *int     `json:"tests_passed,omitempty"`
	TestsTotal     *int     `json:"tests_total,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

func runRun(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "show what would be sent, and send nothing")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	noWait := fs.Bool("no-wait", false, "print the run id and stop, without waiting for the pull request")
	timeout := fs.Duration("timeout", 2*time.Hour, "how long to wait for the run to end, such as 90m")
	base := fs.String("base", "", "the branch the pull request targets, when it is not the default one")
	fromClaudeCode := fs.Bool("from-claude-code", false, "take the task from the newest Claude Code session in this folder")
	qaByAgent := fs.Bool("qa-by-agent", false, "the implementer, the QA and the reviewers are answered by the coding agent that sends the run, with its own subagents (Claude Code's sentinel plugin)")
	if code := parse(fs, args, env, `run [--dry-run] [--json] [--no-wait] [--timeout <duration>] [--base <branch>] [--qa-by-agent] "<task>" | run --from-claude-code [...]`); code >= 0 {
		return code
	}
	task := strings.TrimSpace(fs.Arg(0))
	if *fromClaudeCode {
		if fs.NArg() != 0 {
			fmt.Fprintln(env.Stderr, "sentinel run --from-claude-code takes the task from the Claude Code session; give no task with it")
			return ExitUsage
		}
	} else if fs.NArg() != 1 || task == "" {
		fmt.Fprintln(env.Stderr, `sentinel run takes the task as one argument, for example: sentinel run "fix the flaky login test"`)
		return ExitUsage
	}
	if *timeout <= 0 {
		fmt.Fprintln(env.Stderr, "sentinel run: --timeout must be longer than zero")
		return ExitUsage
	}
	baseGiven := false
	fs.Visit(func(f *flag.Flag) { baseGiven = baseGiven || f.Name == "base" })
	if baseGiven && !validBase(*base) {
		fmt.Fprintln(env.Stderr, "sentinel run: --base needs a branch name")
		return ExitUsage
	}
	if *fromClaudeCode {
		taken, session, err := claudeCodeTask(env, env.Dir)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel run: %v\n", err)
			return ExitFailed
		}
		fmt.Fprintf(env.Stderr, "sentinel: the task comes from Claude Code session %s\n", session)
		task = taken
	}

	// The variables are checked before git reads the tree, so a missing one
	// is said at once; a dry run needs neither.
	var client *server.Client
	if !*dry {
		var code int
		if client, code = newServerClient(env, "run"); code >= 0 {
			return code
		}
	}
	pkg, err := checkout.Build(ctx, env.Runner, env.Dir)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	if *dry {
		return runDryRunReport(env, task, pkg, *asJSON)
	}
	return runTask(ctx, env, client, task, pkg, *base, *qaByAgent, *noWait, *timeout, *asJSON)
}

func runDryRunReport(env Env, task string, pkg checkout.Package, asJSON bool) int {
	if asJSON {
		return writeJSON(env, dryRun{Task: task, Package: pkg, PatchBytes: len(pkg.Patch)})
	}
	fmt.Fprintf(env.Stdout, "Task         %s\n", task)
	fmt.Fprintf(env.Stdout, "Repository   %s\n", pkg.Repository)
	fmt.Fprintf(env.Stdout, "Base commit  %s\n", pkg.BaseCommit)
	if len(pkg.Files) == 0 {
		fmt.Fprintln(env.Stdout, "Patch        none: GitHub already has everything in this working tree")
	} else {
		fmt.Fprintf(env.Stdout, "Patch        %s, %s\n", count(len(pkg.Files), "file"), size(len(pkg.Patch)))
		for _, file := range pkg.Files {
			fmt.Fprintf(env.Stdout, "  %s\n", file)
		}
	}
	fmt.Fprintln(env.Stdout, "\nNothing was sent: this was a dry run.")
	return ExitOK
}

func runTask(ctx context.Context, env Env, client *server.Client, task string, pkg checkout.Package, base string, qaByAgent, noWait bool, timeout time.Duration, asJSON bool) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	run, err := client.StartTask(ctx, server.TaskRequest{
		Repository: pkg.Repository,
		BaseRef:    base,
		Task:       task,
		BaseCommit: pkg.BaseCommit,
		Patch:      pkg.Patch,
		QAByAgent:  qaByAgent,
	})
	if err != nil {
		return fail("%v", err)
	}
	return followRun(ctx, env, client, run, runResult{
		Task:       task,
		Repository: pkg.Repository,
		BaseCommit: pkg.BaseCommit,
		PatchFiles: len(pkg.Files),
	}, noWait, timeout, asJSON)
}

// followRun reports a run that was just created: at once with noWait, else
// when it ends, with its pull request and verdict. The commands that start a
// run share it.
func followRun(ctx context.Context, env Env, client *server.Client, run server.Run, result runResult, noWait bool, timeout time.Duration, asJSON bool) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}
	status := client.Clean(run.Status, 64)
	fmt.Fprintf(env.Stderr, "Run %s created (%s).\n", run.ID, status)
	result.RunID = run.ID
	result.Status = status
	if noWait {
		return printRunResult(env, result, server.Story{}, asJSON, ExitOK)
	}

	// From here a failure leaves a run behind on the server; every message says
	// so, with the id, so the user can look at it there.
	stillRunning := func(why string) int {
		return fail("%s Run %s continues on the server.", why, run.ID)
	}
	w := newWaiter(ctx, env, client, timeout)
	defer w.cancel()
	final, why := w.until(run.ID, run.Status, func(next server.Run) error {
		if next.Status != run.Status {
			fmt.Fprintf(env.Stderr, "Run %s: %s\n", run.ID, client.Clean(next.Status, 64))
		}
		run.Status = next.Status
		run.TotalCostUSD = next.TotalCostUSD
		return nil
	})
	if why != "" {
		return stillRunning(why)
	}
	run.Status = final
	result.Status = client.Clean(run.Status, 64)

	stories, err := client.Stories(w.ctx, run.ID)
	endedWithoutOutcome := run.Status != server.StatusCompleted && run.Status != server.StatusBlocked
	if err != nil && !endedWithoutOutcome {
		return stillRunning(w.classify(err, false))
	}
	var story server.Story
	if err == nil && len(stories) > 0 {
		story = stories[0]
	}
	verdict := story.Verdict()
	if endedWithoutOutcome && verdict == "APPROVED" {
		// A run that failed, was stopped or was abandoned delivered nothing,
		// whatever its story says.
		verdict = "UNKNOWN"
	}
	result.Verdict = client.Clean(verdict, 64)
	cost := runCost(run, story)
	result.CostUSD = &cost
	if story.PRURL != nil && strings.HasPrefix(*story.PRURL, "https://github.com/") {
		result.PullRequestURL = client.Clean(*story.PRURL, server.MaxDetail)
	}
	if ev := story.Evidence; ev != nil && ev.TestsTotal != nil && *ev.TestsTotal > 0 {
		total := int(*ev.TestsTotal)
		result.TestsTotal = &total
		if ev.TestsPassed != nil {
			passed := int(*ev.TestsPassed)
			result.TestsPassed = &passed
		}
	}
	if result.Verdict != "APPROVED" && story.BlockedDetail != nil {
		result.Reason = client.Clean(*story.BlockedDetail, server.MaxDetail)
	}

	// Success is a pull request that passed QA: that is what the task asked for.
	code := ExitFailed
	if result.Verdict == "APPROVED" && result.PullRequestURL != "" {
		code = ExitOK
	}
	return printRunResult(env, result, story, asJSON, code)
}

func printRunResult(env Env, result runResult, story server.Story, asJSON bool, code int) int {
	if asJSON {
		if out := writeJSON(env, result); out != ExitOK {
			return out
		}
		return code
	}
	table(env, "Task", result.Task)
	table(env, "Repository", result.Repository)
	table(env, "Run", fmt.Sprintf("%s (%s)", result.RunID, result.Status))
	if result.PullRequestURL != "" {
		table(env, "Pull request", result.PullRequestURL)
	}
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
