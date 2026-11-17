package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// LoginStart is a sign-in waiting for a person to approve it in the panel.
type LoginStart struct {
	// DeviceCode proves to the server that this command started the sign-in.
	// It is never printed.
	DeviceCode string `json:"device_code"`
	// UserCode is what the person compares in the panel.
	UserCode  string `json:"user_code"`
	ExpiresIn int    `json:"expires_in"`
	Interval  int    `json:"interval"`
}

// LoginKey is what an approved sign-in hands over, once.
type LoginKey struct {
	APIKey       string `json:"api_key"`
	Organization string `json:"organization"`
}

var (
	// ErrLoginPending means nobody approved the sign-in yet.
	ErrLoginPending = errors.New("waiting for approval in the panel")
	// ErrLoginDenied means the sign-in was denied in the panel.
	ErrLoginDenied = errors.New("the sign-in was denied in the panel")
	// ErrLoginExpired means the sign-in expired or was already used.
	ErrLoginExpired = errors.New("the sign-in expired; run sentinel login again")
)

var userCode = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)

// LoginClient talks to a server before there is a key: it sends no
// Authorization header and, like Client, follows no redirect.
type LoginClient struct {
	base, host, userAgent string
	http                  *http.Client
}

// NewLogin returns a sign-in client for the server at rawURL, with the same
// address rules as New.
func NewLogin(rawURL string, httpClient *http.Client, userAgent string) (*LoginClient, error) {
	base, host, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	hc := *httpClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &LoginClient{base: base, host: host, userAgent: userAgent, http: &hc}, nil
}

// Base is the server address the sign-in is for.
func (l *LoginClient) Base() string { return l.base }

// Host is the host (and port, if any) of the server.
func (l *LoginClient) Host() string { return l.host }

// ApprovalURL is the panel page where the person approves code. It is built
// here from the address the user gave, never taken from the server.
func (l *LoginClient) ApprovalURL(code string) string {
	return l.base + "/cli-login/" + code
}

// Start asks the server for a sign-in. client says what this command is, so the
// person who approves can recognize it.
func (l *LoginClient) Start(ctx context.Context, client string) (LoginStart, error) {
	status, data, err := l.post(ctx, "/api/cli/login", map[string]string{"client": client})
	if err != nil {
		return LoginStart{}, err
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
			return LoginStart{}, errors.New("this server does not support sentinel login yet; set SENTINEL_URL and SENTINEL_API_KEY instead")
		}
		return LoginStart{}, loginError(status, data)
	}
	var start LoginStart
	if json.Unmarshal(data, &start) != nil || len(start.DeviceCode) < 32 || !userCode.MatchString(start.UserCode) {
		return LoginStart{}, errors.New("the answer of the server is not the sign-in that was expected")
	}
	return start, nil
}

// Collect asks whether the sign-in was approved. It returns ErrLoginPending
// until it was, and the key on the first call after.
func (l *LoginClient) Collect(ctx context.Context, deviceCode string) (LoginKey, error) {
	status, data, err := l.post(ctx, "/api/cli/login/token", map[string]string{"device_code": deviceCode})
	if err != nil {
		return LoginKey{}, err
	}
	if status != http.StatusOK {
		var envelope struct {
			Detail struct {
				Code string `json:"code"`
			} `json:"detail"`
		}
		_ = json.Unmarshal(data, &envelope)
		switch envelope.Detail.Code {
		case "authorization_pending":
			return LoginKey{}, ErrLoginPending
		case "access_denied":
			return LoginKey{}, ErrLoginDenied
		case "expired_token":
			return LoginKey{}, ErrLoginExpired
		}
		return LoginKey{}, loginError(status, data)
	}
	var key LoginKey
	if json.Unmarshal(data, &key) != nil || !strings.HasPrefix(key.APIKey, "sk-") || strings.IndexFunc(key.APIKey, unicode.IsControl) >= 0 {
		return LoginKey{}, errors.New("the answer of the server is not the key that was expected")
	}
	return key, nil
}

func (l *LoginClient) post(ctx context.Context, path string, body any) (int, []byte, error) {
	// The same bounded, key-free request shape as Client.fetch, without the
	// Authorization header: there is no key yet.
	c := &Client{base: l.base, host: l.host, userAgent: l.userAgent, http: l.http}
	status, data, tooLarge, err := c.fetch(ctx, http.MethodPost, path, body, "application/json")
	if err == nil && tooLarge {
		return status, nil, errors.New("the answer of the server is larger than 1 MiB")
	}
	return status, data, err
}

func loginError(status int, data []byte) error {
	c := &Client{}
	return c.apiError(status, data, len(data) > maxBody)
}
