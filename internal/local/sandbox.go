package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// sandboxRuntimeSetting is the Docker runtime of the containers the agent's
// commands run in. runc shares this machine's kernel; Kata gives each command
// its own microVM where Docker has Kata installed.
const sandboxRuntimeSetting = "SENTINEL_SANDBOX_RUNTIME"

// sandboxImageSetting is the image those containers start from; the check
// starts one, so it fails here and not on the agent's first command.
const sandboxImageSetting = "SENTINEL_SANDBOX_PYTHON_IMAGE"

// checkSandboxRuntime makes sure the runtime the .env asks for can start a
// container, before Sentinel starts: a runtime that cannot would fail every
// command of every run. When it cannot, this run uses runc and says why; the
// .env is left as the user wrote it. With runc and Kata installed, it says how
// to turn Kata on, and changes nothing.
func (i *Install) checkSandboxRuntime(ctx context.Context) error {
	wanted, err := i.envSetting(sandboxRuntimeSetting)
	if err != nil {
		return err
	}
	runtimes := i.dockerRuntimes(ctx)
	if wanted == "" || wanted == "runc" {
		for _, name := range runtimes {
			if strings.HasPrefix(name, "kata") {
				i.say("Docker here has the Kata runtime %q. To run each command of the agent in its own microVM "+
					"(about a second more per command), add %s=%s to %s and run this again.",
					name, sandboxRuntimeSetting, name, filepath.Join(i.Dir, envFile))
				break
			}
		}
		return nil
	}
	reason := ""
	if !slices.Contains(runtimes, wanted) {
		reason = fmt.Sprintf("Docker here has no runtime %q (it has %s)", wanted, strings.Join(runtimes, ", "))
	} else if image := i.sandboxImage(ctx); image == "" {
		reason = "the image of the agent's containers could not be read from the bundle"
	} else if res, err := i.Runner.Run(ctx, execx.Command{Name: "docker", Args: []string{"run", "--rm", "--runtime=" + wanted, image, "true"}}); err != nil || res.ExitCode != 0 {
		reason = fmt.Sprintf("a test container with the runtime %q did not start: %s", wanted, tail(res.Stderr+res.Stdout))
	}
	if reason == "" {
		i.say("The agent's commands run with the Docker runtime %q.", wanted)
		return nil
	}
	i.say("%s=%s in %s: %s. This run uses runc.", sandboxRuntimeSetting, wanted, filepath.Join(i.Dir, envFile), reason)
	// The shell's environment wins over the --env-file in Docker Compose.
	i.composeEnv = append(i.composeEnv, sandboxRuntimeSetting+"=runc")
	return nil
}

// dockerRuntimes lists the runtimes the Docker daemon knows, sorted. Nothing
// when it cannot be read: then only runc is assumed.
func (i *Install) dockerRuntimes(ctx context.Context) []string {
	res, err := i.Runner.Run(ctx, execx.Command{Name: "docker", Args: []string{"info", "--format", "{{json .Runtimes}}"}})
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	var known map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &known) != nil {
		return nil
	}
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sandboxImage is the image of the agent's containers as the bundle resolves
// it, with the .env applied: what Compose itself would pass to the worker.
func (i *Install) sandboxImage(ctx context.Context) string {
	res, err := i.Runner.Run(ctx, i.compose("config", "--format", "json"))
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]*string `json:"environment"`
		} `json:"services"`
	}
	if json.Unmarshal([]byte(res.Stdout), &config) != nil {
		return ""
	}
	for _, service := range config.Services {
		if value := service.Environment[sandboxImageSetting]; value != nil && *value != "" {
			return *value
		}
	}
	return ""
}

// envSetting is the value of one setting of the .env, empty when it is not there.
func (i *Install) envSetting(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	if err != nil {
		return "", err
	}
	value := ""
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), name+"="); ok {
			value = strings.TrimSpace(v)
		}
	}
	return value, nil
}
