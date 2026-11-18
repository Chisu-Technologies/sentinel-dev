// Package chatgpt signs a person in with ChatGPT so Sentinel can use their
// ChatGPT plan, following OpenAI's "token sharing for open-source apps" flow:
// a dynamic client registration, PKCE with state and nonce, a loopback
// callback on 127.0.0.1, the code exchange, and the checks on the ID token and
// on the granted permission. It never sees the person's password: that is
// typed in OpenAI's own page, in their browser.
package chatgpt

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultAuthBase is OpenAI's authorization server.
	DefaultAuthBase = "https://auth.openai.com"
	// Resource is the API the tokens are for.
	Resource = "https://api.openai.com/v1"
	// Scope is what the sign-in asks for; PlanScope is the part that allows
	// plan usage, and a token without it cannot be used.
	Scope     = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	PlanScope = "chatgpt.tokens.use.direct"
	// dynamicClient asks the server to register a client on first sign-in.
	dynamicClient = "dynamic_agent_client"
	callbackPath  = "/auth/callback"
	// preferredPort is used when free; only the port may change between sign-ins.
	preferredPort = 1455
)

// Credentials is what a sign-in leaves, in the fields OpenAI documents for the
// saved record. It holds secrets: never print or log it.
type Credentials struct {
	Issuer       string    `json:"issuer"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email,omitempty"`
	ClientID     string    `json:"client_id"`
	HostID       string    `json:"ext_agent_host_id"`
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	Scopes       []string  `json:"scopes"`
	SavedAt      time.Time `json:"saved_at"`
}

// Flow is one sign-in attempt.
type Flow struct {
	// AuthBase is the authorization server; empty means DefaultAuthBase.
	AuthBase string
	// AppName is shown to the person on first registration.
	AppName string
	// HostID is this installation's stable, opaque ext_agent_host_id.
	HostID string
	// ClientID is the client issued by an earlier sign-in, empty the first time.
	ClientID string
	HTTP     *http.Client
	// Open shows the authorization page to the person (their browser).
	Open func(authorizeURL string) error
	// Now is the clock for the ID token checks; nil means time.Now.
	Now func() time.Time
}

// NewHostID returns a fresh opaque host id (a random UUID URN).
func NewHostID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

type callback struct {
	code, state, clientID, errCode string
}

