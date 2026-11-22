package local

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

var pythonImage = "python@sha256:" + strings.Repeat("5", 64)

// runtimesDocker answers `docker info` with these runtimes, Compose's config
// with the sandbox image, and the test container with probe.
func runtimesDocker(runtimes string, probe execx.Result) *docker {
	return &docker{rules: []rule{
		{contains: "info --format {{json .Runtimes}}", result: execx.Result{Stdout: runtimes + "\n"}},
		{contains: "config --format json", result: execx.Result{Stdout: `{"services": {"worker-sandbox": {"environment": {"SENTINEL_SANDBOX_PYTHON_IMAGE": "` + pythonImage + `"}}}}`}},
		{contains: "docker run --rm --runtime=", result: probe},
	}}
}

func installWithEnv(t *testing.T, d *docker, env string) (*Install, *strings.Builder) {
	t.Helper()
	install, _ := newInstall(t, d)
	var said strings.Builder
	install.Progress = &said
	if err := os.MkdirAll(install.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install.Dir, envFile), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return install, &said
}

const withKata = `{"io.containerd.runc.v2": {}, "kata": {}, "runc": {}}`

func TestWithRuncAndKataInstalledUpSaysHowToTurnKataOnAndChangesNothing(t *testing.T) {
	d := runtimesDocker(withKata, execx.Result{})
	install, said := installWithEnv(t, d, "SENTINEL_DB_PASSWORD=x\n")

	if err := install.checkSandboxRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said.String(), `Docker here has the Kata runtime "kata"`) ||
		!strings.Contains(said.String(), "SENTINEL_SANDBOX_RUNTIME=kata") {
		t.Errorf("said %q", said.String())
	}
	if d.ran("docker run") != 0 || install.composeEnv != nil {
		t.Errorf("calls %v, env %v", d.calls, install.composeEnv)
	}
}

func TestWithRuncAndNoKataUpSaysNothing(t *testing.T) {
	d := runtimesDocker(`{"runc": {}, "runsc": {}}`, execx.Result{})
	install, said := installWithEnv(t, d, "SENTINEL_SANDBOX_RUNTIME=runc\n")

	if err := install.checkSandboxRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if said.String() != "" || install.composeEnv != nil {
		t.Errorf("said %q, env %v", said.String(), install.composeEnv)
	}
}

func TestARuntimeThatStartsATestContainerIsUsed(t *testing.T) {
	d := runtimesDocker(withKata, execx.Result{})
	install, said := installWithEnv(t, d, "SENTINEL_SANDBOX_RUNTIME=kata\n")

	if err := install.checkSandboxRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(d.calls, "docker run --rm --runtime=kata "+pythonImage+" true") {
		t.Errorf("calls %v", d.calls)
	}
	if !strings.Contains(said.String(), `run with the Docker runtime "kata"`) || install.composeEnv != nil {
		t.Errorf("said %q, env %v", said.String(), install.composeEnv)
	}
}

func TestARuntimeDockerDoesNotHaveFallsBackToRuncForThisRunOnly(t *testing.T) {
	d := runtimesDocker(`{"runc": {}}`, execx.Result{})
	install, said := installWithEnv(t, d, "SENTINEL_SANDBOX_RUNTIME=kata\n")

	if err := install.checkSandboxRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said.String(), `Docker here has no runtime "kata" (it has runc). This run uses runc.`) {
		t.Errorf("said %q", said.String())
	}
	if d.ran("docker run") != 0 {
		t.Errorf("a test container was started: %v", d.calls)
	}
	// Compose gets runc for this run; the .env keeps what the user wrote.
	if cmd := install.compose("up", "-d"); !slices.Contains(cmd.Env, "SENTINEL_SANDBOX_RUNTIME=runc") {
		t.Errorf("compose env %v", cmd.Env)
	}
	if value, _ := install.envSetting(sandboxRuntimeSetting); value != "kata" {
		t.Errorf(".env now says %q", value)
	}
}

func TestARuntimeWhoseTestContainerFailsFallsBackToRunc(t *testing.T) {
	d := runtimesDocker(withKata, execx.Result{ExitCode: 125, Stderr: "failed to create shim task: vsock: ENOSYS"})
	install, said := installWithEnv(t, d, "SENTINEL_SANDBOX_RUNTIME=kata\n")

	if err := install.checkSandboxRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said.String(), "did not start: failed to create shim task: vsock: ENOSYS. This run uses runc.") {
		t.Errorf("said %q", said.String())
	}
	if !slices.Contains(install.compose("up").Env, "SENTINEL_SANDBOX_RUNTIME=runc") {
		t.Errorf("compose env %v", install.compose("up").Env)
	}
}

func TestUpChecksTheRuntimeAfterTheImagesAndBeforeStarting(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	d.rules = append([]rule{{contains: "info --format {{json .Runtimes}}", result: execx.Result{Stdout: withKata}}}, d.rules...)
	install, progress := newInstall(t, d)

	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	pull, check, start := -1, -1, -1
	for n, call := range d.calls {
		switch {
		case strings.Contains(call, " pull --quiet"):
			pull = n
		case strings.Contains(call, "{{json .Runtimes}}"):
			check = n
		case strings.Contains(call, "up -d --wait --wait-timeout"):
			start = n
		}
	}
	if !(pull < check && check < start) {
		t.Errorf("pull %d, check %d, start %d: %v", pull, check, start, d.calls)
	}
	if !strings.Contains(progress.String(), `Docker here has the Kata runtime "kata"`) {
		t.Errorf("progress %q", progress.String())
	}
}
