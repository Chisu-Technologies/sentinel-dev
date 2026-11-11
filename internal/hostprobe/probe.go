// Package hostprobe reads facts about a server over ssh without changing it.
package hostprobe

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

//go:embed probe.sh
var script string

const (
	beginMarker = "sentinel-probe-begin"
	endMarker   = "sentinel-probe-end"
	// sshUnreachable is the status ssh itself exits with when it cannot
	// connect or authenticate. Any other status comes from the remote command.
	sshUnreachable = 255
)

// ErrUnreachable means ssh could not connect to the destination or sign in.
var ErrUnreachable = errors.New("host unreachable over ssh")

// ErrIncomplete means the probe started on the host but did not finish.
var ErrIncomplete = errors.New("host probe did not finish")

// Facts is what the probe learned about a host. Numeric fields are zero when
// the host did not report them.
type Facts struct {
	OSID         string `json:"os_id"`
	OSVersionID  string `json:"os_version_id"`
	Kernel       string `json:"kernel"`
	Arch         string `json:"arch"`
	Privileges   string `json:"privileges"` // "root", "sudo" or "none"
	Systemd      bool   `json:"systemd"`
	MemTotalKB   int64  `json:"mem_total_kb"`
	CPUs         int    `json:"cpus"`
	DiskVarAvail int64  `json:"disk_var_avail_kb"`
	Docker       bool   `json:"docker"`
	KVM          bool   `json:"kvm"`
}

// ValidateDestination rejects an ssh destination that ssh could read as
// something other than a host. A destination that starts with "-" is the
// dangerous case: ssh would parse it as an option, and options such as
// ProxyCommand run local commands.
func ValidateDestination(dest string) error {
	if dest == "" {
		return errors.New("destination is empty; use user@host or a Host from your ssh config")
	}
	if strings.HasPrefix(dest, "-") {
		return fmt.Errorf("destination %q starts with \"-\"", dest)
	}
	host := dest
	if at := strings.LastIndex(dest, "@"); at >= 0 {
		if at == 0 {
			return fmt.Errorf("destination %q has an empty user", dest)
		}
		host = dest[at+1:]
	}
	if host == "" || strings.HasPrefix(host, "-") {
		return fmt.Errorf("destination %q has no valid host", dest)
	}
	for _, r := range dest {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("destination %q contains a space or a control character", dest)
		}
	}
	return nil
}

// sshArgs is the full ssh command line for the probe. The script travels on
// stdin, never in the arguments, so nothing in it meets a shell twice.
func sshArgs(dest string) []string {
	return []string{
		// Never prompt: a missing key must fail, not hang waiting for a password.
		"-o", "BatchMode=yes",
		// First contact records the host key; a changed key still fails.
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-T",
		"--", dest,
		"sh", "-s",
	}
}

// Probe runs the read-only probe on dest and returns its facts.
func Probe(ctx context.Context, runner execx.Runner, dest string) (Facts, error) {
	if err := ValidateDestination(dest); err != nil {
		return Facts{}, err
	}
	res, err := runner.Run(ctx, script, "ssh", sshArgs(dest)...)
	if err != nil {
		return Facts{}, fmt.Errorf("run ssh: %w", err)
	}
	if res.ExitCode == sshUnreachable {
		return Facts{}, fmt.Errorf("%w: %s", ErrUnreachable, lastLine(res.Stderr))
	}
	return Parse(res.Stdout)
}

// Parse reads the probe's output. Lines outside the two markers are ignored,
// because a login banner or a shell profile can print before the probe does.
func Parse(output string) (Facts, error) {
	values := map[string]string{}
	inside, finished := false, false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == beginMarker:
			inside = true
		case line == endMarker && inside:
			finished = true
		case inside && !finished:
			if key, value, ok := strings.Cut(line, "="); ok {
				values[key] = value
			}
		}
	}
	if !finished {
		return Facts{}, ErrIncomplete
	}
	return Facts{
		OSID:         values["os_id"],
		OSVersionID:  values["os_version_id"],
		Kernel:       values["kernel"],
		Arch:         values["arch"],
		Privileges:   values["privileges"],
		Systemd:      values["systemd"] == "yes",
		MemTotalKB:   parseInt(values["mem_total_kb"]),
		CPUs:         int(parseInt(values["cpus"])),
		DiskVarAvail: parseInt(values["disk_var_avail_kb"]),
		Docker:       values["docker"] == "yes",
		KVM:          values["kvm"] == "yes",
	}, nil
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
