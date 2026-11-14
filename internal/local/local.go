// Package local brings Sentinel up on this machine, with Docker, from a bundle
// embedded in the command: a compose file that pulls published images, and the
// files it mounts.
//
// Everything it keeps lives in one directory: the bundle, the secrets of this
// installation (generated here, never printed) and two markers. Running it again
// updates the bundle and starts what is stopped; it never regenerates a secret.
package local

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

const (
	// Project is the compose project name: one installation per machine.
	Project = "sentinel"
	// PanelURL is where the panel answers on this machine.
	PanelURL = "http://localhost:8080"

	envFile        = ".env"
	admissionMark  = "admission.done"
	ownerMark      = "owner"
	composeFile    = "compose.yaml"
	waitTimeoutSec = "900"
)

// The six secrets of an installation, and how each is made.
var secrets = []struct {
	name string
	make func(io.Reader) (string, error)
}{
	{"SENTINEL_DB_PASSWORD", token(24)},
	{"SENTINEL_MASTER_KEY", fernetKey},
	{"SENTINEL_ADMIN_KEY", token(32)},
	{"SENTINEL_ARCHITECT_HMAC_KEY", token(48)},
	{"SENTINEL_SIGNING_KEY", token(48)},
}

var emailPattern = regexp.MustCompile(`^[^@\s]{1,64}@[^@\s]{1,255}$`)

// Install is one installation on this machine.
type Install struct {
	Runner execx.Runner
	// Dir holds everything the installation keeps.
	Dir string
	// Bundle is the compose file and the files it mounts.
	Bundle fs.FS
	// Rand is the source of the secrets. Nil means crypto/rand.
	Rand io.Reader
	// Progress receives one line per step.
	Progress io.Writer
}

// Result is what `up` reports at the end.
type Result struct {
	PanelURL string
	// SignInURL is a one-time link for the owner, empty when none was asked for.
	SignInURL string
	// Owner is the email of the owner of this installation.
	Owner string
	// Created is true when this run created the owner.
	Created bool
}

// ErrDockerMissing and ErrDockerStopped are the two ways Docker is not ready.
var (
	ErrDockerMissing = errors.New("Docker is not installed. Install Docker Desktop (Windows, macOS) or Docker Engine (Linux), then run this again")
	ErrDockerStopped = errors.New("Docker is installed but not running. Start Docker, then run this again")
	// ErrDockerDenied is a Linux user outside the docker group: the daemon runs,
	// and this user may not talk to it.
	ErrDockerDenied = errors.New("your user may not use Docker. Run: sudo usermod -aG docker $USER, then log out and in again, and run this again")
	ErrNeedEmail    = errors.New("the first run needs the email of the owner: sentinel up --email you@example.com")
)

func (i *Install) say(format string, a ...any) {
	if i.Progress != nil {
		fmt.Fprintf(i.Progress, format+"\n", a...)
	}
}

func (i *Install) compose(args ...string) execx.Command {
	base := []string{"compose", "-p", Project, "--env-file", filepath.Join(i.Dir, envFile), "-f", filepath.Join(i.Dir, composeFile)}
	return execx.Command{Name: "docker", Args: append(base, args...), Dir: i.Dir}
}

// run runs a command and turns a non-zero exit into an error that carries the
// end of what the program printed.
func (i *Install) run(ctx context.Context, what string, cmd execx.Command) (execx.Result, error) {
	res, err := i.Runner.Run(ctx, cmd)
	if err != nil {
		return res, fmt.Errorf("%s: %w", what, err)
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("%s failed (exit %d): %s", what, res.ExitCode, tail(res.Stderr+res.Stdout))
	}
	return res, nil
}

// CheckDocker reports whether Docker and its compose plugin can be used.
func (i *Install) CheckDocker(ctx context.Context) error {
	if _, err := i.Runner.LookPath("docker"); err != nil {
		return ErrDockerMissing
	}
	if res, err := i.Runner.Run(ctx, execx.Command{Name: "docker", Args: []string{"compose", "version"}}); err != nil || res.ExitCode != 0 {
		return errors.New("Docker Compose v2 is not available. Update Docker, then run this again")
	}
	if res, err := i.Runner.Run(ctx, execx.Command{Name: "docker", Args: []string{"info", "--format", "{{.ServerVersion}}"}}); err != nil || res.ExitCode != 0 {
		if strings.Contains(strings.ToLower(res.Stderr+res.Stdout), "permission denied") {
			return ErrDockerDenied
		}
		return ErrDockerStopped
	}
	return nil
}

