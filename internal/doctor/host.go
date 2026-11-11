package doctor

import (
	"context"
	"errors"
	"fmt"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/hostprobe"
)

// supportedOS lists the distributions and releases Sentinel is built for. A
// release that is not listed is refused instead of guessed at.
var supportedOS = map[string][]string{
	"ubuntu": {"22.04", "24.04"},
	"debian": {"12"},
}

const supportedOSText = "Ubuntu 22.04, Ubuntu 24.04 or Debian 12"

// supportedArch holds the `uname -m` values Sentinel will publish for.
var supportedArch = map[string]bool{"x86_64": true, "aarch64": true, "arm64": true}

// testedMemoryKB is the smallest machine Sentinel is sized for: 4 GB. Such a
// machine reports a little less than 4 GiB as MemTotal, because the kernel
// keeps some for itself, so the line is drawn at 3.5 GiB.
const testedMemoryKB = 3_670_016

// Host probes a server over ssh and checks it against what Sentinel needs. It
// changes nothing on the server.
func Host(ctx context.Context, runner execx.Runner, dest string) Report {
	report := Report{Target: dest}
	facts, err := hostprobe.Probe(ctx, runner, dest)
	if err != nil {
		report.Checks = []Check{unreached(dest, err)}
		return report
	}
	report.Facts = facts
	report.Checks = append(report.Checks, Check{
		ID: "ssh", Title: "ssh", Status: OK, Detail: "connected without a password prompt",
	})
	report.Checks = append(report.Checks, Evaluate(facts)...)
	return report
}

func unreached(dest string, err error) Check {
	check := Check{ID: "ssh", Title: "ssh", Status: Fail, Detail: err.Error()}
	switch {
	case errors.Is(err, hostprobe.ErrUnreachable):
		check.Fix = fmt.Sprintf("Check that `ssh %s true` works from this machine without asking for a password: the address, an ssh key, or an entry in ~/.ssh/config. Then run doctor again.", dest)
	case errors.Is(err, hostprobe.ErrIncomplete):
		check.Fix = "The server did not run the probe to the end. Check that `sh` exists there and that nothing in the login shell ends the session."
	default:
		check.Fix = "Use user@host, or a Host from your ssh config."
	}
	return check
}

// Evaluate turns the facts of a host into checks.
func Evaluate(f hostprobe.Facts) []Check {
	return []Check{osCheck(f), archCheck(f), privilegesCheck(f), systemdCheck(f), memoryCheck(f), dockerCheck(f)}
}

func osCheck(f hostprobe.Facts) Check {
	check := Check{ID: "os", Title: "operating system", Detail: f.OSID + " " + f.OSVersionID}
	for _, release := range supportedOS[f.OSID] {
		if release == f.OSVersionID {
			check.Status = OK
			return check
		}
	}
	check.Status = Fail
	check.Detail += " is not supported"
	check.Fix = "Use a server with " + supportedOSText + "."
	return check
}

func archCheck(f hostprobe.Facts) Check {
	check := Check{ID: "arch", Title: "architecture", Detail: f.Arch}
	if supportedArch[f.Arch] {
		check.Status = OK
		return check
	}
	check.Status = Fail
	check.Detail += " is not supported"
	check.Fix = "Use an x86_64 or arm64 server."
	return check
}

func privilegesCheck(f hostprobe.Facts) Check {
	check := Check{ID: "privileges", Title: "privileges"}
	switch f.Privileges {
	case "root":
		check.Status, check.Detail = OK, "signed in as root"
	case "sudo":
		check.Status, check.Detail = OK, "sudo works without a password"
	default:
		check.Status, check.Detail = Fail, "neither root nor sudo without a password"
		check.Fix = "Sign in as root, or as a user for whom `sudo -n true` succeeds. Installing needs it."
	}
	return check
}

func systemdCheck(f hostprobe.Facts) Check {
	check := Check{ID: "systemd", Title: "systemd"}
	if f.Systemd {
		check.Status, check.Detail = OK, "running"
		return check
	}
	check.Status, check.Detail = Fail, "not running"
	check.Fix = "Use a server that boots with systemd. A container without an init system will not do."
	return check
}

func memoryCheck(f hostprobe.Facts) Check {
	check := Check{ID: "memory", Title: "memory", Detail: fmt.Sprintf("%.1f GiB", float64(f.MemTotalKB)/(1<<20))}
	if f.MemTotalKB >= testedMemoryKB {
		check.Status = OK
		return check
	}
	check.Status = Warn
	check.Detail += ", below the 4 GB Sentinel is sized for"
	check.Fix = "Use a server with at least 4 GB of memory."
	return check
}

func dockerCheck(f hostprobe.Facts) Check {
	check := Check{ID: "docker", Title: "Docker"}
	if f.Docker {
		check.Status, check.Detail = OK, "installed"
		return check
	}
	check.Status, check.Detail = Warn, "not installed"
	check.Fix = "Install Docker Engine: https://docs.docker.com/engine/install/"
	return check
}
