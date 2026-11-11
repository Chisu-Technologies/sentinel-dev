package hostprobe

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

const ubuntuOutput = `Welcome to Ubuntu 24.04 LTS
Last login: today
sentinel-probe-begin
probe_version=1
os_id=ubuntu
os_version_id=24.04
kernel=Linux
arch=x86_64
uid=1000
privileges=sudo
systemd=yes
mem_total_kb=4005120
cpus=2
disk_var_avail_kb=52428800
docker=no
kvm=yes
sentinel-probe-end
`

func TestValidateDestination(t *testing.T) {
	valid := []string{"deploy@10.0.0.5", "build-box", "ops@server.example.com", "user@[2001:db8::1]"}
	for _, dest := range valid {
		if err := ValidateDestination(dest); err != nil {
			t.Errorf("ValidateDestination(%q) = %v, want nil", dest, err)
		}
	}
	invalid := []string{
		"",
		"-oProxyCommand=touch /tmp/owned",
		"-l",
		// Starts with "-" and still has a plausible host after the "@".
		"-oProxyCommand=id@server.example.com",
		"user@-oProxyCommand=id",
		"@host",
		"user@",
		"user@host name",
		"user@host\nother",
		"user@host\x00",
	}
	for _, dest := range invalid {
		if err := ValidateDestination(dest); err == nil {
			t.Errorf("ValidateDestination(%q) = nil, want an error", dest)
		}
	}
}

func TestProbeSendsTheScriptOnStdinAndFencesTheDestination(t *testing.T) {
	fake := &execxtest.Fake{Results: map[string]execx.Result{"ssh": {Stdout: ubuntuOutput}}}

	facts, err := Probe(context.Background(), fake, "deploy@10.0.0.5")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("ran %d programs, want exactly one ssh", len(fake.Calls))
	}
	call := fake.Calls[0]
	wantArgs := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-T",
		"--", "deploy@10.0.0.5",
		"sh", "-s",
	}
	if call.Name != "ssh" || !reflect.DeepEqual(call.Args, wantArgs) {
		t.Errorf("ran %s %q, want ssh %q", call.Name, call.Args, wantArgs)
	}
	if call.Stdin != script {
		t.Error("the probe script did not travel on stdin")
	}
	for _, arg := range call.Args {
		if strings.Contains(arg, "sentinel-probe") {
			t.Errorf("the probe script leaked into the arguments: %q", arg)
		}
	}
	want := Facts{
		OSID: "ubuntu", OSVersionID: "24.04", Kernel: "Linux", Arch: "x86_64",
		Privileges: "sudo", Systemd: true, MemTotalKB: 4005120, CPUs: 2,
		DiskVarAvail: 52428800, Docker: false, KVM: true,
	}
	if facts != want {
		t.Errorf("facts = %+v, want %+v", facts, want)
	}
}

// ssh reads a destination that starts with "-" as an option, and
// -oProxyCommand runs a local command. Such a destination must never reach it.
func TestProbeRefusesAnOptionLikeDestinationWithoutRunningAnything(t *testing.T) {
	fake := &execxtest.Fake{}
	_, err := Probe(context.Background(), fake, "-oProxyCommand=id")
	if err == nil {
		t.Fatal("Probe accepted a destination that ssh would parse as an option")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("Probe ran %+v before refusing the destination", fake.Calls)
	}
}

func TestProbeReportsAnUnreachableHostWithTheReasonSSHGave(t *testing.T) {
	fake := &execxtest.Fake{Results: map[string]execx.Result{"ssh": {
		ExitCode: 255,
		Stderr:   "Warning: Permanently added the host key.\ndeploy@10.0.0.5: Permission denied (publickey).\n",
	}}}
	_, err := Probe(context.Background(), fake, "deploy@10.0.0.5")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if !strings.Contains(err.Error(), "Permission denied (publickey).") {
		t.Errorf("err = %q, want it to carry the last line ssh printed", err)
	}
}

func TestProbeReportsWhenSSHCannotBeStarted(t *testing.T) {
	fake := &execxtest.Fake{Errors: map[string]error{"ssh": errors.New("executable file not found")}}
	if _, err := Probe(context.Background(), fake, "build-box"); err == nil {
		t.Fatal("Probe hid the failure to start ssh")
	}
}

func TestParse(t *testing.T) {
	t.Run("ignores what the login shell prints around the probe", func(t *testing.T) {
		facts, err := Parse("os_id=fake\n" + ubuntuOutput + "os_id=also-fake\n")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if facts.OSID != "ubuntu" {
			t.Errorf("OSID = %q, want the value between the markers", facts.OSID)
		}
	})
	t.Run("accepts CRLF line endings", func(t *testing.T) {
		facts, err := Parse(strings.ReplaceAll(ubuntuOutput, "\n", "\r\n"))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if facts.Arch != "x86_64" || !facts.KVM {
			t.Errorf("facts = %+v, want the same values as with LF", facts)
		}
	})
	t.Run("a probe cut short is an error, not a host with empty facts", func(t *testing.T) {
		cut := ubuntuOutput[:strings.Index(ubuntuOutput, "mem_total_kb")]
		if _, err := Parse(cut); !errors.Is(err, ErrIncomplete) {
			t.Errorf("err = %v, want ErrIncomplete", err)
		}
	})
	t.Run("no probe output at all is incomplete", func(t *testing.T) {
		if _, err := Parse("bash: sh: command not found\n"); !errors.Is(err, ErrIncomplete) {
			t.Errorf("err = %v, want ErrIncomplete", err)
		}
	})
	t.Run("a value that is not a number counts as not reported", func(t *testing.T) {
		facts, err := Parse("sentinel-probe-begin\nmem_total_kb=\ncpus=many\ndisk_var_avail_kb=-5\nsentinel-probe-end\n")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if facts.MemTotalKB != 0 || facts.CPUs != 0 || facts.DiskVarAvail != 0 {
			t.Errorf("facts = %+v, want zeros", facts)
		}
	})
}

// The script is what actually runs on a server, so it is run for real where a
// POSIX shell and /proc exist.
func TestTheScriptRunsOnThisMachine(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the probe targets Linux servers")
	}
	res, err := execx.OS{}.Run(context.Background(), execx.Command{Name: "sh", Args: []string{"-s"}, Stdin: script})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("sh -s: err=%v exit=%d stderr=%q", err, res.ExitCode, res.Stderr)
	}
	facts, err := Parse(res.Stdout)
	if err != nil {
		t.Fatalf("Parse(%q): %v", res.Stdout, err)
	}
	if facts.Kernel != "Linux" || facts.Arch == "" || facts.MemTotalKB == 0 || facts.CPUs == 0 {
		t.Errorf("facts = %+v, want kernel, architecture, memory and CPUs reported", facts)
	}
	if facts.Privileges != "root" && facts.Privileges != "sudo" && facts.Privileges != "none" {
		t.Errorf("privileges = %q, want root, sudo or none", facts.Privileges)
	}
}