// Up brings the installation up. email is needed the first time, to create the
// owner; signIn asks for a fresh sign-in link on a later run.
func (i *Install) Up(ctx context.Context, email string, signIn bool) (Result, error) {
	result := Result{PanelURL: PanelURL}
	if err := i.CheckDocker(ctx); err != nil {
		return result, err
	}
	owner, _ := i.readMark(ownerMark)
	email = strings.TrimSpace(strings.ToLower(email))
	if owner == "" && email == "" {
		return result, ErrNeedEmail
	}
	if email != "" && !emailPattern.MatchString(email) {
		return result, errors.New("that is not an email address")
	}
	if owner != "" && email != "" && email != owner {
		return result, fmt.Errorf("this installation already has an owner, %s", owner)
	}

	if err := os.MkdirAll(i.Dir, 0o700); err != nil {
		return result, err
	}
	if err := i.writeBundle(); err != nil {
		return result, fmt.Errorf("writing the bundle: %w", err)
	}
	created, err := i.ensureSecrets()
	if err != nil {
		return result, fmt.Errorf("writing the secrets: %w", err)
	}
	if created {
		i.say("Created the secrets of this installation in %s", filepath.Join(i.Dir, envFile))
	}

	i.say("Downloading the images (the first time takes a few minutes)")
	if _, err := i.run(ctx, "downloading the images", i.compose("pull", "--quiet")); err != nil {
		return result, err
	}

	if done, _ := i.readMark(admissionMark); done == "" {
		i.say("Preparing the database")
		if _, err := i.run(ctx, "starting the database", i.compose("up", "-d", "--wait", "postgres", "temporal")); err != nil {
			return result, err
		}
		// The first start of the API creates the schema and then stops at the
		// key check, which is the next step. That stop is expected.
		res, err := i.Runner.Run(ctx, i.compose("run", "--rm", "--no-deps", "api"))
		if err != nil {
			return result, fmt.Errorf("creating the database schema: %w", err)
		}
		if res.ExitCode != 0 && !strings.Contains(res.Stdout+res.Stderr, "master_key_admission_marker_missing") {
			return result, fmt.Errorf("creating the database schema failed (exit %d): %s", res.ExitCode, tail(res.Stderr+res.Stdout))
		}
		// The key goes from the container's own environment to a file inside
		// that container: it never passes through this program's arguments.
		init := "umask 077; printf %s \"$SENTINEL_MASTER_KEY\" > /tmp/master-current.key; " +
			"exec python -m tools.operator.master_key_rotation initialize-admission " +
			"--key-file /tmp/master-current.key --generation 1 --operator sentinel-up " +
			"--operation-id sentinel-up-admission-1 --acknowledge-maintenance"
		if _, err := i.run(ctx, "admitting the master key", i.compose("run", "--rm", "--no-deps", "--entrypoint", "sh", "api", "-euc", init)); err != nil {
			return result, err
		}
		if err := i.writeMark(admissionMark, "1"); err != nil {
			return result, err
		}
	}

	i.say("Starting Sentinel")
	if _, err := i.run(ctx, "starting Sentinel", i.compose("up", "-d", "--wait", "--wait-timeout", waitTimeoutSec)); err != nil {
		return result, err
	}

	if owner == "" || signIn {
		who := owner
		if who == "" {
			who = email
		}
		res, err := i.run(ctx, "creating the sign-in link", i.compose("exec", "-T", "api", "python", "-m", "tools.operator.open_edition_owner", "--email", who))
		if err != nil {
			return result, err
		}
		var answer struct {
			Result    string `json:"result"`
			ErrorCode string `json:"error_code"`
			Email     string `json:"email"`
			Created   string `json:"created"`
			SignInURL string `json:"sign_in_url"`
		}
		if err := json.Unmarshal([]byte(lastLine(res.Stdout)), &answer); err != nil || answer.Result != "ok" {
			if answer.ErrorCode != "" {
				return result, fmt.Errorf("creating the sign-in link failed: %s", answer.ErrorCode)
			}
			return result, errors.New("creating the sign-in link failed: the answer was not understood")
		}
		if !strings.HasPrefix(answer.SignInURL, PanelURL+"/") {
			return result, errors.New("creating the sign-in link failed: the link does not point at this machine")
		}
		if owner == "" {
			if err := i.writeMark(ownerMark, answer.Email); err != nil {
				return result, err
			}
		}
		result.SignInURL = answer.SignInURL
		result.Owner = answer.Email
		result.Created = answer.Created == "yes"
		return result, nil
	}
	result.Owner = owner
	return result, nil
}

// Down stops the installation. Its data stays.
func (i *Install) Down(ctx context.Context) error {
	if err := i.CheckDocker(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(i.Dir, composeFile)); err != nil {
		return errors.New("Sentinel is not installed on this machine")
	}
	_, err := i.run(ctx, "stopping Sentinel", i.compose("stop"))
	return err
}

func (i *Install) writeBundle() error {
	return fs.WalkDir(i.Bundle, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(i.Dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(i.Bundle, path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}

// ensureSecrets writes the secrets once. It reports whether it wrote them.
func (i *Install) ensureSecrets() (bool, error) {
	path := filepath.Join(i.Dir, envFile)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	source := i.randSource()
	var b strings.Builder
	for _, s := range secrets {
		value, err := s.make(source)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(&b, "%s=%s\n", s.name, value)
	}
	b.WriteString("SENTINEL_MASTER_KEY_GENERATION=1\n")
	// Created, not truncated: a second process must not overwrite secrets
	// another one has just written.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	if _, err := file.WriteString(b.String()); err != nil {
		file.Close()
		return false, err
	}
	return true, file.Close()
}

func (i *Install) randSource() io.Reader {
	if i.Rand != nil {
		return i.Rand
	}
	return rand.Reader
}

// HasGitHubApp reports whether this installation has its GitHub App.
func (i *Install) HasGitHubApp() bool {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	return err == nil && envValue(string(data), "GITHUB_APP_ID") != ""
}

func (i *Install) readMark(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (i *Install) writeMark(name, value string) error {
	return os.WriteFile(filepath.Join(i.Dir, name), []byte(value+"\n"), 0o600)
}

func token(n int) func(io.Reader) (string, error) {
	return func(r io.Reader) (string, error) {
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(buf), nil
	}
}

// fernetKey is 32 random bytes in padded URL-safe base64, the form the server's
// vault expects.
func fernetKey(r io.Reader) (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	return strings.Join(lines, "\n")
}
