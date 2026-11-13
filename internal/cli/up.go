package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
)

// localDir is where an installation on this machine keeps its files, unless
// --dir names another place.
func localDir(env Env) (string, error) {
	if dir := env.getenv("SENTINEL_HOME"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("could not find a folder for the installation; pass --dir")
	}
	return filepath.Join(base, "sentinel", "local"), nil
}

func newInstall(env Env, dir string) *local.Install {
	bundle := env.Bundle
	if bundle == nil {
		bundle = local.Bundle()
	}
	return &local.Install{Runner: env.Runner, Dir: dir, Bundle: bundle, Rand: env.Rand, Progress: env.Stderr}
}

func runUp(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	email := fs.String("email", "", "the email of the owner; needed the first time")
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	signIn := fs.Bool("sign-in", false, "print a new sign-in link for the owner")
	if code := parse(fs, args, env, "up [--email <address>] [--sign-in] [--dir <folder>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel up takes no arguments, only flags")
		return ExitUsage
	}
	if *dir == "" {
		d, err := localDir(env)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel up: %v\n", err)
			return ExitFailed
		}
		*dir = d
	}
	result, err := newInstall(env, *dir).Up(ctx, *email, *signIn)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel up: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintf(env.Stdout, "Sentinel is running: %s\n", result.PanelURL)
	if result.SignInURL != "" {
		if result.Created {
			fmt.Fprintf(env.Stdout, "Owner: %s\n", result.Owner)
		}
		fmt.Fprintln(env.Stdout, "Open this link to sign in. It works once and expires in 15 minutes:")
		fmt.Fprintln(env.Stdout, result.SignInURL)
	} else {
		fmt.Fprintln(env.Stdout, "To get a new sign-in link: sentinel up --sign-in")
	}
	return ExitOK
}

func runDown(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("down", flag.ContinueOnError)
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	if code := parse(fs, args, env, "down [--dir <folder>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel down takes no arguments, only flags")
		return ExitUsage
	}
	if *dir == "" {
		d, err := localDir(env)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel down: %v\n", err)
			return ExitFailed
		}
		*dir = d
	}
	if err := newInstall(env, *dir).Down(ctx); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel down: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintln(env.Stdout, "Sentinel is stopped. Its data stays; sentinel up starts it again.")
	return ExitOK
}
