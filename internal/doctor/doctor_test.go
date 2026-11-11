package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/hostprobe"
)

func statusOf(t *testing.T, checks []Check, id string) Status {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c.Status
		}
	}
	t.Fatalf("no check with id %q in %+v", id, checks)
	return ""
}

func goodHost() hostprobe.Facts {
	return hostprobe.Facts{
		OSID: "ubuntu", OSVersionID: "24.04", Kernel: "Linux", Arch: "x86_64",
		Privileges: "sudo", Systemd: true, MemTotalKB: 4005120, CPUs: 2,
		DiskVarAvail: 52428800, Docker: true,
	}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name   string
		change func(*hostprobe.Facts)
		id     string
		want   Status
	}{
		{"ubuntu 24.04", func(f *hostprobe.Facts) {}, "os", OK},
		{"ubuntu 22.04", func(f *hostprobe.Facts) { f.OSVersionID = "22.04" }, "os", OK},
		{"debian 12", func(f *hostprobe.Facts) { f.OSID, f.OSVersionID = "debian", "12" }, "os", OK},
		{"a newer ubuntu is refused, not guessed at", func(f *hostprobe.Facts) { f.OSVersionID = "26.04" }, "os", Fail},
		{"a debian release under the ubuntu name does not pass", func(f *hostprobe.Facts) { f.OSVersionID = "12" }, "os", Fail},
		{"another distribution", func(f *hostprobe.Facts) { f.OSID, f.OSVersionID = "fedora", "41" }, "os", Fail},
		{"no os-release", func(f *hostprobe.Facts) { f.OSID, f.OSVersionID = "unknown", "unknown" }, "os", Fail},
		{"arm64 server", func(f *hostprobe.Facts) { f.Arch = "aarch64" }, "arch", OK},
		{"32-bit arm", func(f *hostprobe.Facts) { f.Arch = "armv7l" }, "arch", Fail},
		{"root", func(f *hostprobe.Facts) { f.Privileges = "root" }, "privileges", OK},
		{"no way to become root", func(f *hostprobe.Facts) { f.Privileges = "none" }, "privileges", Fail},
		{"privileges not reported", func(f *hostprobe.Facts) { f.Privileges = "" }, "privileges", Fail},
		{"no systemd", func(f *hostprobe.Facts) { f.Systemd = false }, "systemd", Fail},
		{"a 4 GB machine reports less than 4 GiB and still passes", func(f *hostprobe.Facts) { f.MemTotalKB = 3_900_000 }, "memory", OK},
		{"a 2 GB machine is a warning", func(f *hostprobe.Facts) { f.MemTotalKB = 2_000_000 }, "memory", Warn},
		{"memory not reported is a warning", func(f *hostprobe.Facts) { f.MemTotalKB = 0 }, "memory", Warn},
		{"docker missing is a warning", func(f *hostprobe.Facts) { f.Docker = false }, "docker", Warn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := goodHost()
			tc.change(&facts)
			checks := Evaluate(facts)
			if got := statusOf(t, checks, tc.id); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.id, got, tc.want)
			}
			for _, c := range checks {
				if c.Status != OK && c.Fix == "" {
					t.Errorf("check %q is %s and offers no fix", c.ID, c.Status)
				}
				if c.Status == OK && c.Fix != "" {
					t.Errorf("check %q passed and still offers a fix: %q", c.ID, c.Fix)
				}
			}
		})
	}
}

func TestAGoodHostPassesEveryCheck(t *testing.T) {
	report := Report{Checks: Evaluate(goodHost())}
	if report.Failed() || report.count(Warn) != 0 {
		t.Errorf("checks = %+v, want all ok", report.Checks)
	}
}

func signedIn() *execxtest.Fake {
	return &execxtest.Fake{Results: map[string]execx.Result{
		"git --version": {Stdout: "git version 2.47.0\n"},
		"gh --version":  {Stdout: "gh version 2.60.0 (2026-01-01)\nhttps://github.com/cli/cli\n"},
		"ssh -V":        {Stderr: "OpenSSH_9.9p1, OpenSSL 3.3.2\n"},
		// What gh prints here must never reach a report.
		"gh auth status --hostname github.com": {Stdout: "Logged in to github.com account octocat\nToken: gho_notARealToken\n"},
	}}
}

func TestLocalPassesWithTheToolsInstalledAndSignedIn(t *testing.T) {
	report := Local(context.Background(), signedIn())
	if report.Failed() {
		t.Fatalf("report = %+v, want no failure", report.Checks)
	}
	for _, c := range report.Checks {
		if c.ID == "ssh" && c.Detail != "OpenSSH_9.9p1, OpenSSL 3.3.2" {
			t.Errorf("ssh detail = %q, want the version ssh prints on stderr", c.Detail)
		}
		if c.ID == "gh" && c.Detail != "gh version 2.60.0 (2026-01-01)" {
			t.Errorf("gh detail = %q, want only the first line", c.Detail)
		}
	}
}

