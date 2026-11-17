package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/update"
)

// releaseSource is where `sentinel update` reads releases. Tests replace it.
// Env.HTTP is not used: it is the client for a Sentinel server, which never
// follows redirects, and GitHub sends release downloads to its file storage.
var releaseSource = func(Env) update.Source {
	return update.GitHub(&http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 5 {
				return errors.New("redirect refused")
			}
			return nil
		},
	})
}

// executablePath is the binary `sentinel update` replaces. Tests replace it.
var executablePath = os.Executable

func runUpdate(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "only say whether a newer release exists")
	if code := parse(fs, args, env, "update [--check]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel update takes no arguments")
		return ExitUsage
	}
	current := env.Build.Version
	if !update.IsRelease(current) {
		fmt.Fprintf(env.Stderr, "sentinel: this is a development build (%s). Install a release with the one-line installer instead.\n", current)
		return ExitFailed
	}

	source := releaseSource(env)
	latest, err := source.Latest(ctx)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	if !update.Newer(current, latest) {
		fmt.Fprintf(env.Stdout, "sentinel %s is the latest release.\n", current)
		return ExitOK
	}
	if *check {
		fmt.Fprintf(env.Stdout, "sentinel %s is available; this is %s. To install it: sentinel update\n", latest, current)
		return ExitOK
	}

	executable, err := executablePath()
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: cannot find this binary: %v\n", err)
		return ExitFailed
	}
	result, err := source.Install(ctx, latest, update.Options{
		GOOS:             env.Build.OS,
		GOARCH:           env.Build.Arch,
		Executable:       executable,
		Runner:           env.Runner,
		RequireSignature: env.getenv("SENTINEL_REQUIRE_SIGNATURE") == "1",
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	if result.SignatureChecked {
		fmt.Fprintf(env.Stdout, "Signature checked: built by the release workflow of %s.\n", update.Repo)
	} else {
		fmt.Fprintln(env.Stdout, "Signature not checked: cosign is not installed.")
	}
	fmt.Fprintf(env.Stdout, "Updated sentinel from %s to %s.\n", current, latest)
	return ExitOK
}
