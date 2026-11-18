package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installedWithEnv(t *testing.T, d *docker, env string) *Install {
	t.Helper()
	install, _ := newInstall(t, d)
	if err := os.MkdirAll(install.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install.Dir, composeFile), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install.Dir, envFile), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return install
}

func envLine(t *testing.T, install *Install) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(install.Dir, envFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, ModelsSetting+"=") {
			return line
		}
	}
	return ""
}

func TestAModelOnThisMachineIsReachedThroughTheHostNameOfDocker(t *testing.T) {
	cases := map[string]string{
		"http://localhost:11434/v1/":          "http://host.docker.internal:11434/v1",
		"http://127.0.0.1:1234/v1":            "http://host.docker.internal:1234/v1",
		"http://[::1]:8000/v1":                "http://host.docker.internal:8000/v1",
		"http://host.docker.internal:1234":    "http://host.docker.internal:1234",
		"http://192.168.1.40:11434/v1":        "http://192.168.1.40:11434/v1",
		"https://models.example.com/v1":       "https://models.example.com/v1",
		"https://LOCALHOST:8443/openai/v1///": "https://host.docker.internal:8443/openai/v1",
	}
	for raw, want := range cases {
		got, err := ModelURL(raw)
		if err != nil || got != want {
			t.Errorf("ModelURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestPlainHTTPToAnotherMachineOrAnAddressWithExtrasIsRefused(t *testing.T) {
	for _, raw := range []string{
		"http://models.example.com/v1",  // a key must not cross a network in clear
		"http://169.254.169.254/latest", // link-local is not private: cloud metadata
		"ftp://localhost/v1",
		"localhost:11434",
		"http://user:secret@localhost:11434/v1",
		"http://localhost:11434/v1?key=x",
		"http://localhost:11434/v1#x",
	} {
		if got, err := ModelURL(raw); err == nil {
			t.Errorf("ModelURL(%q) = %q, want an error", raw, got)
		}
	}
}

func TestAddingAModelWritesItForComposeAndRestartsOnlyWhatReadsIt(t *testing.T) {
	d := &docker{}
	install := installedWithEnv(t, d, "SENTINEL_MASTER_KEY=k\n")

	address, err := install.AddModel(context.Background(), "ollama", "http://localhost:11434/v1")
	if err != nil {
		t.Fatal(err)
	}
	if address != "http://host.docker.internal:11434/v1" {
		t.Errorf("address = %s", address)
	}
	// Single-quoted, so compose takes the JSON as it is.
	if got := envLine(t, install); got != `SENTINEL_INFERENCE_OPERATOR_ENDPOINTS_JSON='{"ollama":"http://host.docker.internal:11434/v1"}'` {
		t.Errorf(".env line = %s", got)
	}
	if d.ran("up -d --no-deps --wait api broker") != 1 {
		t.Errorf("the API and the broker were not restarted alone:\n%s", strings.Join(d.calls, "\n"))
	}
	data, _ := os.ReadFile(filepath.Join(install.Dir, envFile))
	if !strings.HasPrefix(string(data), "SENTINEL_MASTER_KEY=k\n") {
		t.Errorf("the other settings changed:\n%s", data)
	}
}

func TestModelsAreReadBackReplacedAndRemoved(t *testing.T) {
	d := &docker{}
	install := installedWithEnv(t, d, `SENTINEL_INFERENCE_OPERATOR_ENDPOINTS_JSON='{"lmstudio":"http://host.docker.internal:1234/v1"}'`+"\n")
	ctx := context.Background()

	if _, err := install.AddModel(ctx, "ollama", "http://localhost:11434/v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := install.AddModel(ctx, "ollama", "http://localhost:11500/v1"); err != nil {
		t.Fatal(err)
	}
	models, err := install.Models()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models["ollama"] != "http://host.docker.internal:11500/v1" || models["lmstudio"] != "http://host.docker.internal:1234/v1" {
		t.Errorf("models = %v", models)
	}
	if err := install.RemoveModel(ctx, "lmstudio"); err != nil {
		t.Fatal(err)
	}
	if err := install.RemoveModel(ctx, "ollama"); err != nil {
		t.Fatal(err)
	}
	if got := envLine(t, install); got != ModelsSetting+"=" {
		t.Errorf("the last removal left %s", got)
	}
	if err := install.RemoveModel(ctx, "ollama"); err == nil {
		t.Error("removing a model that is not set did not fail")
	}
}

func TestAModelIsRefusedBeforeAnythingChanges(t *testing.T) {
	d := &docker{}
	install := installedWithEnv(t, d, `SENTINEL_INFERENCE_OPERATOR_ENDPOINTS_JSON='{"ollama":"http://host.docker.internal:11434/v1"}'`+"\n")
	ctx := context.Background()
	before := envLine(t, install)

	for _, bad := range [][2]string{
		{"Ollama", "http://localhost:11434/v1"},    // the panel shows lowercase names
		{"twin", "http://127.0.0.1:11434/v1"},      // the same server under two names
		{"remote", "http://models.example.com/v1"}, // clear text off this machine
	} {
		if _, err := install.AddModel(ctx, bad[0], bad[1]); err == nil {
			t.Errorf("AddModel(%q, %q) did not fail", bad[0], bad[1])
		}
	}
	if envLine(t, install) != before || len(d.calls) != 0 {
		t.Errorf("a refused model changed the installation:\n%s", strings.Join(d.calls, "\n"))
	}
}