// SignIn runs the flow until the person finishes in the browser, ctx ends,
// or a check fails.
func (f Flow) SignIn(ctx context.Context) (Credentials, error) {
	if f.HostID == "" {
		return Credentials{}, errors.New("a host id is needed before the first sign-in")
	}
	base := strings.TrimRight(f.AuthBase, "/")
	if base == "" {
		base = DefaultAuthBase
	}
	client := f.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	state, err := randomToken()
	if err != nil {
		return Credentials{}, err
	}
	nonce, err := randomToken()
	if err != nil {
		return Credentials{}, err
	}
	verifier, err := randomToken()
	if err != nil {
		return Credentials{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	// The listener comes first, so the browser's redirect always finds it.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return Credentials{}, fmt.Errorf("opening the local sign-in callback: %w", err)
		}
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d%s", ln.Addr().(*net.TCPAddr).Port, callbackPath)

	got := make(chan callback, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callbackPath {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		select {
		case got <- callback{q.Get("code"), q.Get("state"), q.Get("client_id"), q.Get("error")}:
		default:
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "Sentinel received the sign-in. You can close this tab and go back to the terminal.\n")
	})}
	go srv.Serve(ln)
	defer srv.Close()

	clientID := f.ClientID
	params := url.Values{
		"ext_agent_host_id":     {f.HostID},
		"response_type":         {"code"},
		"redirect_uri":          {redirect},
		"scope":                 {Scope},
		"resource":              {Resource},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
	}
	if clientID == "" {
		params.Set("client_id", dynamicClient)
		params.Set("agent_name_hint", f.AppName)
	} else {
		params.Set("client_id", clientID)
	}
	if err := f.Open(base + "/api/accounts/authorize?" + params.Encode()); err != nil {
		return Credentials{}, err
	}

	var cb callback
	select {
	case cb = <-got:
	case <-ctx.Done():
		return Credentials{}, fmt.Errorf("the sign-in was not finished: %w", ctx.Err())
	}
	if cb.state != state {
		return Credentials{}, errors.New("the sign-in answer does not belong to this attempt (state mismatch)")
	}
	if cb.errCode != "" {
		if cb.errCode == "access_denied" {
			return Credentials{}, errors.New("the sign-in was declined in the browser")
		}
		return Credentials{}, fmt.Errorf("the sign-in failed: %s", cb.errCode)
	}
	switch {
	case clientID == "" && cb.clientID == "":
		return Credentials{}, errors.New("the registration did not finish: no client was issued")
	case clientID == "":
		clientID = cb.clientID
	case cb.clientID != "" && cb.clientID != clientID:
		return Credentials{}, errors.New("the sign-in answered for a different client")
	}
	if cb.code == "" {
		return Credentials{}, errors.New("the sign-in answer has no code")
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {cb.code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirect},
		"resource":      {Resource},
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := postForm(ctx, client, base+"/api/accounts/oauth/token", form, &tok); err != nil {
		return Credentials{}, err
	}
	scopes := strings.Fields(tok.Scope)
	if !contains(scopes, PlanScope) {
		return Credentials{}, errors.New("the sign-in did not grant use of the ChatGPT plan")
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return Credentials{}, errors.New("the sign-in answer is missing a token")
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	claims, err := verifyIDToken(ctx, client, base, tok.IDToken, clientID, nonce, now())
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{
		Issuer: claims.Issuer, Subject: claims.Subject, Email: claims.Email,
		ClientID: clientID, HostID: f.HostID,
		IDToken: tok.IDToken, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		TokenType: tok.TokenType, ExpiresIn: tok.ExpiresIn, Scopes: scopes, SavedAt: now().UTC(),
	}, nil
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func postForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reaching OpenAI: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// Only the error code is repeated: the body could hold a token.
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("OpenAI refused the code exchange: %s", e.Error)
	}
	return json.Unmarshal(body, out)
}

type idClaims struct {
	Issuer   string          `json:"iss"`
	Subject  string          `json:"sub"`
	Audience json.RawMessage `json:"aud"`
	Expiry   int64           `json:"exp"`
	Nonce    string          `json:"nonce"`
	Email    string          `json:"email"`
}

func (c idClaims) hasAudience(want string) bool {
	var one string
	if json.Unmarshal(c.Audience, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(c.Audience, &many) == nil {
		return contains(many, want)
	}
	return false
}

// verifyIDToken checks the signature against the server's published keys, and
// the issuer, audience, expiry and nonce.
func verifyIDToken(ctx context.Context, client *http.Client, base, token, clientID, nonce string, now time.Time) (idClaims, error) {
	fail := func(why string) (idClaims, error) {
		return idClaims{}, fmt.Errorf("the ID token from OpenAI is not valid: %s", why)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fail("not a JWT")
	}
	headerJSON, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	payloadJSON, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err3 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return fail("not base64url")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(headerJSON, &header) != nil || header.Alg != "RS256" {
		return fail("unsupported signature algorithm")
	}
	key, err := signingKey(ctx, client, base, header.Kid)
	if err != nil {
		return idClaims{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return fail("bad signature")
	}
	var c idClaims
	if json.Unmarshal(payloadJSON, &c) != nil {
		return fail("unreadable claims")
	}
	switch {
	case c.Issuer != base:
		return fail("wrong issuer")
	case !c.hasAudience(clientID):
		return fail("issued for another client")
	case c.Expiry <= now.Unix():
		return fail("expired")
	case c.Nonce != nonce:
		return fail("nonce mismatch")
	case c.Subject == "":
		return fail("no subject")
	}
	return c, nil
}

func signingKey(ctx context.Context, client *http.Client, base, kid string) (*rsa.PublicKey, error) {
	var discovery struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := getJSON(ctx, client, base+"/.well-known/openid-configuration", &discovery); err != nil {
		return nil, err
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := getJSON(ctx, client, discovery.JWKSURI, &set); err != nil {
		return nil, err
	}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (kid != "" && k.Kid != kid) {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	}
	return nil, errors.New("OpenAI did not publish the key that signed the ID token")
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reaching OpenAI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAI answered %s for %s", resp.Status, endpoint)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
