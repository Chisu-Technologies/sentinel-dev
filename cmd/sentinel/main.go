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
	// Both only feed `sentinel agents`; without them it reads and writes no file.
	homeDir, _ := os.UserHomeDir()
	executable, _ := os.Executable()
	code := cli.Run(ctx, os.Args[1:], cli.Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Runner:     execx.OS{},
		Build:      buildinfo.Read(),
		Getenv:     os.Getenv,
		ConfigDir:  configDir,
		HomeDir:    homeDir,
		Executable: executable,
	})
	stop()
	os.Exit(code)
}
