package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// maxLoginWait bounds the wait whatever the server says: a sign-in nobody
// approves should not keep a terminal busy for long.
const maxLoginWait = 15 * time.Minute

func runLogin(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	address := fs.String("url", "", "the address of your Sentinel server; when not given, SENTINEL_URL or the one sentinel up installed here")
	withChatGPT := fs.Bool("chatgpt", false, "lend your ChatGPT plan to the Sentinel you are signed in to")
	if code := parse(fs, args, env, "login [--url <address>] | login --chatgpt"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel login takes no arguments; give the server with --url or SENTINEL_URL")
		return ExitUsage
	}
	if *withChatGPT {
		if *address != "" {
			fmt.Fprintln(env.Stderr, "sentinel login --chatgpt uses the server you are already signed in to; sign in with sentinel login --url first")
			return ExitUsage
		}
		// One command, no prerequisite to guess: a CLI that is not signed in yet
		// signs in to the Sentinel `up` runs on this machine first.
		if panel, ok := unsignedLocalInstall(env); ok {
			fmt.Fprintf(env.Stdout, "First, this command signs in to the Sentinel on this machine (%s).\n", panel)
			if code := signInTo(ctx, env, panel); code != ExitOK {
				return code
			}
			fmt.Fprintln(env.Stdout)
		}
		return runLoginChatGPT(ctx, env)
	}
	raw := *address
	if raw == "" {
		raw = env.getenv("SENTINEL_URL")
	}
	if raw == "" {
		// The panel's setup guide says "run sentinel login" with no address: on a
		// machine where `up` installed Sentinel, that one is the server.
		panel, ok := localPanel(env)
		if !ok {
			fmt.Fprintln(env.Stderr, "sentinel login needs the address of your Sentinel server: sentinel login --url https://sentinel.example.com")
			return ExitUsage
		}
		fmt.Fprintf(env.Stdout, "Signing in to the Sentinel on this machine (%s).\n", panel)
		raw = panel
	}
	return signInTo(ctx, env, raw)
}

// unsignedLocalInstall reports the panel of this machine's installation when
// the CLI has no server to talk to yet: no SENTINEL_URL, no saved sign-in.
func unsignedLocalInstall(env Env) (string, bool) {
	if env.getenv("SENTINEL_URL") != "" || env.getenv("SENTINEL_API_KEY") != "" {
		return "", false
	}
	if _, ok, err := loadCredentials(env); err != nil || ok {
		return "", false
	}
	return localPanel(env)
}

// localPanel reports the panel of the installation `up` made on this machine.
func localPanel(env Env) (string, bool) {
	// Env.HomeDir, not the process's home: a test leaves it empty and never sees
	// the machine's own installation.
	dir := env.getenv("SENTINEL_HOME")
	if dir == "" {
		if env.HomeDir == "" {
			return "", false
		}
		dir = filepath.Join(env.HomeDir, ".sentinel", "local")
	}
	// `up` writes the installation's secrets there first; without them there is no local Sentinel.
	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		return "", false
	}
	if env.LocalPanelURL != "" {
		return env.LocalPanelURL, true
	}
	return local.PanelURL, true
}

// signInTo signs this CLI in to the server at raw, approved in its panel.
func signInTo(ctx context.Context, env Env, raw string) int {
	httpClient := env.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client, err := server.NewLogin(raw, httpClient, "sentinel/"+env.Build.Version)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel login: %v\n", err)
		return ExitUsage
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}

	start, err := client.Start(ctx, fmt.Sprintf("sentinel %s on %s/%s", env.Build.Version, env.Build.OS, env.Build.Arch))
	if err != nil {
		return fail("%v", err)
	}
	fmt.Fprintf(env.Stdout, "Open this page and approve the code %s:\n\n  %s\n\nWaiting for the approval...\n", start.UserCode, client.ApprovalURL(start.UserCode))
	openInBrowser(ctx, env, client.ApprovalURL(start.UserCode))

	wait := time.Duration(start.ExpiresIn) * time.Second
	if wait <= 0 || wait > maxLoginWait {
		wait = maxLoginWait
	}
	interval := time.Duration(start.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	sleep := env.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	for {
		if err := sleep(ctx, interval); err != nil {
			return fail("nobody approved the sign-in in time; run sentinel login again")
		}
		key, err := client.Collect(ctx, start.DeviceCode)
		if errors.Is(err, server.ErrLoginPending) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return fail("nobody approved the sign-in in time; run sentinel login again")
			}
			return fail("%v", err)
		}
		path, err := saveCredentials(env, credentials{URL: client.Base(), APIKey: key.APIKey})
		if err != nil {
			return fail("the sign-in was approved but its key could not be saved: %v. Revoke the key named \"sentinel CLI\" in Settings > API keys and try again", err)
		}
		organization := "your organization"
		if key.Organization != "" {
			organization = cleanName(key.Organization)
		}
		fmt.Fprintf(env.Stdout, "Signed in to %s, %s. The key is saved in %s; revoke it in Settings > API keys.\n", client.Host(), organization, path)
		return ExitOK
	}
}

func runLogout(_ context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	if code := parse(fs, args, env, "logout"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel logout takes no arguments")
		return ExitUsage
	}
	path, removed, err := removeCredentials(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel: %v\n", err)
		return ExitFailed
	}
	if !removed {
		fmt.Fprintln(env.Stdout, "There was no sign-in to remove.")
		return ExitOK
	}
	fmt.Fprintf(env.Stdout, "Removed %s. The key still works until you revoke it in Settings > API keys.\n", path)
	return ExitOK
}

// cleanName keeps a name from the server fit to print on one line.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) > 120 {
		s = string([]rune(s)[:120])
	}
	return s
}
