package local

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// docker plays Docker: each command is answered by the first rule whose text is
// part of its command line. Every command is recorded.
type docker struct {
	missing bool
	// noGH is a machine without the GitHub CLI.
	noGH  bool
	rules []rule
	calls []string
}

type rule struct {
	contains string
	result   execx.Result
	err      error
}

func (d *docker) LookPath(name string) (string, error) {
	if d.missing || (d.noGH && name == "gh") {
		return "", exec.ErrNotFound
	}
	return "/usr/bin/" + name, nil
}

func (d *docker) Run(_ context.Context, cmd execx.Command) (execx.Result, error) {
	line := strings.Join(append([]string{cmd.Name}, cmd.Args...), " ")
	d.calls = append(d.calls, line)
	for _, r := range d.rules {
		if strings.Contains(line, r.contains) {
			return r.result, r.err
		}
	}
	return execx.Result{}, nil
}

func (d *docker) ran(part string) int {
	n := 0
	for _, c := range d.calls {
		if strings.Contains(c, part) {
			n++
		}
	}
	return n
}

const owner = `{"created": "yes", "email": "you@example.com", "result": "ok", "sign_in_url": "http://localhost:8080/api/auth/verify?token=abc_DEF-1"}`

// ghToken is the token of the GitHub CLI session the fakes answer with.
var ghToken = "gho_" + strings.Repeat("A", 36)

func freshDocker() *docker {
	return &docker{rules: []rule{
		{contains: "gh auth token --hostname github.com", result: execx.Result{Stdout: ghToken + "\n"}},
		{contains: "open_edition_github", result: execx.Result{Stdout: `{"login": "you", "result": "ok"}` + "\n"}},
		{contains: "compose -p sentinel --env-file", result: execx.Result{}},
		// The first start of the API stops at the key check, as on a real server.
		{contains: "run --rm --no-deps api", result: execx.Result{ExitCode: 3, Stderr: "AdmissionError: master_key_admission_marker_missing"}},
	}}
}

func withOwner(d *docker, answer string) *docker {
	d.rules = append([]rule{{contains: "open_edition_owner", result: execx.Result{Stdout: "warming up\n" + answer + "\n"}}}, d.rules...)
	return d
}

var bundle = fstest.MapFS{
	"compose.yaml":                       {Data: []byte("services: {}\n")},
	"VERSION":                            {Data: []byte("0123\n")},
	"postgres-init/01-init-databases.sh": {Data: []byte("#!/bin/bash\n")},
	"squid/squid.conf":                   {Data: []byte("http_port 3128\n")},
}

func newInstall(t *testing.T, d *docker) (*Install, *bytes.Buffer) {
	t.Helper()
	var progress bytes.Buffer
	return &Install{
		Runner:   d,
		Dir:      filepath.Join(t.TempDir(), "local"),
		Bundle:   bundle,
		Rand:     bytes.NewReader(bytes.Repeat([]byte{7}, 1024)),
		Progress: &progress,
	}, &progress
}

func TestTheFirstUpCreatesEverythingInOrderAndReturnsTheSignInLink(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, _ := newInstall(t, d)

	result, err := install.Up(context.Background(), "You@Example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SignInURL != "http://localhost:8080/api/auth/verify?token=abc_DEF-1" || !result.Created || result.Owner != "you@example.com" {
		t.Errorf("result = %+v", result)
	}
	order := []string{"compose version", "info", " pull --quiet", "up -d --wait postgres temporal", "run --rm --no-deps api", "initialize-admission", "up -d --wait --wait-timeout 900", "open_edition_owner --email you@example.com"}
	at := 0
	for _, call := range d.calls {
		if at < len(order) && strings.Contains(call, order[at]) {
			at++
		}
	}
	if at != len(order) {
		t.Errorf("step %q missing or out of order in:\n%s", order[at], strings.Join(d.calls, "\n"))
	}
	for _, file := range []string{"compose.yaml", "VERSION", "postgres-init/01-init-databases.sh", "squid/squid.conf"} {
		if _, err := os.Stat(filepath.Join(install.Dir, file)); err != nil {
			t.Errorf("the bundle file %s was not written: %v", file, err)
		}
	}
}

func TestTheSecretsAreWrittenOnceAndNeverPrinted(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, progress := newInstall(t, d)
	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(install.Dir, ".env")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SENTINEL_DB_PASSWORD=", "SENTINEL_MASTER_KEY=", "SENTINEL_MASTER_KEY_GENERATION=1", "SENTINEL_ADMIN_KEY=", "SENTINEL_ARCHITECT_HMAC_KEY=", "SENTINEL_SIGNING_KEY="} {
		if !strings.Contains(string(first), name) {
			t.Errorf("%s missing from the secrets", name)
		}
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("secrets mode = %v, want 0600", info.Mode().Perm())
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(first)), "\n") {
		value := line[strings.IndexByte(line, '=')+1:]
		if len(value) > 4 && (strings.Contains(progress.String(), value) || strings.Contains(strings.Join(d.calls, "\n"), value)) {
			t.Errorf("a secret reached the output or a command line: %s", line[:strings.IndexByte(line, '=')])
		}
	}

	// A second run, with another random source, keeps them.
	install.Rand = bytes.NewReader(bytes.Repeat([]byte{9}, 1024))
	if _, err := install.Up(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Error("a second run changed the secrets")
	}
}

