// Package local brings Sentinel up on this machine, with Docker, from a bundle
// embedded in the command: a compose file that pulls published images, and the
// files it mounts.
//
// Everything it keeps lives in one directory: the bundle, the secrets of this
// installation (generated here, never printed) and two markers. Running it again
// updates the bundle and starts what is stopped; it never regenerates a secret.
package local

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// The secrets of an installation, and how each is made.
var secrets = []struct {
	name string
	make func(io.Reader) (string, error)
}{
	{"SENTINEL_DB_PASSWORD", token(24)},
	{"SENTINEL_MASTER_KEY", fernetKey},
	{"SENTINEL_ADMIN_KEY", token(32)},
	{"SENTINEL_ARCHITECT_HMAC_KEY", token(48)},
	{signingKeySetting, signingKey},
	{toolBrokerKeySetting, token(32)},
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
	// GitHubTokenInput, when set, holds the GitHub token on its first line
	// instead of the GitHub CLI of this machine. `sentinel setup` uses it to
	// bring its own session to a server that has no GitHub CLI.
	GitHubTokenInput io.Reader
	// PortFree reports whether a port of 127.0.0.1 is free. Nil tries to listen on it.
	PortFree func(port int) bool

	// composeEnv overrides settings of the .env for this run only.
	composeEnv []string
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
	// GitHubLogin is the GitHub account Sentinel works as: the one of the
	// GitHub CLI session.
	GitHubLogin string
	// LiveViewPort is where Live View opens the app of a session, on 127.0.0.1.
	LiveViewPort int
}

// ErrDockerMissing and ErrDockerStopped are the two ways Docker is not ready.
var (
	ErrDockerMissing = errors.New("Docker is not installed. Install Docker Desktop (Windows, macOS) or Docker Engine (Linux), then run this again")
	ErrDockerStopped = errors.New("Docker is installed but not running. Start Docker, then run this again")
	// ErrDockerDenied is a Linux user outside the docker group: the daemon runs,
	// and this user may not talk to it.
	ErrDockerDenied = errors.New("your user may not use Docker. Run: sudo usermod -aG docker $USER, then log out and in again, and run this again")
	ErrNeedEmail    = errors.New("the first run needs the email of the owner: sentinel up --email you@example.com")
	// ErrGitHubTokenInput means the standard input held no GitHub token.
	ErrGitHubTokenInput = errors.New("the standard input did not hold a GitHub token on its first line")
	// Sentinel reaches GitHub with the session of the GitHub CLI on this
	// machine, and with nothing else.
	ErrGitHubCLIMissing = errors.New("the GitHub CLI is not installed. Install it from https://cli.github.com, sign in with: gh auth login, then run this again")
	ErrGitHubLogin      = errors.New("the GitHub CLI has no session. Sign in with: gh auth login, then run this again")
)

