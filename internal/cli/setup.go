package cli

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/doctor"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/hostprobe"
)

// releaseTag is what a released build reports as its version. A local build
// reports "dev": setup refuses it rather than install a different release.
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// setupEmail is the narrow form of an address that may travel in the remote
// command line: ssh hands that line to the server's shell.
var setupEmail = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)

// remoteBinary is where install.sh puts sentinel, relative to the home
// directory ssh starts in.
const remoteBinary = ".local/bin/sentinel"

// sshTo is ssh as doctor runs it: never a password prompt, a host key recorded
// on first contact and refused if it changes, and "--" so the destination can
// never be read as an option.
func sshTo(dest string, remote ...string) []string {
	return append([]string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-T",
		"--", dest,
	}, remote...)
}

// installScript installs release version on the server unless it is already
// there. version matched releaseTag, so it is safe inside the script.
func installScript(version string) string {
	return strings.Join([]string{
		"set -eu",
		`bin="$HOME/` + remoteBinary + `"`,
		`if [ -x "$bin" ] && [ "$("$bin" version 2>/dev/null | cut -d ' ' -f 2)" = "` + version + `" ]; then`,
		`  echo "sentinel ` + version + ` is already installed"`,
		"  exit 0",
		"fi",
		`command -v curl >/dev/null 2>&1 || { echo "curl is needed on the server to download sentinel" >&2; exit 3; }`,
		`curl -fsSL "https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/` + version +
			`/scripts/install.sh" | SENTINEL_VERSION="` + version + `" SENTINEL_NO_MODIFY_PATH=1 sh`,
		"",
	}, "\n")
}

// setupBlockers are the doctor checks that stop setup. Privileges are not
// among them: setup never runs sudo, it needs a user that may use Docker.
func setupBlockers(report doctor.Report) []doctor.Check {
	var blockers []doctor.Check
	for _, check := range report.Checks {
		switch {
		case check.ID == "privileges":
		case check.Status == doctor.Fail:
			blockers = append(blockers, check)
		case check.ID == "docker" && check.Status != doctor.OK:
			blockers = append(blockers, check)
		}
	}
	return blockers
}

func runSetup(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	email := fs.String("email", "", "the email of the owner; needed the first time")
	signIn := fs.Bool("sign-in", false, "print a new sign-in link for the owner")
	if code := parse(fs, args, env, "setup [--email <address>] [--sign-in] <destination>"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "sentinel setup takes one destination: user@host, or a Host from your ssh config")
		return ExitUsage
	}
	dest := fs.Arg(0)
	if err := hostprobe.ValidateDestination(dest); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel setup: %v\n", err)
		return ExitUsage
	}
	if *email != "" && !setupEmail.MatchString(*email) {
		fmt.Fprintln(env.Stderr, "sentinel setup: that is not an email address")
		return ExitUsage
	}
	version := env.Build.Version
	if !releaseTag.MatchString(version) {
		fmt.Fprintf(env.Stderr, "sentinel setup: this build of sentinel (%s) is not a release, so the server cannot get the same one. Install a release of sentinel and run this again\n", version)
		return ExitFailed
	}

	report := doctor.Host(ctx, env.Runner, dest)
	if blockers := setupBlockers(report); len(blockers) > 0 {
		report.WriteText(env.Stderr)
		fmt.Fprintf(env.Stderr, "sentinel setup: %s is not ready: fix %s, then run this again\n", dest, checkTitles(blockers))
		return ExitFailed
	}

	// The session of this machine goes to the server on ssh's standard input,
	// never in an argument, and is never printed.
	token, err := newInstall(env, "").GitHubToken(ctx)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel setup: %v\n", err)
		return ExitFailed
	}

	fmt.Fprintf(env.Stderr, "Installing sentinel %s on %s\n", version, dest)
	res, err := env.Runner.Run(ctx, execx.Command{Name: "ssh", Args: sshTo(dest, "sh", "-s"), Stdin: installScript(version)})
	if err != nil || res.ExitCode != 0 {
		fmt.Fprintf(env.Stderr, "sentinel setup: installing sentinel on %s failed: %s\n", dest, failure(res, err))
		return ExitFailed
	}

	remote := []string{remoteBinary, "up", "--github-token-stdin"}
	if *email != "" {
		remote = append(remote, "--email", strings.ToLower(*email))
	}
	if *signIn {
		remote = append(remote, "--sign-in")
	}
	fmt.Fprintf(env.Stderr, "Starting Sentinel on %s (the first time takes a few minutes)\n", dest)
	res, err = env.Runner.Run(ctx, execx.Command{Name: "ssh", Args: sshTo(dest, remote...), Stdin: token + "\n"})
	if err != nil || res.ExitCode != 0 {
		fmt.Fprintf(env.Stderr, "sentinel setup: starting Sentinel on %s failed: %s\n", dest, failure(res, err))
		return ExitFailed
	}
	fmt.Fprint(env.Stdout, res.Stdout)
	fmt.Fprintf(env.Stdout, "The panel listens only on the server. To open it from this machine, keep this running:\n  ssh -N -L 8080:127.0.0.1:8080 %s\nand open http://localhost:8080, or the sign-in link above.\n", dest)
	return ExitOK
}

func checkTitles(checks []doctor.Check) string {
	titles := make([]string, len(checks))
	for i, check := range checks {
		titles[i] = check.Title
	}
	return strings.Join(titles, ", ")
}

// failure is the last line a failed remote step printed, for the error message.
func failure(res execx.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	for _, out := range []string{res.Stderr, res.Stdout} {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
			return last
		}
	}
	return fmt.Sprintf("exit status %d", res.ExitCode)
}
