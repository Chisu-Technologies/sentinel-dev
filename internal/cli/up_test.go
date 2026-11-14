package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// fakeDocker answers like a machine with Docker running and a fresh
// installation: the first start of the API stops at the key check, and the
// owner tool prints its JSON line.
type fakeDocker struct {
	missing bool
	calls   []string
}

func (f *fakeDocker) LookPath(name string) (string, error) {
	if f.missing {
		return "", exec.ErrNotFound
	}
	return "/usr/bin/" + name, nil
}

func (f *fakeDocker) Run(_ context.Context, cmd execx.Command) (execx.Result, error) {
	line := strings.Join(append([]string{cmd.Name}, cmd.Args...), " ")
	f.calls = append(f.calls, line)
	switch {
	case line == "gh auth token --hostname github.com":
		return execx.Result{Stdout: "gho_" + strings.Repeat("a", 36) + "\n"}, nil
	case strings.Contains(line, "open_edition_github"):
		return execx.Result{Stdout: `{"login": "you", "result": "ok"}` + "\n"}, nil
	case strings.Contains(line, "run --rm --no-deps api"):
		return execx.Result{ExitCode: 3, Stderr: "master_key_admission_marker_missing"}, nil
	case strings.Contains(line, "open_edition_owner"):
		return execx.Result{Stdout: `{"created": "yes", "email": "you@example.com", "result": "ok", "sign_in_url": "http://localhost:8080/api/auth/verify?token=t0k3n"}` + "\n"}, nil
	}
	return execx.Result{}, nil
}

func upCommand(t *testing.T, docker *fakeDocker, home string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: docker,
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string {
			if name == "SENTINEL_HOME" {
				return home
			}
			return ""
		},
		Bundle: fstest.MapFS{"compose.yaml": {Data: []byte("services: {}\n")}},
		Rand:   bytes.NewReader(bytes.Repeat([]byte{5}, 1024)),
	})
	return result{code, stdout.String(), stderr.String()}
}

func TestUpPrintsThePanelAndASignInLinkTheFirstTime(t *testing.T) {
	home := filepath.Join(t.TempDir(), "local")
	got := upCommand(t, &fakeDocker{}, home, "up", "--email", "you@example.com")

	if got.code != ExitOK {
		t.Fatalf("got %+v", got)
	}
	for _, want := range []string{"Sentinel is running: http://localhost:8080", "Owner: you@example.com", "http://localhost:8080/api/auth/verify?token=t0k3n"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", got.stdout, want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "compose.yaml")); err != nil {
		t.Errorf("the installation is not in SENTINEL_HOME: %v", err)
	}

	again := upCommand(t, &fakeDocker{}, home, "up")
	if again.code != ExitOK || strings.Contains(again.stdout, "token=") || !strings.Contains(again.stdout, "sentinel up --sign-in") {
		t.Errorf("second run: %+v", again)
	}
}

func TestUpWithoutAnEmailTheFirstTimeSaysWhatToRun(t *testing.T) {
	got := upCommand(t, &fakeDocker{}, filepath.Join(t.TempDir(), "local"), "up")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "sentinel up --email you@example.com") {
		t.Errorf("got %+v", got)
	}
}

func TestUpAndDownSayWhenDockerIsMissing(t *testing.T) {
	for _, name := range []string{"up", "down"} {
		got := upCommand(t, &fakeDocker{missing: true}, filepath.Join(t.TempDir(), "local"), name, "--dir", t.TempDir())
		if got.code != ExitFailed || !strings.Contains(got.stderr, "Docker is not installed") {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestUpAndDownRefuseArguments(t *testing.T) {
	for _, args := range [][]string{{"up", "now"}, {"down", "all"}, {"up", "--nope"}} {
		if got := upCommand(t, &fakeDocker{}, t.TempDir(), args...); got.code != ExitUsage {
			t.Errorf("sentinel %q = %+v, want status 2", args, got)
		}
	}
}

func TestDownStopsWithoutRemovingData(t *testing.T) {
	home := filepath.Join(t.TempDir(), "local")
	if got := upCommand(t, &fakeDocker{}, home, "up", "--email", "you@example.com"); got.code != ExitOK {
		t.Fatalf("up: %+v", got)
	}
	docker := &fakeDocker{}
	got := upCommand(t, docker, home, "down")
	if got.code != ExitOK || !strings.Contains(got.stdout, "Its data stays") {
		t.Errorf("got %+v", got)
	}
	for _, call := range docker.calls {
		if strings.Contains(call, " down") || strings.Contains(call, " -v") || strings.Contains(call, " rm") {
			t.Errorf("down ran something that removes data: %s", call)
		}
	}
}

// On Windows the user configuration folder is under AppData, which Docker
// Desktop did not see; the installation lives in the home folder instead.
func TestTheInstallationFolderIsInTheHomeFolderNotTheConfigurationFolder(t *testing.T) {
	dir, err := localDir(Env{})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".sentinel", "local"); dir != want {
		t.Errorf("localDir = %q, want %q", dir, want)
	}
	if config, err := os.UserConfigDir(); err == nil && strings.HasPrefix(dir, config) && config != home {
		t.Errorf("localDir %q is inside the configuration folder %q", dir, config)
	}
	if got, _ := localDir(Env{Getenv: func(string) string { return "/somewhere/else" }}); got != "/somewhere/else" {
		t.Errorf("SENTINEL_HOME ignored: %q", got)
	}
}
