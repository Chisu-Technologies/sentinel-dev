package cli

import (
	"context"
	"flag"
	"fmt"
	"runtime"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
)

// reposPage is where the panel installs the App on repositories.
const reposPage = local.PanelURL + "/config?tab=repos"

func runGitHub(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("github", flag.ContinueOnError)
	org := fs.String("org", "", "create the App under this GitHub organization instead of your account")
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	noBrowser := fs.Bool("no-browser", false, "print the address to open instead of opening the browser")
	if code := parse(fs, args, env, "github [--org <name>] [--no-browser] [--dir <folder>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel github takes no arguments, only flags")
		return ExitUsage
	}
	if *dir == "" {
		d, err := localDir(env)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel github: %v\n", err)
			return ExitFailed
		}
		*dir = d
	}
	open := func(url string) error { return openBrowser(ctx, env, url) }
	if *noBrowser {
		open = nil
	}
	app, err := newInstall(env, *dir).ConnectGitHub(ctx, local.GitHubConnect{
		Org:  *org,
		Open: open,
		Waiting: func(url string) {
			fmt.Fprintf(env.Stderr, "Confirm the new GitHub App in your browser. If no page opened, go to:\n  %s\n", url)
		},
		Web:  env.getenv("SENTINEL_GITHUB_WEB"),
		API:  env.getenv("SENTINEL_GITHUB_API"),
		HTTP: env.HTTP,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel github: %v\n", err)
		return ExitFailed
	}
	owner := app.Owner.Login
	if owner == "" {
		owner = "your account"
	}
	fmt.Fprintf(env.Stdout, "Created the GitHub App %s, owned by %s. Sentinel is running with it.\n", app.Slug, owner)
	fmt.Fprintln(env.Stdout, "Now choose the repositories it may work on: Connect GitHub, on this page of the panel:")
	fmt.Fprintln(env.Stdout, reposPage)
	if !*noBrowser {
		_ = openBrowser(ctx, env, reposPage)
	}
	return ExitOK
}

// openBrowser asks the system to open url in the user's browser.
func openBrowser(ctx context.Context, env Env, url string) error {
	if env.Runner == nil {
		return nil
	}
	cmd := execx.Command{Name: "xdg-open", Args: []string{url}}
	switch runtime.GOOS {
	case "windows":
		cmd = execx.Command{Name: "rundll32", Args: []string{"url.dll,FileProtocolHandler", url}}
	case "darwin":
		cmd = execx.Command{Name: "open", Args: []string{url}}
	}
	_, err := env.Runner.Run(ctx, cmd)
	return err
}
