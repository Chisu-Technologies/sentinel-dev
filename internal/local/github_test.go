package local

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testPEM = "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf9Cnzj4p4WGeKLs1Pt8Qu\nKUpRKfFLfRYC9AIKjbJTWit+CqvjWYzvQwECAwEAAQ==\n-----END RSA PRIVATE KEY-----\n"

// fakeGitHub answers the one request the flow makes to GitHub's API, and
// records the codes it was asked to exchange.
type fakeGitHub struct {
	status    int
	exchanged []string
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m := regexp.MustCompile(`^/app-manifests/([^/]+)/conversions$`).FindStringSubmatch(r.URL.Path)
	if r.Method != http.MethodPost || m == nil {
		http.NotFound(w, r)
		return
	}
	f.exchanged = append(f.exchanged, m[1])
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id": 424242, "slug": "sentinel-local-abcdef", "client_id": "Iv23liCLIENT",
		"client_secret": "c0ffee5ecret", "webhook_secret": "w3bh00k", "pem": testPEM,
		"html_url": "https://github.com/apps/sentinel-local-abcdef", "owner": map[string]string{"login": "octo"},
	})
}

// browser plays the user: it opens the local page, checks the form that would
// go to GitHub, and then follows the redirect GitHub would make, with the
// state the page carried unless wrongState is set first.
type browser struct {
	t          *testing.T
	wrongState bool
	action     string
	manifest   map[string]any
	refused    int
}