func TestTheMasterKeyIsAFernetKey(t *testing.T) {
	key, err := fernetKey(bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 44 || !strings.HasSuffix(key, "=") || strings.ContainsAny(key, "+/") {
		t.Errorf("key %q is not 32 bytes in padded URL-safe base64", key)
	}
}

func TestALaterUpSkipsTheSetupAndPrintsNoLinkUnlessAsked(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, _ := newInstall(t, d)
	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	d.calls = nil

	result, err := install.Up(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SignInURL != "" || result.Owner != "you@example.com" {
		t.Errorf("result = %+v", result)
	}
	for _, step := range []string{"initialize-admission", "run --rm --no-deps api", "open_edition_owner"} {
		if d.ran(step) != 0 {
			t.Errorf("a later run ran %q again", step)
		}
	}

	d.calls = nil
	result, err = install.Up(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.SignInURL == "" || d.ran("open_edition_owner --email you@example.com") != 1 {
		t.Errorf("--sign-in: result %+v, calls %v", result, d.calls)
	}
}

func TestTheFirstUpNeedsAnEmailAndTouchesNothingWithoutOne(t *testing.T) {
	d := freshDocker()
	install, _ := newInstall(t, d)

	_, err := install.Up(context.Background(), "", false)
	if !errors.Is(err, ErrNeedEmail) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(install.Dir); err == nil {
		t.Error("the installation folder was created without an owner")
	}
	if d.ran("compose -p") != 0 {
		t.Errorf("compose ran: %v", d.calls)
	}
}

func TestASecondOwnerIsRefused(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, _ := newInstall(t, d)
	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	if _, err := install.Up(context.Background(), "someone@example.com", false); err == nil || !strings.Contains(err.Error(), "already has an owner, you@example.com") {
		t.Errorf("err = %v", err)
	}
}

func TestDockerThatIsMissingOrStoppedIsSaidPlainly(t *testing.T) {
	install, _ := newInstall(t, &docker{missing: true})
	if _, err := install.Up(context.Background(), "you@example.com", false); !errors.Is(err, ErrDockerMissing) {
		t.Errorf("missing: %v", err)
	}

	denied := &docker{rules: []rule{{contains: "docker info", result: execx.Result{ExitCode: 1, Stderr: "permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock"}}}}
	install, _ = newInstall(t, denied)
	if _, err := install.Up(context.Background(), "you@example.com", false); !errors.Is(err, ErrDockerDenied) {
		t.Errorf("denied: %v", err)
	}

	stopped := &docker{rules: []rule{{contains: "docker info", result: execx.Result{ExitCode: 1}}}}
	install, _ = newInstall(t, stopped)
	if _, err := install.Up(context.Background(), "you@example.com", false); !errors.Is(err, ErrDockerStopped) {
		t.Errorf("stopped: %v", err)
	}
}

func TestAFailedSchemaStepStopsTheSetupWithWhatDockerSaid(t *testing.T) {
	d := &docker{rules: []rule{
		{contains: "gh auth token", result: execx.Result{Stdout: ghToken + "\n"}},
		{contains: "run --rm --no-deps api", result: execx.Result{ExitCode: 1, Stderr: "could not connect to the database"}},
	}}
	install, _ := newInstall(t, d)

	_, err := install.Up(context.Background(), "you@example.com", false)
	if err == nil || !strings.Contains(err.Error(), "could not connect to the database") {
		t.Fatalf("err = %v", err)
	}
	if d.ran("initialize-admission") != 0 {
		t.Error("the key was admitted after the schema step failed")
	}
	if _, err := os.Stat(filepath.Join(install.Dir, admissionMark)); err == nil {
		t.Error("the setup was marked done")
	}
}

func TestAnOwnerAnswerThatIsNotUnderstoodOrPointsElsewhereIsRefused(t *testing.T) {
	for name, answer := range map[string]string{
		"refused":   `{"result": "failed", "error_code": "open_edition_owner_exists"}`,
		"not json":  `Traceback (most recent call last)`,
		"elsewhere": `{"created": "yes", "email": "you@example.com", "result": "ok", "sign_in_url": "https://attacker.example/api/auth/verify?token=x"}`,
	} {
		d := withOwner(freshDocker(), answer)
		install, _ := newInstall(t, d)
		result, err := install.Up(context.Background(), "you@example.com", false)
		if err == nil || result.SignInURL != "" {
			t.Errorf("%s: result %+v, err %v", name, result, err)
		}
		if _, err := os.Stat(filepath.Join(install.Dir, ownerMark)); err == nil {
			t.Errorf("%s: the owner was recorded", name)
		}
	}
}

func TestDownStopsAndKeepsTheData(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, _ := newInstall(t, d)
	if err := install.Down(context.Background()); err == nil {
		t.Error("down on a machine without Sentinel succeeded")
	}
	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	d.calls = nil
	if err := install.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.ran(" stop") != 1 || d.ran("down") != 0 || d.ran(" -v") != 0 {
		t.Errorf("calls = %v, want one stop and nothing that removes data", d.calls)
	}
}

func TestUpUsesTheGitHubCLISessionAndReadsItAgainEachTime(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, progress := newInstall(t, d)
	if _, err := install.Up(context.Background(), "you@example.com", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(install.Dir, ".env")
	first, _ := os.ReadFile(path)
	if !strings.Contains(string(first), "SENTINEL_GITHUB_USER_TOKEN="+ghToken+"\n") {
		t.Fatalf("the session is not in the .env:\n%s", first)
	}
	if strings.Contains(progress.String(), ghToken) {
		t.Error("the token was printed")
	}

	// After another `gh auth login`, the next run hands Sentinel the new token
	// and changes nothing else.
	renewed := "gho_" + strings.Repeat("B", 36)
	d.rules[1].result = execx.Result{Stdout: renewed + "\n"}
	if _, err := install.Up(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if strings.Count(string(second), "SENTINEL_GITHUB_USER_TOKEN=") != 1 || !strings.Contains(string(second), "SENTINEL_GITHUB_USER_TOKEN="+renewed+"\n") {
		t.Errorf("the renewed session was not kept once:\n%s", second)
	}
	if strings.Replace(string(second), renewed, ghToken, 1) != string(first) {
		t.Error("a later run changed more than the session")
	}
}

func TestUpWithoutAGitHubCLISessionStopsBeforeDoingAnything(t *testing.T) {
	cases := []struct {
		name string
		d    *docker
		want error
	}{
		{"no GitHub CLI", &docker{noGH: true}, ErrGitHubCLIMissing},
		{"signed out", &docker{rules: []rule{{contains: "gh auth token", result: execx.Result{ExitCode: 1, Stderr: "no oauth token found for github.com"}}}}, ErrGitHubLogin},
		{"an answer that is not a token", &docker{rules: []rule{{contains: "gh auth token", result: execx.Result{Stdout: "not a token\n"}}}}, ErrGitHubLogin},
	}
	for _, c := range cases {
		install, _ := newInstall(t, c.d)
		if _, err := install.Up(context.Background(), "you@example.com", false); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if c.d.ran("pull") != 0 || c.d.ran("up -d") != 0 {
			t.Errorf("%s: Docker was used: %v", c.name, c.d.calls)
		}
		if _, err := os.Stat(filepath.Join(install.Dir, ".env")); err == nil {
			t.Errorf("%s: secrets were written", c.name)
		}
	}
	if !strings.Contains(ErrGitHubLogin.Error(), "gh auth login") || !strings.Contains(ErrGitHubCLIMissing.Error(), "https://cli.github.com") {
		t.Error("the errors do not say what to run")
	}
}

func TestUpLinksTheSessionAccountAfterTheOwnerAndSaysWhatGitHubRefused(t *testing.T) {
	d := withOwner(freshDocker(), owner)
	install, _ := newInstall(t, d)
	result, err := install.Up(context.Background(), "you@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.GitHubLogin != "you" {
		t.Errorf("GitHubLogin = %q", result.GitHubLogin)
	}
	// The first run creates the organization the account is linked to.
	ownerAt, linkAt := -1, -1
	for n, call := range d.calls {
		if strings.Contains(call, "open_edition_owner") {
			ownerAt = n
		}
		if strings.Contains(call, "exec -T api python -m tools.operator.open_edition_github") {
			linkAt = n
		}
	}
	if ownerAt < 0 || linkAt < ownerAt {
		t.Errorf("the link ran before the owner existed:\n%s", strings.Join(d.calls, "\n"))
	}

	for code, want := range map[string]string{
		"github_session_rejected":         "gh auth login",
		"github_session_lacks_repo_scope": "gh auth refresh -s repo",
		"github_session_missing":          "gh auth login",
	} {
		refused := withOwner(freshDocker(), owner)
		refused.rules[2].result = execx.Result{ExitCode: 2, Stdout: `{"error_code": "` + code + `", "result": "failed"}` + "\n"}
		install, _ := newInstall(t, refused)
		if _, err := install.Up(context.Background(), "you@example.com", false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to say %q", code, err, want)
		}
	}
}
