// Command sentinel is the Sentinel command-line client.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/cli"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

func main() {
	// Ctrl-C cancels the context, which ends any child process (ssh, git, gh)
	// instead of leaving it behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// Without a configuration folder there is simply no saved sign-in.
	configDir, _ := os.UserConfigDir()
	code := cli.Run(ctx, os.Args[1:], cli.Env{
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Runner:    execx.OS{},
		Build:     buildinfo.Read(),
		Getenv:    os.Getenv,
		ConfigDir: configDir,
	})
	stop()
	os.Exit(code)
}
