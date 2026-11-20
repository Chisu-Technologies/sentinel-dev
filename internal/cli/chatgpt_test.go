package cli

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

const (
	chatgptAccess  = "chatgpt-access-secret"
	chatgptRefresh = "chatgpt-refresh-secret"
)

// fakeOpenAIAuth plays OpenAI's authorization server for `login --chatgpt`.
type fakeOpenAIAuth struct {
	*httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	challenge string
	nonce     string
	opened    []url.Values
	// refusals is how many code exchanges answer invalid_grant before one succeeds.
	refusals int
}

func newFakeOpenAIAuth(t *testing.T) *fakeOpenAIAuth {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeOpenAIAuth{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"jwks_uri": f.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/api/accounts/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.refusals > 0 {
			f.refusals--
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
		signing := enc(map[string]string{"alg": "RS256", "kid": "k1"}) + "." + enc(map[string]any{
			"iss": f.URL, "sub": "user-1", "aud": r.PostForm.Get("client_id"), "email": "p@example.com",
			"exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce,
		})
		digest := sha256.Sum256([]byte(signing))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": chatgptAccess, "refresh_token": chatgptRefresh, "token_type": "Bearer",
			"expires_in": 3600, "scope": "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
			"id_token": signing + "." + base64.RawURLEncoding.EncodeToString(sig),
		})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// browser stands for the person's browser: asked to open the page, it signs in
// and follows OpenAI's redirect to the loopback callback.
type browser struct{ auth *fakeOpenAIAuth }

func (b *browser) LookPath(name string) (string, error) { return "/usr/bin/" + name, nil }

func (b *browser) Run(_ context.Context, cmd execx.Command) (execx.Result, error) {
	page, err := url.Parse(cmd.Args[len(cmd.Args)-1])
	if err != nil {
		return execx.Result{}, err
	}
	q := page.Query()
	b.auth.mu.Lock()
	b.auth.challenge, b.auth.nonce = q.Get("code_challenge"), q.Get("nonce")
	b.auth.opened = append(b.auth.opened, q)
	b.auth.mu.Unlock()
	client := q.Get("client_id")
	if client == "dynamic_agent_client" {
		client = "oaiapp_issued"
	}
	back := url.Values{"code": {"the-code"}, "state": {q.Get("state")}, "client_id": {client}}
	go http.Get(q.Get("redirect_uri") + "?" + back.Encode())
	return execx.Result{}, nil
}

// fakeSentinel keeps what `login --chatgpt` hands it.
func fakeSentinel(t *testing.T, received *[]map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/org/inference/chatgpt-session" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk_sentinel_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		json.Unmarshal(body, &got)
		*received = append(*received, got)
		io.WriteString(w, `{"status":"signed_in","account":"p@example.com"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func chatgptLogin(t *testing.T, auth *fakeOpenAIAuth, sentinel, configDir string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	vars := map[string]string{"SENTINEL_URL": sentinel, "SENTINEL_API_KEY": "sk_sentinel_key"}
	code := Run(context.Background(), []string{"login", "--chatgpt"}, Env{
		Stdout:          &stdout,
		Stderr:          &stderr,
		Runner:          &browser{auth: auth},
		Build:           buildinfo.Info{Version: "v0.4.0", OS: "linux", Arch: "amd64"},
		Getenv:          func(name string) string { return vars[name] },
		ConfigDir:       configDir,
		ChatGPTAuthBase: auth.URL,
	})
	return result{code, stdout.String(), stderr.String()}
}

func TestLoginWithChatGPTHandsTheSignInToTheServerAndPrintsNoToken(t *testing.T) {
	auth := newFakeOpenAIAuth(t)
	var received []map[string]any
	sentinel := fakeSentinel(t, &received)
	dir := t.TempDir()

	got := chatgptLogin(t, auth, sentinel.URL, dir)

	if got.code != ExitOK {
		t.Fatalf("got %+v", got)
	}
	if len(received) != 1 {
		t.Fatalf("the server received %d sign-ins", len(received))
	}
	want := map[string]any{"access_token": chatgptAccess, "refresh_token": chatgptRefresh,
		"expires_in": float64(3600), "client_id": "oaiapp_issued", "account": "p@example.com"}
	for name, value := range want {
		if received[0][name] != value {
			t.Errorf("%s = %v, want %v", name, received[0][name], value)
		}
	}
	if strings.Contains(got.stdout+got.stderr, chatgptAccess) || strings.Contains(got.stdout+got.stderr, chatgptRefresh) {
		t.Error("a token was printed")
	}
	if !strings.Contains(got.stdout, "Signed in with ChatGPT as p@example.com") {
		t.Errorf("stdout = %s", got.stdout)
	}
	// What stays on this machine names the installation and the issued client:
	// no token at all.
	kept, err := os.ReadFile(filepath.Join(dir, "sentinel", "chatgpt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(kept), "secret") || !strings.Contains(string(kept), "oaiapp_issued") ||
		!strings.Contains(string(kept), "urn:uuid:") {
		t.Errorf("kept %s", kept)
	}
}

func TestASecondChatGPTSignInReusesTheHostAndTheIssuedClient(t *testing.T) {
	auth := newFakeOpenAIAuth(t)
	var received []map[string]any
	sentinel := fakeSentinel(t, &received)
	dir := t.TempDir()

	first := chatgptLogin(t, auth, sentinel.URL, dir)
	second := chatgptLogin(t, auth, sentinel.URL, dir)

	if first.code != ExitOK || second.code != ExitOK {
		t.Fatalf("first %+v second %+v", first, second)
	}
	if auth.opened[0].Get("client_id") != "dynamic_agent_client" || auth.opened[1].Get("client_id") != "oaiapp_issued" {
		t.Errorf("client ids %q then %q", auth.opened[0].Get("client_id"), auth.opened[1].Get("client_id"))
	}
	if auth.opened[0].Get("ext_agent_host_id") != auth.opened[1].Get("ext_agent_host_id") {
		t.Error("the host id changed between sign-ins")
	}
}

func TestLoginWithChatGPTNeedsASentinelToHandItTo(t *testing.T) {
	auth := newFakeOpenAIAuth(t)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"login", "--chatgpt"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &browser{auth: auth},
		Getenv: func(string) string { return "" }, ConfigDir: t.TempDir(), ChatGPTAuthBase: auth.URL,
	})
	if code != ExitFailed || len(auth.opened) != 0 {
		t.Fatalf("code %d, opened %d pages; stderr %s", code, len(auth.opened), stderr.String())
	}
}

// OpenAI registers the client and then refuses the exchange with a bare invalid_grant when the
// person's workspace and plan cannot lend ChatGPT plan usage. Its documentation says to keep the
// issued client and start again with it; only that returning sign-in shows the person why.
func TestARefusedExchangeKeepsTheIssuedClientAndSaysWhatToCheck(t *testing.T) {
	auth := newFakeOpenAIAuth(t)
	auth.refusals = 1
	var received []map[string]any
	sentinel := fakeSentinel(t, &received)
	dir := t.TempDir()

	first := chatgptLogin(t, auth, sentinel.URL, dir)
	if first.code != ExitFailed || len(received) != 0 {
		t.Fatalf("first %+v, server received %d", first, len(received))
	}
	if !strings.Contains(first.stderr, "invalid_grant") || !strings.Contains(first.stderr, "ChatGPT Plus or Pro") {
		t.Errorf("stderr = %s", first.stderr)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "sentinel", "chatgpt.json"))
	if err != nil || !strings.Contains(string(kept), "oaiapp_issued") {
		t.Fatalf("kept %s (%v)", kept, err)
	}

	second := chatgptLogin(t, auth, sentinel.URL, dir)
	if second.code != ExitOK || len(received) != 1 {
		t.Fatalf("second %+v", second)
	}
	if got := auth.opened[1].Get("client_id"); got != "oaiapp_issued" {
		t.Errorf("the second sign-in asked as %q, not the issued client", got)
	}
}