func TestLocalNeverRepeatsWhatGhSaysAboutTheSession(t *testing.T) {
	var out bytes.Buffer
	report := Local(context.Background(), signedIn())
	report.WriteText(&out)
	if err := report.WriteJSON(&out); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"gho_", "octocat", "Token"} {
		if strings.Contains(out.String(), leaked) {
			t.Errorf("the report repeats %q from `gh auth status`:\n%s", leaked, out.String())
		}
	}
}

func TestLocalWithoutGhFailsOnceAndDoesNotAskItAnything(t *testing.T) {
	fake := signedIn()
	fake.Missing = map[string]bool{"gh": true}
	report := Local(context.Background(), fake)
	if got := statusOf(t, report.Checks, "gh"); got != Fail {
		t.Errorf("gh = %s, want fail", got)
	}
	for _, c := range report.Checks {
		if c.ID == "gh-sign-in" {
			t.Error("the sign-in was checked although gh is not installed")
		}
	}
	for _, call := range fake.Calls {
		if call.Name == "gh" {
			t.Errorf("ran gh %q although it is not installed", call.Args)
		}
	}
}

func TestLocalReportsASignedOutGhWithTheCommandThatFixesIt(t *testing.T) {
	fake := signedIn()
	fake.Results["gh auth status --hostname github.com"] = execx.Result{ExitCode: 1, Stderr: "You are not logged into any GitHub hosts.\n"}
	report := Local(context.Background(), fake)
	if !report.Failed() {
		t.Fatal("a signed-out gh did not fail the report")
	}
	for _, c := range report.Checks {
		if c.ID == "gh-sign-in" && c.Fix != "Run: gh auth login" {
			t.Errorf("fix = %q, want the command that signs in", c.Fix)
		}
	}
}

func TestHostThatCannotBeReachedIsOneFailedCheckNamingTheDestination(t *testing.T) {
	fake := &execxtest.Fake{Results: map[string]execx.Result{"ssh": {ExitCode: 255, Stderr: "ssh: connect to host 10.0.0.5 port 22: Connection timed out\n"}}}
	report := Host(context.Background(), fake, "deploy@10.0.0.5")
	if len(report.Checks) != 1 || report.Checks[0].Status != Fail {
		t.Fatalf("checks = %+v, want one failed check", report.Checks)
	}
	check := report.Checks[0]
	if !strings.Contains(check.Detail, "Connection timed out") {
		t.Errorf("detail = %q, want the reason ssh gave", check.Detail)
	}
	if !strings.Contains(check.Fix, "ssh deploy@10.0.0.5") {
		t.Errorf("fix = %q, want the command to try by hand", check.Fix)
	}
	if report.Facts != nil {
		t.Errorf("facts = %+v, want none for a host that was not reached", report.Facts)
	}
}

func TestHostReportCarriesTheMeasuredFacts(t *testing.T) {
	out := "sentinel-probe-begin\nos_id=debian\nos_version_id=12\nkernel=Linux\narch=aarch64\nprivileges=root\nsystemd=yes\nmem_total_kb=8000000\ncpus=4\ndisk_var_avail_kb=100\ndocker=yes\nkvm=no\nsentinel-probe-end\n"
	fake := &execxtest.Fake{Results: map[string]execx.Result{"ssh": {Stdout: out}}}
	report := Host(context.Background(), fake, "build-box")
	if report.Failed() {
		t.Fatalf("checks = %+v, want no failure", report.Checks)
	}
	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Target string
		Facts  hostprobe.Facts
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("the JSON report does not parse: %v\n%s", err, buf.String())
	}
	if decoded.Target != "build-box" || decoded.Facts.OSID != "debian" || decoded.Facts.CPUs != 4 {
		t.Errorf("decoded = %+v, want the target and the facts", decoded)
	}
}

func TestWriteText(t *testing.T) {
	report := Report{Target: "This machine", Checks: []Check{
		{ID: "git", Title: "git", Status: OK, Detail: "git version 2.47.0"},
		{ID: "gh-sign-in", Title: "GitHub sign-in", Status: Fail, Detail: "gh is not signed in to github.com", Fix: "Run: gh auth login"},
	}}
	var out bytes.Buffer
	report.WriteText(&out)
	want := "This machine\n" +
		"  ok    git             git version 2.47.0\n" +
		"  FAIL  GitHub sign-in  gh is not signed in to github.com\n" +
		"        fix: Run: gh auth login\n" +
		"\n" +
		"1 check failed.\n"
	if out.String() != want {
		t.Errorf("text report =\n%s\nwant\n%s", out.String(), want)
	}

	report.Checks[1].Status = Warn
	out.Reset()
	report.WriteText(&out)
	if !strings.HasSuffix(out.String(), "All checks passed, with 1 warning.\n") {
		t.Errorf("summary = %q, want the warning counted", out.String())
	}
}
