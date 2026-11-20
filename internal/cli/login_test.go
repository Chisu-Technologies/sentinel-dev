package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

const (
	loginKey    = "sk-sentinel-login-test-key"
	deviceCode  = "dddddddddddddddddddddddddddddddddddddddddddd"
	loginUserCd = "K7QM-3XPD"
)

// loginServer approves the sign-in after pendingPolls collections, or answers
// collect with finalCode instead of a key.
type loginServer struct {
	*httptest.Server
	mu           sync.Mutex
	started      map[string]string
	auths        []string
	polls        int
	pendingPolls int
	finalCode    string
}

func newLoginServer(t *testing.T, pendingPolls int, finalCode string) *loginServer {
	t.Helper()
	l := &loginServer{pendingPolls: pendingPolls, finalCode: finalCode}
	l.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.auths = append(l.auths, r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/cli/login":
			json.Unmarshal(body, &l.started)
			io.WriteString(w, `{"device_code":"`+deviceCode+`","user_code":"`+loginUserCd+`","verification_path":"/elsewhere","expires_in":600,"interval":5}`)
		case "POST /api/cli/login/token":
			l.polls++
			if l.polls <= l.pendingPolls {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"detail":{"code":"authorization_pending","message":"Waiting"}}`)
				return
			}
			if l.finalCode != "" {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"detail":{"code":"`+l.finalCode+`","message":"no"}}`)
				return
			}
			io.WriteString(w, `{"api_key":"`+loginKey+`","organization":"Acme"}`)
		case "GET /api/runs":
			io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(l.Close)
	return l
}

func loginCommand(t *testing.T, configDir string, vars map[string]string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Env{
		Stdout:    &stdout,
		Stderr:    &stderr,
		Runner:    &execxtest.Fake{},
		Build:     buildinfo.Info{Version: "v0.3.4", OS: "linux", Arch: "amd64"},
		Getenv:    func(name string) string { return vars[name] },
		Sleep:     func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		ConfigDir: configDir,
	})
	return result{code, stdout.String(), stderr.String()}
}

func TestLoginWaitsForTheApprovalAndSavesTheKeyForThisUserOnly(t *testing.T) {
	l := newLoginServer(t, 2, "")
	dir := t.TempDir()

	got := loginCommand(t, dir, nil, "login", "--url", l.URL)

	if got.code != ExitOK {
		t.Fatalf("got %+v", got)
	}
	// The page is built from the address the user gave, not from the server.
	if !strings.Contains(got.stdout, l.URL+"/cli-login/"+loginUserCd) || strings.Contains(got.stdout, "/elsewhere") {
		t.Errorf("stdout = %s", got.stdout)
	}
	if strings.Contains(got.stdout+got.stderr, loginKey) || strings.Contains(got.stdout+got.stderr, deviceCode) {
		t.Error("the key or the device code was printed")
	}
	if l.started["client"] != "sentinel v0.3.4 on linux/amd64" {
		t.Errorf("client = %q", l.started["client"])
	}
	for i, auth := range l.auths {
		if auth != "" {
			t.Errorf("request %d carried Authorization %q", i, auth)
		}
	}
	path := filepath.Join(dir, "sentinel", "credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved credentials
	if json.Unmarshal(data, &saved) != nil || saved.URL != l.URL || saved.APIKey != loginKey {
		t.Errorf("saved %s", data)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", info.Mode().Perm())
		}
	}
}

func TestLoginThatIsDeniedSavesNothing(t *testing.T) {
	l := newLoginServer(t, 0, "access_denied")
	dir := t.TempDir()

	got := loginCommand(t, dir, nil, "login", "--url", l.URL)

	if got.code != ExitFailed || !strings.Contains(got.stderr, "denied") {
		t.Errorf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "sentinel", "credentials.json")); err == nil {
		t.Error("a denied sign-in saved a key")
	}
}

// The panel's setup guide says "run sentinel login": with no address and a
// Sentinel that `up` installed here, that is the server it signs in to.
func TestLoginWithoutAnAddressSignsInToTheSentinelOnThisMachine(t *testing.T) {
	l := newLoginServer(t, 0, "")
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"login"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Build:         buildinfo.Info{Version: "v0.7.1", OS: "windows", Arch: "amd64"},
		Getenv:        func(name string) string { return map[string]string{"SENTINEL_HOME": home}[name] },
		Sleep:         func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		ConfigDir:     dir,
		LocalPanelURL: l.URL,
	})
	if code != ExitOK {
		t.Fatalf("exit %d; stdout %s; stderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Signing in to the Sentinel on this machine ("+l.URL+")") {
		t.Errorf("stdout = %s", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "sentinel", "credentials.json"))
	var saved credentials
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.URL != l.URL || saved.APIKey != loginKey {
		t.Errorf("saved %s (%v)", data, err)
	}
}

func TestLoginWithoutAnAddressOrALocalSentinelSaysWhatItNeeds(t *testing.T) {
	got := loginCommand(t, t.TempDir(), map[string]string{"SENTINEL_HOME": t.TempDir()}, "login")
	if got.code != ExitUsage || !strings.Contains(got.stderr, "sentinel login --url") {
		t.Errorf("got %+v", got)
	}
}

func TestLoginRefusesAPlainHTTPServerThatIsNotLocal(t *testing.T) {
	got := loginCommand(t, t.TempDir(), nil, "login", "--url", "http://sentinel.example.com")
	if got.code != ExitUsage || !strings.Contains(got.stderr, "https") {
		t.Errorf("got %+v", got)
	}
}

func TestCommandsUseTheSavedSignInWhenTheEnvironmentHasNone(t *testing.T) {
	l := newLoginServer(t, 0, "")
	dir := t.TempDir()
	if got := loginCommand(t, dir, nil, "login", "--url", l.URL); got.code != ExitOK {
		t.Fatalf("login: %+v", got)
	}

	got := loginCommand(t, dir, nil, "runs")

	if got.code != ExitOK {
		t.Fatalf("runs: %+v", got)
	}
	if last := l.auths[len(l.auths)-1]; last != "Bearer "+loginKey {
		t.Errorf("runs sent Authorization %q", last)
	}
}

func TestTheEnvironmentWinsOverTheSavedSignIn(t *testing.T) {
	l := newLoginServer(t, 0, "")
	dir := t.TempDir()
	loginCommand(t, dir, nil, "login", "--url", l.URL)

	loginCommand(t, dir, map[string]string{"SENTINEL_URL": l.URL, "SENTINEL_API_KEY": "sk-from-env"}, "runs")

	if last := l.auths[len(l.auths)-1]; last != "Bearer sk-from-env" {
		t.Errorf("runs sent Authorization %q", last)
	}
}

func TestLogoutForgetsTheSignIn(t *testing.T) {
	l := newLoginServer(t, 0, "")
	dir := t.TempDir()
	loginCommand(t, dir, nil, "login", "--url", l.URL)

	out := loginCommand(t, dir, nil, "logout")
	again := loginCommand(t, dir, nil, "logout")
	runs := loginCommand(t, dir, nil, "runs")

	if out.code != ExitOK || !strings.Contains(out.stdout, "revoke it in Settings") {
		t.Errorf("logout: %+v", out)
	}
	if again.code != ExitOK || !strings.Contains(again.stdout, "no sign-in") {
		t.Errorf("second logout: %+v", again)
	}
	if runs.code != ExitFailed || !strings.Contains(runs.stderr, "sentinel login") {
		t.Errorf("runs after logout: %+v", runs)
	}
}
