package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, runner execx.Runner, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: runner,
		Build:  buildinfo.Info{Version: "v0.0.0-test", Go: "go1.26", OS: "linux", Arch: "amd64"},
	})
	return result{code, stdout.String(), stderr.String()}
}

func readyMachine() *execxtest.Fake {
	return &execxtest.Fake{Results: map[string]execx.Result{
		"git --version": {Stdout: "git version 2.47.0\n"},
		"gh --version":  {Stdout: "gh version 2.60.0\n"},
		"ssh -V":        {Stderr: "OpenSSH_9.9p1\n"},
	}}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		got := run(t, &execxtest.Fake{}, args...)
		if got.code != ExitOK || !strings.Contains(got.stdout, "Usage:") || got.stderr != "" {
			t.Errorf("sentinel %q = %+v, want the usage on stdout and status 0", args, got)
		}
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	got := run(t, &execxtest.Fake{}, "deploy")
	if got.code != ExitUsage {
		t.Errorf("status = %d, want %d", got.code, ExitUsage)
	}
	if !strings.Contains(got.stderr, `unknown command "deploy"`) || got.stdout != "" {
		t.Errorf("output = %+v, want the complaint on stderr only", got)
	}
}

func TestVersion(t *testing.T) {
	got := run(t, &execxtest.Fake{}, "version")
	if got.code != ExitOK || got.stdout != "sentinel v0.0.0-test (go1.26, linux/amd64)\n" {
		t.Errorf("sentinel version = %+v", got)
	}

	got = run(t, &execxtest.Fake{}, "version", "--json")
	var info buildinfo.Info
	if err := json.Unmarshal([]byte(got.stdout), &info); err != nil {
		t.Fatalf("version --json does not parse: %v\n%s", err, got.stdout)
	}
	if got.code != ExitOK || info.Version != "v0.0.0-test" || info.Arch != "amd64" {
		t.Errorf("version --json = %+v (status %d)", info, got.code)
	}

	if got := run(t, &execxtest.Fake{}, "version", "extra"); got.code != ExitUsage {
		t.Errorf("version with an argument: status = %d, want %d", got.code, ExitUsage)
	}
}

func TestDoctorOnAReadyMachineExitsZero(t *testing.T) {
	got := run(t, readyMachine(), "doctor")
	if got.code != ExitOK || !strings.Contains(got.stdout, "All checks passed.") {
		t.Errorf("sentinel doctor = %+v, want status 0 and every check passed", got)
	}
}

func TestDoctorExitsOneWhenACheckFails(t *testing.T) {
	fake := readyMachine()
	fake.Missing = map[string]bool{"git": true}
	got := run(t, fake, "doctor")
	if got.code != ExitFailed {
		t.Errorf("status = %d, want %d", got.code, ExitFailed)
	}
	if !strings.Contains(got.stdout, "fix: Install git") {
		t.Errorf("stdout = %q, want the fix for the missing tool", got.stdout)
	}
}

func TestDoctorJSONIsOneDocumentAndKeepsTheExitStatus(t *testing.T) {
	fake := readyMachine()
	fake.Missing = map[string]bool{"ssh": true}
	got := run(t, fake, "doctor", "--json")
	var report struct {
		Target string
		Checks []struct{ ID, Status, Fix string }
	}
	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		t.Fatalf("doctor --json does not parse: %v\n%s", err, got.stdout)
	}
	if got.code != ExitFailed || report.Target != "This machine" {
		t.Errorf("status = %d, target = %q", got.code, report.Target)
	}
	failed := 0
	for _, c := range report.Checks {
		if c.Status == "fail" {
			failed++
			if c.ID != "ssh" || c.Fix == "" {
				t.Errorf("failed check = %+v, want ssh with a fix", c)
			}
		}
	}
	if failed != 1 {
		t.Errorf("%d checks failed, want 1", failed)
	}
}

func TestDoctorWithADestinationProbesItOverSSH(t *testing.T) {
	out := "sentinel-probe-begin\nos_id=ubuntu\nos_version_id=24.04\nkernel=Linux\narch=x86_64\nprivileges=root\nsystemd=yes\nmem_total_kb=8000000\ncpus=4\ndisk_var_avail_kb=100\ndocker=yes\nkvm=no\nsentinel-probe-end\n"
	fake := &execxtest.Fake{Results: map[string]execx.Result{"ssh": {Stdout: out}}}
	got := run(t, fake, "doctor", "deploy@10.0.0.5")
	if got.code != ExitOK || !strings.HasPrefix(got.stdout, "deploy@10.0.0.5\n") {
		t.Errorf("sentinel doctor deploy@10.0.0.5 = %+v", got)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].Name != "ssh" {
		t.Errorf("calls = %+v, want one ssh and no local checks", fake.Calls)
	}
}

func TestDoctorRejectsMoreThanOneDestinationAndUnknownFlags(t *testing.T) {
	fake := &execxtest.Fake{}
	for _, args := range [][]string{{"doctor", "a", "b"}, {"doctor", "--nope"}, {"doctor", "-oProxyCommand=id"}} {
		got := run(t, fake, args...)
		if got.code != ExitUsage {
			t.Errorf("sentinel %q: status = %d, want %d", args, got.code, ExitUsage)
		}
	}
	if len(fake.Calls) != 0 {
		t.Errorf("a rejected command line still ran %+v", fake.Calls)
	}
}

func TestCommandHelpGoesToStderrAndExitsZero(t *testing.T) {
	got := run(t, &execxtest.Fake{}, "doctor", "-h")
	if got.code != ExitOK || !strings.Contains(got.stderr, "Usage: sentinel doctor [--json] [destination]") {
		t.Errorf("sentinel doctor -h = %+v", got)
	}
}
