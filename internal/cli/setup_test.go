package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

const setupToken = "gho_setupTokenThatMustNotLeak0123456789"

func probeOutput(privileges, docker string) string {
	return "sentinel-probe-begin\nprobe_version=1\nos_id=ubuntu\nos_version_id=24.04\nkernel=Linux\n" +
		"arch=x86_64\nuid=1000\nprivileges=" + privileges + "\nsystemd=yes\nmem_total_kb=4005120\ncpus=2\n" +
		"disk_var_avail_kb=52428800\ndocker=" + docker + "\nkvm=yes\nsentinel-probe-end\n"
}

// setupRunner answers the three ssh steps of setup by what each sends: the
// probe and the install both run `sh -s`, and only their scripts differ.
type setupRunner struct {
	probe   string
	install execx.Result
	up      execx.Result
	calls   []execx.Command
}

func (r *setupRunner) LookPath(name string) (string, error) { return "/usr/bin/" + name, nil }

func (r *setupRunner) Run(_ context.Context, cmd execx.Command) (execx.Result, error) {
	r.calls = append(r.calls, cmd)
	switch {
	case cmd.Name == "gh":
		return execx.Result{Stdout: setupToken + "\n"}, nil
	case strings.Contains(cmd.Stdin, "sentinel-probe"):
		return execx.Result{Stdout: r.probe}, nil
	case strings.Contains(cmd.Stdin, "install.sh"):
		return r.install, nil
	default:
		return r.up, nil
	}
}

func runSetupWith(t *testing.T, runner *setupRunner, version string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"setup"}, args...), Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: runner,
		Build:  buildinfo.Info{Version: version},
	})
	return code, stdout.String(), stderr.String()
}

func goodRunner() *setupRunner {
	return &setupRunner{
		// No sudo: setup never runs it, so a user that may use Docker is enough.
		probe: probeOutput("none", "yes"),
		up: execx.Result{Stdout: "Sentinel is running: http://localhost:8080\n" +
			"Open this link to sign in. It works once and expires in 15 minutes:\n" +
			"http://localhost:8080/api/auth/verify?token=one-time\n"},
	}
}

func TestSetupInstallsTheSameReleaseAndStartsSentinelWithTheSessionOnStdin(t *testing.T) {
	runner := goodRunner()
	code, stdout, stderr := runSetupWith(t, runner, "v0.3.2", "--email", "Owner@Example.com", "deploy@server.example.com")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	var install, up *execx.Command
	for i := range runner.calls {
		call := &runner.calls[i]
		if strings.Contains(call.Stdin, "install.sh") {
			install = call
		}
		if call.Name == "ssh" && strings.Contains(strings.Join(call.Args, " "), " up ") {
			up = call
		}
	}
	if install == nil || up == nil {
		t.Fatalf("calls = %+v, want an install and an up over ssh", runner.calls)
	}
	// The release is pinned to this CLI's own version, for the script and the binary.
	if !strings.Contains(install.Stdin, "sentinel-dev/v0.3.2/scripts/install.sh") ||
		!strings.Contains(install.Stdin, `SENTINEL_VERSION="v0.3.2"`) {
		t.Errorf("install script = %q, want release v0.3.2 pinned", install.Stdin)
	}
	want := sshTo("deploy@server.example.com", ".local/bin/sentinel", "up", "--github-token-stdin", "--email", "owner@example.com")
	if !reflect.DeepEqual(up.Args, want) {
		t.Errorf("up args = %q, want %q", up.Args, want)
	}
	if up.Stdin != setupToken+"\n" {
		t.Errorf("up stdin does not carry the session")
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call.Args, " "), setupToken) {
			t.Errorf("the session travelled in an argument of %s", call.Name)
		}
	}
	if strings.Contains(stdout+stderr, setupToken) {
		t.Error("the session was printed")
	}
	if !strings.Contains(stdout, "verify?token=one-time") || !strings.Contains(stdout, "ssh -N -L 8080:127.0.0.1:8080 deploy@server.example.com") {
		t.Errorf("stdout = %q, want the server's sign-in link and the tunnel to open it", stdout)
	}
}

func TestSetupRefusesABuildThatIsNotARelease(t *testing.T) {
	runner := goodRunner()
	code, _, stderr := runSetupWith(t, runner, "dev", "deploy@server.example.com")
	if code != ExitFailed || len(runner.calls) != 0 {
		t.Fatalf("exit = %d, calls = %d, want a refusal before any command", code, len(runner.calls))
	}
	if !strings.Contains(stderr, "not a release") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestSetupStopsBeforeInstallingOnAServerWithoutDocker(t *testing.T) {
	runner := goodRunner()
	runner.probe = probeOutput("sudo", "no")
	code, _, stderr := runSetupWith(t, runner, "v0.3.2", "deploy@server.example.com")
	if code != ExitFailed {
		t.Fatalf("exit = %d, want %d", code, ExitFailed)
	}
	if len(runner.calls) != 1 {
		t.Errorf("calls = %+v, want only the probe", runner.calls)
	}
	if !strings.Contains(stderr, "fix Docker") {
		t.Errorf("stderr = %q, want Docker named as what to fix", stderr)
	}
}

func TestSetupRejectsADestinationOrEmailThatCouldReachTheShell(t *testing.T) {
	for _, args := range [][]string{
		{"-oProxyCommand=id@server.example.com"},
		{"--email", "a;touch x@example.com", "deploy@server.example.com"},
		{"--email", "a$(id)@example.com", "deploy@server.example.com"},
	} {
		runner := goodRunner()
		if code, _, _ := runSetupWith(t, runner, "v0.3.2", args...); code != ExitUsage || len(runner.calls) != 0 {
			t.Errorf("args %q: exit = %d, calls = %d, want a usage error before any command", args, code, len(runner.calls))
		}
	}
}

func TestSetupReportsTheLastLineOfAFailedRemoteStep(t *testing.T) {
	runner := goodRunner()
	runner.up = execx.Result{ExitCode: 1, Stderr: "Downloading the images\nsentinel up: Docker is installed but not running\n"}
	code, _, stderr := runSetupWith(t, runner, "v0.3.2", "deploy@server.example.com")
	if code != ExitFailed || !strings.Contains(stderr, "Docker is installed but not running") {
		t.Errorf("exit = %d, stderr = %q", code, stderr)
	}
}
