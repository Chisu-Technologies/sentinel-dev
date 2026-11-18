package cli

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
)

// runBackup dumps the databases of the Sentinel on this machine.
func runBackup(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	out := fs.String("out", "", "the folder the backup goes in; the installation's backups folder by default")
	if code := parse(fs, args, env, "backup [--out <folder>] [--dir <folder>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel backup takes no arguments, only flags")
		return ExitUsage
	}
	install, code := localInstall(env, "backup", *dir)
	if code >= 0 {
		return code
	}
	parent := *out
	if parent == "" {
		parent = filepath.Join(install.Dir, "backups")
	}
	path, err := install.Backup(ctx, parent, time.Now())
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel backup: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintf(env.Stdout, "Backup in %s\n", path)
	fmt.Fprintf(env.Stdout, "Keep %s with it: its master key is what reads the encrypted data, and the backup does not carry it.\n",
		filepath.Join(install.Dir, ".env"))
	return ExitOK
}

// runRestore replaces the databases of the Sentinel on this machine with a
// backup. It needs --yes: what is there now is lost.
func runRestore(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	yes := fs.Bool("yes", false, "replace the data of this installation with the backup")
	if code := parse(fs, args, env, "restore --yes <backup folder> [--dir <folder>]"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel restore takes one backup folder, as sentinel backup printed it")
		return ExitUsage
	}
	backup := fs.Arg(0)
	// Checked before --yes, so a damaged backup is said even to someone who
	// is only looking.
	if err := local.VerifyBackup(backup); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel restore: %v\n", err)
		return ExitFailed
	}
	if !*yes {
		fmt.Fprintf(env.Stderr, "sentinel restore replaces every run, plan and setting of this installation with %s.\nRun it again with --yes to do it.\n", backup)
		return ExitUsage
	}
	install, code := localInstall(env, "restore", *dir)
	if code >= 0 {
		return code
	}
	if err := install.Restore(ctx, backup); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel restore: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintf(env.Stdout, "Restored from %s. Sentinel is stopped; sentinel up starts it again.\n", backup)
	return ExitOK
}

// localInstall is the installation on this machine, in dir or the default
// folder. A code of zero or more means the command ends with it.
func localInstall(env Env, command, dir string) (*local.Install, int) {
	if dir == "" {
		d, err := localDir(env)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel %s: %v\n", command, err)
			return nil, ExitFailed
		}
		dir = d
	}
	return newInstall(env, dir), -1
}
