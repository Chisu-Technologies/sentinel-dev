package doctor

import (
	"context"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// Local checks the tools the sentinel client needs on this machine: git to
// package a task, gh for the GitHub sign-in it reuses, and ssh to reach a
// server.
func Local(ctx context.Context, runner execx.Runner) Report {
	report := Report{Target: "This machine"}
	report.Checks = append(report.Checks, tool(ctx, runner, "git", "git",
		"Install git: https://git-scm.com/downloads", "--version"))

	gh := tool(ctx, runner, "gh", "GitHub CLI",
		"Install the GitHub CLI: https://cli.github.com", "--version")
	report.Checks = append(report.Checks, gh)
	// Asking gh about its session only makes sense when gh exists.
	if gh.Status == OK {
		report.Checks = append(report.Checks, ghSignIn(ctx, runner))
	}

	report.Checks = append(report.Checks, tool(ctx, runner, "ssh", "ssh",
		"Install an OpenSSH client. On Windows: Settings > System > Optional features > OpenSSH Client.",
		"-V"))
	return report
}

// tool checks that a program is installed and reports its version line.
func tool(ctx context.Context, runner execx.Runner, name, title, fix, versionArg string) Check {
	check := Check{ID: name, Title: title}
	if _, err := runner.LookPath(name); err != nil {
		check.Status, check.Detail, check.Fix = Fail, "not installed", fix
		return check
	}
	check.Status, check.Detail = OK, "installed"
	// The version is a courtesy. A program that is installed but will not
	// print one is still installed.
	if res, err := runner.Run(ctx, execx.Command{Name: name, Args: []string{versionArg}}); err == nil && res.ExitCode == 0 {
		// ssh -V writes to stderr; git and gh write to stdout.
		if line := firstLine(res.Stdout + res.Stderr); line != "" {
			check.Detail = line
		}
	}
	return check
}

// ghSignIn checks that gh holds a session for github.com. It reads only the
// exit status: the output of that command names accounts and token scopes,
// and none of that belongs in a report.
func ghSignIn(ctx context.Context, runner execx.Runner) Check {
	check := Check{ID: "gh-sign-in", Title: "GitHub sign-in"}
	res, err := runner.Run(ctx, execx.Command{Name: "gh", Args: []string{"auth", "status", "--hostname", "github.com"}})
	if err != nil || res.ExitCode != 0 {
		check.Status, check.Detail, check.Fix = Fail, "gh is not signed in to github.com", "Run: gh auth login"
		return check
	}
	check.Status, check.Detail = OK, "signed in to github.com"
	return check
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
