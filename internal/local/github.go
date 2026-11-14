package local

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The engine reaches GitHub only as a GitHub App. The App of the hosted service
// cannot serve an installation on someone's computer, so each installation
// registers its own, through GitHub's app-manifest flow: the browser posts a
// description of the App to GitHub, the user confirms, and GitHub sends a
// one-time code back to this computer, which exchanges it for the App's
// credentials. They stay in the installation's .env, where the engine reads them.

// GitHubApp is what GitHub returns for the new App.
type GitHubApp struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
	HTMLURL       string `json:"html_url"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// GitHubConnect is how ConnectGitHub reaches the browser and GitHub.
type GitHubConnect struct {
	// Org registers the App under this GitHub organization. Empty: under the
	// account of whoever confirms it.
	Org string
	// Open shows a page to the user, in their browser.
	Open func(url string) error
	// Waiting is told the address of the local page, for when no browser opened.
	Waiting func(url string)
	// Web and API are GitHub's addresses. Empty: github.com and its API.
	Web, API string
	// HTTP makes the request to GitHub's API. Nil: a client with a 30 s timeout.
	HTTP *http.Client
	// Timeout is how long to wait for the user. Zero: 15 minutes.
	Timeout time.Duration
}

// ErrNotInstalled is a command that needs `sentinel up` to have run first.
var ErrNotInstalled = errors.New("Sentinel is not installed on this machine: run sentinel up first")

// The permissions the engine asks GitHub for: contents, pull requests and
// issues for runs, checks to read CI, workflows to push a change to
// .github/workflows. Administration (creating repositories in an organization)
// is left out: it can also delete them.
var appPermissions = map[string]string{
	"metadata":      "read",
	"contents":      "write",
	"pull_requests": "write",
	"issues":        "write",
	"checks":        "read",
	"workflows":     "write",
}

var orgPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

// manifest describes the App to GitHub. redirect is where GitHub sends the code.
func manifest(name, redirect string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"name":        name,
		"url":         "https://github.com/Chisu-Technologies/sentinel-dev",
		"description": "Sentinel running on one computer. Created by `sentinel github`.",
		"public":      false,
		// GitHub cannot reach a computer behind a home router, so the App has
		// no webhook. Linking the installation goes through the browser.
		"hook_attributes": map[string]any{"url": PanelURL + "/api/webhook/github", "active": false},
		"redirect_url":    redirect,
		// Where GitHub returns after the App is installed, and where the engine
		// proves the user may see that installation. Both are the engine's own.
		"setup_url":                PanelURL + "/api/auth/github/setup",
		"callback_urls":            []string{PanelURL + "/api/auth/github/setup/authorize"},
		"setup_on_update":          true,
		"request_oauth_on_install": false,
		"default_permissions":      appPermissions,
		"default_events":           []string{},
	})
}

var formPage = template.Must(template.New("form").Parse(`<!doctype html>
<meta charset="utf-8"><title>Sentinel: create the GitHub App</title>
<body style="font-family:system-ui,sans-serif;max-width:36rem;margin:3rem auto;padding:0 1rem">
<p>Taking you to GitHub to create the GitHub App of this Sentinel…</p>
<form id="f" method="post" action="{{.Action}}">
<input type="hidden" name="manifest" value="{{.Manifest}}">
<button type="submit">Continue on GitHub</button>
</form>
<script>document.getElementById("f").submit()</script>
`))

const donePage = `<!doctype html>
<meta charset="utf-8"><title>Sentinel</title>
<body style="font-family:system-ui,sans-serif;max-width:36rem;margin:3rem auto;padding:0 1rem">
<p>GitHub answered. You can close this tab and go back to the terminal.</p>
`

// ConnectGitHub registers a GitHub App for this installation, keeps its
// credentials and restarts Sentinel with them.
func (i *Install) ConnectGitHub(ctx context.Context, c GitHubConnect) (GitHubApp, error) {
	var app GitHubApp
	envPath := filepath.Join(i.Dir, envFile)
	current, err := os.ReadFile(envPath)
	if err != nil {
		return app, ErrNotInstalled
	}
	if slug := envValue(string(current), "GITHUB_APP_SLUG"); envValue(string(current), "GITHUB_APP_ID") != "" {
		return app, fmt.Errorf("this installation already has a GitHub App (%s). To install it on more repositories, use Connect GitHub in the panel", slug)
	}
	if c.Org != "" && !orgPattern.MatchString(c.Org) {
		return app, errors.New("that is not a GitHub organization name")
	}
	if err := i.CheckDocker(ctx); err != nil {
		return app, err
	}

	web, api := strings.TrimRight(c.Web, "/"), strings.TrimRight(c.API, "/")
	if web == "" {
		web = "https://github.com"
	}
	if api == "" {
		api = "https://api.github.com"
	}
	source := i.randSource()
	state, err := token(24)(source)
	if err != nil {
		return app, err
	}
	suffix := make([]byte, 3)
	if _, err := io.ReadFull(source, suffix); err != nil {
		return app, err
	}
	name := fmt.Sprintf("sentinel-local-%x", suffix)

	// Only this computer can reach the page and the callback.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return app, fmt.Errorf("opening a port for GitHub's answer: %w", err)
	}
	base := "http://" + listener.Addr().String()
	body, err := manifest(name, base+"/callback")
	if err != nil {
		listener.Close()
		return app, err
	}
	action := web + "/settings/apps/new?state=" + url.QueryEscape(state)
	if c.Org != "" {
		action = web + "/organizations/" + url.PathEscape(c.Org) + "/settings/apps/new?state=" + url.QueryEscape(state)
	}

	codes := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = formPage.Execute(w, map[string]string{"Action": action, "Manifest": string(body)})
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		got, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
		// A code with any other state did not come from the page this command
		// served, and is not exchanged.
		if subtle.ConstantTimeCompare([]byte(got), []byte(state)) != 1 || code == "" {
			http.Error(w, "This answer is not for this Sentinel.", http.StatusBadRequest)
			return
		}
		select {
		case codes <- code:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, donePage)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go server.Serve(listener)
	defer server.Close()

	if c.Waiting != nil {
		c.Waiting(base + "/")
	}
	if c.Open != nil {
		_ = c.Open(base + "/")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	var code string
	select {
	case code = <-codes:
	case <-time.After(timeout):
		return app, errors.New("GitHub did not answer in time; nothing was created here. Run this again")
	case <-ctx.Done():
		return app, ctx.Err()
	}
	server.Close()

	app, err = convertManifest(ctx, c.HTTP, api, code)
	if err != nil {
		return app, err
	}
	i.say("Created the GitHub App %s", app.Slug)
	if err := appendEnv(envPath, current, app); err != nil {
		return app, fmt.Errorf("keeping the App's credentials: %w", err)
	}
	i.say("Restarting Sentinel with it")
	if _, err := i.run(ctx, "restarting Sentinel", i.compose("up", "-d", "--wait", "--wait-timeout", waitTimeoutSec)); err != nil {
		return app, err
	}
	return app, nil
}

// convertManifest exchanges the one-time code for the App's credentials. The
// request needs no token: the code is the proof, and it works once.
func convertManifest(ctx context.Context, client *http.Client, api, code string) (GitHubApp, error) {
	var app GitHubApp
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api+"/app-manifests/"+url.PathEscape(code)+"/conversions", nil)
	if err != nil {
		return app, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := client.Do(req)
	if err != nil {
		return app, fmt.Errorf("asking GitHub for the App's credentials: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return app, err
	}
	if res.StatusCode != http.StatusCreated {
		return app, fmt.Errorf("GitHub refused to hand over the App's credentials (HTTP %d). The code works once and for one hour; run this again", res.StatusCode)
	}
	if err := json.Unmarshal(data, &app); err != nil {
		return app, errors.New("GitHub's answer was not understood")
	}
	if app.ID <= 0 || app.Slug == "" || app.ClientID == "" || app.ClientSecret == "" ||
		!strings.HasPrefix(strings.TrimSpace(app.PEM), "-----BEGIN") {
		return app, errors.New("GitHub's answer lacks part of the App's credentials")
	}
	return app, nil
}

// appendEnv adds the App to the .env, written whole beside it and renamed over
// it, so an interrupted write leaves the previous file.
func appendEnv(path string, current []byte, app GitHubApp) error {
	// One line: the engine turns a literal \n back into a newline. Single
	// quotes keep compose from reading the backslashes.
	pem := strings.ReplaceAll(strings.TrimSpace(strings.ReplaceAll(app.PEM, "\r\n", "\n")), "\n", `\n`)
	var b strings.Builder
	b.Write(current)
	if len(current) > 0 && !strings.HasSuffix(string(current), "\n") {
		b.WriteString("\n")
	}
	b.WriteString("# GitHub App of this installation, created by sentinel github\n")
	fmt.Fprintf(&b, "GITHUB_APP_ID=%s\n", strconv.FormatInt(app.ID, 10))
	fmt.Fprintf(&b, "GITHUB_APP_SLUG=%s\n", app.Slug)
	fmt.Fprintf(&b, "GITHUB_APP_PRIVATE_KEY='%s'\n", pem)
	fmt.Fprintf(&b, "GITHUB_APP_USER_CLIENT_ID=%s\n", app.ClientID)
	fmt.Fprintf(&b, "GITHUB_APP_USER_CLIENT_SECRET=%s\n", app.ClientSecret)
	if app.WebhookSecret != "" {
		fmt.Fprintf(&b, "GITHUB_WEBHOOK_SECRET=%s\n", app.WebhookSecret)
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// envValue is the value of name in a .env, or "".
func envValue(env, name string) string {
	for _, line := range strings.Split(env, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), name+"="); ok {
			return strings.Trim(strings.TrimSpace(value), `'"`)
		}
	}
	return ""
}
