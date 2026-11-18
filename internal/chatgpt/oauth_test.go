package chatgpt

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOpenAI plays OpenAI's authorization server: discovery, keys, the
// authorization page (as the person's browser would follow it) and the token
// endpoint, which checks PKCE.
type fakeOpenAI struct {
	*httptest.Server
	key        *rsa.PrivateKey
	signer     *rsa.PrivateKey // nil means key
	mu         sync.Mutex
	challenge  string
	nonce      string
	scope      string // granted scope; empty means everything asked
	issued     string // client id issued on registration
	tokenForms []url.Values
	tamper     func(claims map[string]any)
}

func newFake(t *testing.T) *fakeOpenAI {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeOpenAI{key: key, issued: "oaiapp_test123"}
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
		f.tokenForms = append(f.tokenForms, r.PostForm)
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		scope := f.scope
		if scope == "" {
			scope = Scope
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-secret", "refresh_token": "rt-secret", "token_type": "Bearer",
			"expires_in": 3600, "scope": scope, "id_token": f.idToken(r.PostForm.Get("client_id")),
		})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeOpenAI) idToken(aud string) string {
	claims := map[string]any{"iss": f.URL, "sub": "user-1", "aud": aud, "email": "p@example.com",
		"exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce}
	if f.tamper != nil {
		f.tamper(claims)
	}
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	signing := enc(map[string]string{"alg": "RS256", "kid": "k1"}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	signer := f.key
	if f.signer != nil {
		signer = f.signer
	}
	sig, _ := rsa.SignPKCS1v15(rand.Reader, signer, crypto.SHA256, sum[:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// browser follows the authorization page: the person signs in and consents,
// and OpenAI redirects to the loopback callback.
func (f *fakeOpenAI) browser(t *testing.T, opened *url.Values, answer func(q url.Values) url.Values) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		*opened = q
		f.mu.Lock()
		f.challenge, f.nonce = q.Get("code_challenge"), q.Get("nonce")
		f.mu.Unlock()
		back := url.Values{"code": {"the-code"}, "state": {q.Get("state")}, "client_id": {f.issued}}
		if answer != nil {
			back = answer(back)
		}
		go http.Get(q.Get("redirect_uri") + "?" + back.Encode())
		return nil
	}
}

func signIn(t *testing.T, f *fakeOpenAI, flow Flow) (Credentials, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	flow.AuthBase, flow.HTTP = f.URL, f.Client()
	if flow.HostID == "" {
		flow.HostID = "urn:uuid:00000000-0000-4000-8000-000000000001"
	}
	return flow.SignIn(ctx)
}

func TestFirstSignInRegistersAClientAndKeepsTheIssuedID(t *testing.T) {
	f := newFake(t)
	var opened url.Values
	creds, err := signIn(t, f, Flow{AppName: "Sentinel", Open: f.browser(t, &opened, nil)})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"client_id": "dynamic_agent_client", "agent_name_hint": "Sentinel", "response_type": "code",
		"scope": Scope, "resource": Resource, "code_challenge_method": "S256",
		"ext_agent_host_id": "urn:uuid:00000000-0000-4000-8000-000000000001",
	} {
		if got := opened.Get(name); got != want {
			t.Errorf("authorize %s = %q, want %q", name, got, want)
		}
	}
	if !strings.HasPrefix(opened.Get("redirect_uri"), "http://127.0.0.1:") || !strings.HasSuffix(opened.Get("redirect_uri"), "/auth/callback") {
		t.Errorf("redirect_uri = %q", opened.Get("redirect_uri"))
	}
	if creds.ClientID != "oaiapp_test123" || creds.Subject != "user-1" || creds.RefreshToken != "rt-secret" {
		t.Errorf("creds = %+v", creds)
	}
	form := f.tokenForms[0]
	if form.Get("client_id") != "oaiapp_test123" || form.Get("redirect_uri") != opened.Get("redirect_uri") || form.Get("resource") != Resource {
		t.Errorf("token form = %v", form)
	}
}

func TestAReauthorizationUsesTheSavedClientAndRefusesAnotherOne(t *testing.T) {
	f := newFake(t)
	var opened url.Values
	_, err := signIn(t, f, Flow{ClientID: "oaiapp_saved", Open: f.browser(t, &opened, func(q url.Values) url.Values {
		q.Set("client_id", "oaiapp_other")
		return q
	})})
	if opened.Get("client_id") != "oaiapp_saved" || opened.Has("agent_name_hint") {
		t.Errorf("a reauthorization should name the saved client and no app name: %v", opened)
	}
	if err == nil || !strings.Contains(err.Error(), "different client") {
		t.Errorf("err = %v, want a different-client refusal", err)
	}
}

func TestTheSignInIsRefusedWhenAnyCheckFails(t *testing.T) {
	cases := map[string]struct {
		answer func(url.Values) url.Values
		setup  func(*fakeOpenAI)
		want   string
	}{
		"state":     {answer: func(q url.Values) url.Values { q.Set("state", "x"); return q }, want: "state mismatch"},
		"declined":  {answer: func(q url.Values) url.Values { q.Set("error", "access_denied"); return q }, want: "declined"},
		"no client": {answer: func(q url.Values) url.Values { q.Del("client_id"); return q }, want: "no client was issued"},
		"no plan":   {setup: func(f *fakeOpenAI) { f.scope = "openid profile email offline_access" }, want: "did not grant"},
		"nonce":     {setup: func(f *fakeOpenAI) { f.tamper = func(c map[string]any) { c["nonce"] = "x" } }, want: "nonce"},
		"audience":  {setup: func(f *fakeOpenAI) { f.tamper = func(c map[string]any) { c["aud"] = "someone-else" } }, want: "another client"},
		"expired": {setup: func(f *fakeOpenAI) {
			f.tamper = func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }
		}, want: "expired"},
		"issuer": {setup: func(f *fakeOpenAI) { f.tamper = func(c map[string]any) { c["iss"] = "https://evil.example" } }, want: "issuer"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			var opened url.Values
			_, err := signIn(t, f, Flow{AppName: "Sentinel", Open: f.browser(t, &opened, tc.answer)})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Errorf("a token reached the error: %v", err)
			}
		})
	}
}

func TestABadSignatureIsRefused(t *testing.T) {
	f := newFake(t)
	f.signer, _ = rsa.GenerateKey(rand.Reader, 2048) // not the key the JWKS publishes
	var opened url.Values
	_, err := signIn(t, f, Flow{AppName: "Sentinel", Open: f.browser(t, &opened, nil)})
	if err == nil || !strings.Contains(err.Error(), "bad signature") {
		t.Fatalf("err = %v, want a bad-signature refusal", err)
	}
}

func TestNewHostIDIsARandomUUIDURN(t *testing.T) {
	a, _ := NewHostID()
	b, _ := NewHostID()
	if a == b || !strings.HasPrefix(a, "urn:uuid:") || len(a) != len("urn:uuid:")+36 {
		t.Errorf("host ids %q %q", a, b)
	}
}