// GitHubTokenSetting is where the engine reads the token of the user's GitHub
// CLI session.
const GitHubTokenSetting = "SENTINEL_GITHUB_USER_TOKEN"

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_]{20,255}$`)

func (i *Install) say(format string, a ...any) {
	if i.Progress != nil {
		fmt.Fprintf(i.Progress, format+"\n", a...)
	}
}

func (i *Install) compose(args ...string) execx.Command {
	base := []string{"compose", "-p", Project, "--env-file", filepath.Join(i.Dir, envFile), "-f", filepath.Join(i.Dir, composeFile)}
	return execx.Command{Name: "docker", Args: append(base, args...), Dir: i.Dir, Env: i.composeEnv}
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
	// Read before anything is written or downloaded: without a session there
	// is nothing Sentinel could do on GitHub.
	githubToken, err := i.GitHubToken(ctx)
	if err != nil {
		return result, err
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
	} else if repaired, err := i.repairSigningKey(); err != nil {
		return result, fmt.Errorf("checking the receipt signing key: %w", err)
	} else if repaired {
		i.say("Replaced the receipt signing key: the one this installation had could not sign. Receipts from now on are signed.")
	}
	if !created {
		if added, err := i.addToolBrokerKey(); err != nil {
			return result, fmt.Errorf("checking the tool broker key: %w", err)
		} else if added {
			i.say("Added the key that lets agents reach the MCP servers you add in Settings > External tools.")
		}
	}
	if result.LiveViewPort, err = i.ensureLiveViewPort(); err != nil {
		return result, err
	}
	// Read again on every run, so a new `gh auth login` reaches Sentinel.
	if err := i.setEnv(GitHubTokenSetting, githubToken); err != nil {
		return result, fmt.Errorf("keeping the GitHub session: %w", err)
	}

	i.say("Downloading the images (the first time takes a few minutes)")
	if _, err := i.run(ctx, "downloading the images", i.compose("pull", "--quiet")); err != nil {
		return result, err
	}
	if err := i.checkSandboxRuntime(ctx); err != nil {
		return result, fmt.Errorf("checking the runtime of the agent's containers: %w", err)
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
	} else {
		result.Owner = owner
	}

	// After the owner: the first run creates the organization the account is
	// linked to.
	login, err := i.linkGitHub(ctx)
	if err != nil {
		return result, err
	}
	result.GitHubLogin = login
	return result, nil
}

// What the engine's link tool answers, said as what to run.
var gitHubLinkErrors = map[string]error{
	"github_session_missing":          ErrGitHubLogin,
	"github_session_rejected":         errors.New("GitHub does not accept the GitHub CLI session any more. Sign in again with: gh auth login, then run this again"),
	"github_session_lacks_repo_scope": errors.New("the GitHub CLI session cannot work on repositories. Run: gh auth refresh -s repo, then run this again"),
	"github_unreachable":              errors.New("GitHub could not be reached to check the GitHub CLI session. Check the connection and run this again"),
}

// linkGitHub has Sentinel record the account of the gh session as the GitHub
// link of its organization, and returns that account's login.
func (i *Install) linkGitHub(ctx context.Context) (string, error) {
	res, err := i.Runner.Run(ctx, i.compose("exec", "-T", "api", "python", "-m", "tools.operator.open_edition_github"))
	if err != nil {
		return "", fmt.Errorf("linking GitHub: %w", err)
	}
	var answer struct {
		Result    string `json:"result"`
		ErrorCode string `json:"error_code"`
		Login     string `json:"login"`
	}
	if json.Unmarshal([]byte(lastLine(res.Stdout)), &answer) != nil || (answer.Result != "ok" && answer.ErrorCode == "") {
		return "", fmt.Errorf("linking GitHub failed (exit %d): %s", res.ExitCode, tail(res.Stderr+res.Stdout))
	}
	if answer.Result != "ok" {
		if known, ok := gitHubLinkErrors[answer.ErrorCode]; ok {
			return "", known
		}
		return "", fmt.Errorf("linking GitHub failed: %s", answer.ErrorCode)
	}
	return answer.Login, nil
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

// GitHubToken is the token of the GitHub CLI session on this machine. It is
// never printed.
func (i *Install) GitHubToken(ctx context.Context) (string, error) {
	if i.GitHubTokenInput != nil {
		line, err := bufio.NewReader(io.LimitReader(i.GitHubTokenInput, 4096)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", ErrGitHubTokenInput
		}
		token := strings.TrimSpace(line)
		if !tokenPattern.MatchString(token) {
			return "", ErrGitHubTokenInput
		}
		return token, nil
	}
	if _, err := i.Runner.LookPath("gh"); err != nil {
		return "", ErrGitHubCLIMissing
	}
	res, err := i.Runner.Run(ctx, execx.Command{Name: "gh", Args: []string{"auth", "token", "--hostname", "github.com"}})
	if err != nil || res.ExitCode != 0 {
		return "", ErrGitHubLogin
	}
	token := strings.TrimSpace(res.Stdout)
	if !tokenPattern.MatchString(token) {
		return "", ErrGitHubLogin
	}
	return token, nil
}

// setEnv sets one setting of the .env, written whole beside it and renamed
// over it, so an interrupted write leaves the previous file.
func (i *Install) setEnv(name, value string) error {
	path := filepath.Join(i.Dir, envFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	found := false
	for n, line := range lines {
		if strings.HasPrefix(line, name+"=") {
			lines[n] = name + "=" + value
			found = true
		}
	}
	if !found {
		lines = append(lines, name+"="+value)
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	source := i.Rand
	if source == nil {
		source = rand.Reader
	}
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

// signingKeySetting signs the receipts of the runs.
const signingKeySetting = "SENTINEL_SIGNING_KEY"

// signingKey is an Ed25519 private key: 32 random bytes in standard base64, the
// form the server loads (and the one Sentinel's own servers are given). Any
// other form makes the server sign nothing.
func signingKey(r io.Reader) (string, error) {
	buf := make([]byte, ed25519SeedSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

const ed25519SeedSize = 32

// validSigningKey reports whether value is a key the server can sign with.
func validSigningKey(value string) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(raw) == ed25519SeedSize
}

// repairSigningKey replaces a signing key the server cannot use. Versions up
// to 0.3.2 wrote 48 bytes in URL-safe base64, so no receipt of those
// installations was signed; a new key loses nothing. It reports whether it
// replaced the key.
func (i *Install) repairSigningKey() (bool, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), signingKeySetting+"="); ok && validSigningKey(value) {
			return false, nil
		}
	}
	source := i.Rand
	if source == nil {
		source = rand.Reader
	}
	key, err := signingKey(source)
	if err != nil {
		return false, err
	}
	return true, i.setEnv(signingKeySetting, key)
}

// toolBrokerKeySetting signs the capability each run gets to reach the API's
// tool broker, which calls the MCP servers an admin adds. Installations made
// before 0.6.0 have none, and without it agents get no external tools.
const toolBrokerKeySetting = "SENTINEL_TOOL_BROKER_SIGNING_KEY"

// addToolBrokerKey writes the tool broker key when the installation has none.
// A key that is there is never replaced: the capabilities of runs in progress
// are signed with it. It reports whether it wrote one.
func (i *Install) addToolBrokerKey() (bool, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), toolBrokerKeySetting+"="); ok && strings.TrimSpace(value) != "" {
			return false, nil
		}
	}
	source := i.Rand
	if source == nil {
		source = rand.Reader
	}
	key, err := token(32)(source)
	if err != nil {
		return false, err
	}
	return true, i.setEnv(toolBrokerKeySetting, key)
}

// liveViewPortSetting is the port of 127.0.0.1 where Live View opens the app a
// session runs, at http://lv-….localhost:<port>: the bundle publishes the
// installation's gateway there and the server builds each session's address
// with it.
const liveViewPortSetting = "SENTINEL_LIVE_VIEW_PORT"

const (
	firstLiveViewPort = 8421
	lastLiveViewPort  = 8499
)

// ensureLiveViewPort returns the installation's Live View port, choosing a free
// one the first time. A port that is there is kept, even when something holds
// it now (Sentinel's own gateway does while it runs): an address must not
// move between runs.
func (i *Install) ensureLiveViewPort() (int, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), liveViewPortSetting+"="); ok {
			if port, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && port >= 1024 && port <= 65535 {
				return port, nil
			}
		}
	}
	free := i.PortFree
	if free == nil {
		free = portFree
	}
	for port := firstLiveViewPort; port <= lastLiveViewPort; port++ {
		if free(port) {
			return port, i.setEnv(liveViewPortSetting, strconv.Itoa(port))
		}
	}
	return 0, fmt.Errorf("no port between %d and %d of 127.0.0.1 is free for Live View. Free one, or set %s in %s, then run this again",
		firstLiveViewPort, lastLiveViewPort, liveViewPortSetting, filepath.Join(i.Dir, envFile))
}

func portFree(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	listener.Close()
	return true
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