func (b *browser) open(page string) error {
	go func() {
		res, err := http.Get(page)
		if err != nil {
			b.t.Errorf("opening the local page: %v", err)
			return
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		form := regexp.MustCompile(`action="([^"]+)"[\s\S]*name="manifest" value="([^"]+)"`).FindStringSubmatch(string(body))
		if form == nil {
			b.t.Errorf("the local page has no manifest form:\n%s", body)
			return
		}
		b.action = html.UnescapeString(form[1])
		if err := json.Unmarshal([]byte(html.UnescapeString(form[2])), &b.manifest); err != nil {
			b.t.Errorf("the manifest is not JSON: %v", err)
			return
		}
		target, _ := url.Parse(b.action)
		state := target.Query().Get("state")
		redirect := b.manifest["redirect_url"].(string)
		if b.wrongState {
			res, err := http.Get(redirect + "?code=stolen&state=" + url.QueryEscape(state+"x"))
			if err == nil {
				b.refused = res.StatusCode
				res.Body.Close()
			}
		}
		res, err = http.Get(redirect + "?code=one-time-code&state=" + url.QueryEscape(state))
		if err == nil {
			res.Body.Close()
		}
	}()
	return nil
}

func installedWithSecrets(t *testing.T) (*Install, *docker, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, envFile), []byte("SENTINEL_DB_PASSWORD=x\nSENTINEL_MASTER_KEY_GENERATION=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := freshDocker()
	return &Install{Runner: d, Dir: dir, Bundle: bundle, Rand: strings.NewReader(strings.Repeat("r", 200))}, d, filepath.Join(dir, envFile)
}

func connect(t *testing.T, install *Install, gh *fakeGitHub, b *browser, org string) (GitHubApp, error) {
	t.Helper()
	api := httptest.NewServer(gh)
	t.Cleanup(api.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return install.ConnectGitHub(ctx, GitHubConnect{
		Org: org, Open: b.open, Web: "https://github.example", API: api.URL, Timeout: 10 * time.Second,
	})
}

func TestConnectGitHubCreatesTheAppAndKeepsItsCredentials(t *testing.T) {
	install, d, envPath := installedWithSecrets(t)
	gh := &fakeGitHub{}
	b := &browser{t: t}

	app, err := connect(t, install, gh, b, "")
	if err != nil {
		t.Fatal(err)
	}

	if app.Slug != "sentinel-local-abcdef" || app.Owner.Login != "octo" {
		t.Errorf("app = %+v", app)
	}
	if !strings.HasPrefix(b.action, "https://github.example/settings/apps/new?state=") {
		t.Errorf("the form posts to %q", b.action)
	}
	if got := fmt.Sprint(gh.exchanged); got != "[one-time-code]" {
		t.Errorf("codes exchanged = %s", got)
	}
	// What GitHub is told: the engine's own setup and authorization routes, no
	// webhook, and the permissions the engine asks for.
	m := b.manifest
	if m["setup_url"] != "http://localhost:8080/api/auth/github/setup" ||
		fmt.Sprint(m["callback_urls"]) != "[http://localhost:8080/api/auth/github/setup/authorize]" ||
		m["public"] != false || m["request_oauth_on_install"] != false {
		t.Errorf("manifest = %v", m)
	}
	if hook := m["hook_attributes"].(map[string]any); hook["active"] != false {
		t.Errorf("the webhook is active: %v", hook)
	}
	if !strings.HasPrefix(m["redirect_url"].(string), "http://127.0.0.1:") {
		t.Errorf("GitHub's answer goes to %v, not to this computer", m["redirect_url"])
	}
	perms := m["default_permissions"].(map[string]any)
	if perms["contents"] != "write" || perms["pull_requests"] != "write" || perms["issues"] != "write" ||
		perms["checks"] != "read" || perms["workflows"] != "write" || perms["administration"] != nil {
		t.Errorf("permissions = %v", perms)
	}
	if !regexp.MustCompile(`^sentinel-local-[0-9a-f]{6}$`).MatchString(m["name"].(string)) {
		t.Errorf("name = %v", m["name"])
	}

	data, _ := os.ReadFile(envPath)
	env := string(data)
	for _, want := range []string{
		"SENTINEL_DB_PASSWORD=x\n",
		"GITHUB_APP_ID=424242\n",
		"GITHUB_APP_SLUG=sentinel-local-abcdef\n",
		`GITHUB_APP_PRIVATE_KEY='-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf9Cnzj4p4WGeKLs1Pt8Qu\nKUpRKfFLfRYC9AIKjbJTWit+CqvjWYzvQwECAwEAAQ==\n-----END RSA PRIVATE KEY-----'` + "\n",
		"GITHUB_APP_USER_CLIENT_ID=Iv23liCLIENT\n",
		"GITHUB_APP_USER_CLIENT_SECRET=c0ffee5ecret\n",
		"GITHUB_WEBHOOK_SECRET=w3bh00k\n",
	} {
		if !strings.Contains(env, want) {
			t.Errorf(".env lacks %q:\n%s", want, env)
		}
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(envPath); info.Mode().Perm() != 0o600 {
			t.Errorf(".env mode = %v", info.Mode().Perm())
		}
	}
	if d.ran("up -d --wait") != 1 {
		t.Errorf("Sentinel was not restarted with the App: %v", d.calls)
	}
	if !install.HasGitHubApp() {
		t.Error("HasGitHubApp is false after connecting")
	}
}

func TestConnectGitHubRefusesAnAnswerWithAnotherState(t *testing.T) {
	install, _, _ := installedWithSecrets(t)
	gh := &fakeGitHub{}
	b := &browser{t: t, wrongState: true}

	if _, err := connect(t, install, gh, b, ""); err != nil {
		t.Fatal(err)
	}
	if b.refused != http.StatusBadRequest {
		t.Errorf("an answer with another state got HTTP %d", b.refused)
	}
	if got := fmt.Sprint(gh.exchanged); got != "[one-time-code]" {
		t.Errorf("codes exchanged = %s; the one with another state must not be", got)
	}
}

func TestConnectGitHubUnderAnOrganization(t *testing.T) {
	install, _, _ := installedWithSecrets(t)
	b := &browser{t: t}
	if _, err := connect(t, install, &fakeGitHub{}, b, "acme-labs"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.action, "https://github.example/organizations/acme-labs/settings/apps/new?state=") {
		t.Errorf("the form posts to %q", b.action)
	}
}

func TestConnectGitHubLeavesTheEnvAsItWasWhenGitHubRefuses(t *testing.T) {
	install, d, envPath := installedWithSecrets(t)
	before, _ := os.ReadFile(envPath)

	_, err := connect(t, install, &fakeGitHub{status: http.StatusUnprocessableEntity}, &browser{t: t}, "")

	if err == nil || !strings.Contains(err.Error(), "HTTP 422") {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(envPath)
	if string(after) != string(before) {
		t.Errorf(".env changed:\n%s", after)
	}
	if d.ran("up -d") != 0 {
		t.Errorf("Sentinel was restarted: %v", d.calls)
	}
}

func TestConnectGitHubNeedsAnInstallationWithoutAnApp(t *testing.T) {
	notInstalled := &Install{Runner: freshDocker(), Dir: t.TempDir(), Bundle: bundle}
	opened := false
	open := func(string) error { opened = true; return nil }
	if _, err := notInstalled.ConnectGitHub(context.Background(), GitHubConnect{Open: open}); err != ErrNotInstalled {
		t.Errorf("not installed: err = %v", err)
	}

	install, _, envPath := installedWithSecrets(t)
	os.WriteFile(envPath, []byte("GITHUB_APP_ID=7\nGITHUB_APP_SLUG=sentinel-local-000000\n"), 0o600)
	_, err := install.ConnectGitHub(context.Background(), GitHubConnect{Open: open})
	if err == nil || !strings.Contains(err.Error(), "already has a GitHub App (sentinel-local-000000)") {
		t.Errorf("already connected: err = %v", err)
	}

	install2, _, _ := installedWithSecrets(t)
	if _, err := install2.ConnectGitHub(context.Background(), GitHubConnect{Org: "bad/org", Open: open}); err == nil {
		t.Error("an organization name with a slash was accepted")
	}
	if opened {
		t.Error("the browser was opened although nothing could be created")
	}
}
