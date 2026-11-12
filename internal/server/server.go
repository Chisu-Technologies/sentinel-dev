// Package server is the HTTP client of a Sentinel server. It only asks the
// server to do work and reads back what the server says; none of the work
// happens here.
//
// The API key travels in one header and goes nowhere else: redirects are not
// followed, every string that reaches the user is scrubbed of the key first,
// and nothing returned by the server is trusted as a URL path or a terminal
// control sequence.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxBody bounds what is read from any response, so a wrong or hostile
	// endpoint cannot make the client hold unbounded memory.
	maxBody = 1 << 20
	// MaxDetail is the longest server message shown to the user.
	MaxDetail = 300
)

// Terminal run statuses. Any other status means the run is still going.
const (
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
	StatusFailed    = "failed"
	StatusStopped   = "stopped"
	StatusCancelled = "cancelled"
	StatusZombie    = "zombie"
)

// IsTerminal reports whether a run with this status will not change again.
func IsTerminal(status string) bool {
	switch status {
	case StatusCompleted, StatusBlocked, StatusFailed, StatusStopped, StatusCancelled, StatusZombie:
		return true
	}
	return false
}

// A run id is put in a URL path, so only a conservative alphabet is accepted:
// a "/" or ".." from the server must not steer a later request elsewhere.
var runID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidRunID reports whether id is safe to use in a URL path.
func ValidRunID(id string) bool { return runID.MatchString(id) }

// APIError is a non-success answer of the server.
type APIError struct {
	Status int
	Code   string
	Detail string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("the server answered %d %s", e.Status, http.StatusText(e.Status))
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.Code != "" {
		msg += " (" + e.Code + ")"
	}
	if e.Status == http.StatusUnauthorized {
		msg += "; check SENTINEL_API_KEY"
	}
	return msg
}

// Client talks to one server with one key.
type Client struct {
	base      string
	host      string
	key       string
	userAgent string
	http      *http.Client
}

// ParseURL checks a server address and returns it without a trailing slash and
// its host. https is required; http is accepted only for a loopback host, where
// the key never crosses a network. Credentials, a query and a fragment are
// refused because the key has one place to live. The error never repeats the
// address: it may carry credentials.
func ParseURL(raw string) (base, host string, err error) {
	if strings.ContainsAny(raw, "?#") {
		return "", "", errors.New("the server address must not have a query or a fragment")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", "", errors.New("the server address is not a valid URL")
	}
	if u.User != nil {
		return "", "", errors.New("the server address must not carry a user name or password")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return "", "", errors.New("the server address must use https; http is accepted only for localhost")
		}
	default:
		return "", "", errors.New("the server address must use https")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u.String(), u.Host, nil
}

func isLoopback(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// New returns a client for the server at rawURL. httpClient is copied and its
// redirect policy replaced: a redirect would send the key to another host.
func New(rawURL, key string, httpClient *http.Client, userAgent string) (*Client, error) {
	base, host, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errors.New("the API key is empty")
	}
	if strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return nil, errors.New("the API key contains a control character")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	hc := *httpClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: base, host: host, key: key, userAgent: userAgent, http: &hc}, nil
}

// Host is the host (and port, if any) of the server.
func (c *Client) Host() string { return c.host }

// Clean makes a string from the server fit to print: control characters
// removed, the key replaced if the server echoed it, cut to limit characters.
// Characters are stripped before the key is searched for, so a key split by
// control characters is still found.
func (c *Client) Clean(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, c.key, "[redacted]")
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit]) + "..."
	}
	return s
}

// CleanText is Clean for text of many lines, such as a run log: newlines and
// tabs stay because they are its layout; every other control character
// (an escape sequence starts with one) is removed. The key is replaced as in
// Clean. There is no length limit: the caller already bounds what it reads.
func (c *Client) CleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	return strings.ReplaceAll(s, c.key, "[redacted]")
}

// Run is the part of a run that the client reads. The pointer fields are null
// on the server until the run has a story, a pull request or an end.
type Run struct {
	ID           string  `json:"id"`
	Repo         string  `json:"repo"`
	Status       string  `json:"status"`
	Mode         string  `json:"mode"`
	Story        *string `json:"story"`
	PRNumber     *string `json:"pr_number"`
	PRURL        *string `json:"pr_url"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	StartedAt    *string `json:"started_at"`
	FinishedAt   *string `json:"finished_at"`
}

// StartRequest asks for the verification of an existing pull request.
type StartRequest struct {
	Repository string
	PullNumber int
	// BaseRef is sent only when not empty.
	BaseRef string
}

// Evidence is what the server attached to a story as proof.
type Evidence struct {
	VerifyVerdict string   `json:"verify_verdict"`
	TestsPassed   *float64 `json:"tests_passed"`
	TestsTotal    *float64 `json:"tests_total"`
}

// Story is the result of a run.
type Story struct {
	State         string    `json:"state"`
	Title         string    `json:"title"`
	PRURL         *string   `json:"pr_url"`
	CostUSD       float64   `json:"cost_usd"`
	BlockedReason *string   `json:"blocked_reason"`
	BlockedDetail *string   `json:"blocked_detail"`
	Evidence      *Evidence `json:"evidence"`
	DiffFiles     float64   `json:"diff_files"`
	DiffAdded     float64   `json:"diff_added"`
	DiffRemoved   float64   `json:"diff_removed"`
}

const verdictPrefix = "Sentinel verification verdict: "

// Verdict says how the verification ended: the verdict the server recorded as
// evidence, else APPROVED for a story that is done, else the verdict the server
// gave as the reason a story is blocked, else UNKNOWN.
func (s Story) Verdict() string {
	if s.Evidence != nil && s.Evidence.VerifyVerdict != "" {
		return s.Evidence.VerifyVerdict
	}
	if s.State == "done" {
		return "APPROVED"
	}
	if s.BlockedDetail != nil {
		if rest, ok := strings.CutPrefix(*s.BlockedDetail, verdictPrefix); ok && rest != "" {
			return rest
		}
	}
	return "UNKNOWN"
}

// StartVerify creates the run. The id the server returns is checked here, so
// no caller can put an unchecked one in a path.
func (c *Client) StartVerify(ctx context.Context, req StartRequest) (Run, error) {
	body := map[string]any{"repo": req.Repository, "mode": "verify_pr", "pr_number": req.PullNumber}
	if req.BaseRef != "" {
		body["base_ref"] = req.BaseRef
	}
	var run Run
	if err := c.do(ctx, http.MethodPost, "/api/runs", body, http.StatusCreated, &run); err != nil {
		return Run{}, err
	}
	if !ValidRunID(run.ID) {
		return Run{}, errors.New("the server returned a run id that is not valid")
	}
	return run, nil
}

// Run reads the current state of a run.
func (c *Client) Run(ctx context.Context, id string) (Run, error) {
	if !ValidRunID(id) {
		return Run{}, errors.New("not a valid run id")
	}
	var run Run
	err := c.do(ctx, http.MethodGet, "/api/runs/"+id, nil, http.StatusOK, &run)
	return run, err
}

// Runs lists the runs, newest first.
func (c *Client) Runs(ctx context.Context, limit, offset int) ([]Run, error) {
	query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var runs []Run
	err := c.do(ctx, http.MethodGet, "/api/runs?"+query.Encode(), nil, http.StatusOK, &runs)
	return runs, err
}

// Log reads the most recent lines of a run's log, as the server bounds them.
// found is false when the run has no log yet, which is not an error: a run
// that has just started has none. The text is returned as received; the caller
// passes it through CleanText before showing it.
func (c *Client) Log(ctx context.Context, id string) (text string, found bool, err error) {
	if !ValidRunID(id) {
		return "", false, errors.New("not a valid run id")
	}
	status, data, tooLarge, err := c.fetch(ctx, http.MethodGet, "/api/runs/"+id+"/log", nil, "text/plain")
	if err != nil {
		return "", false, err
	}
	switch {
	case status == http.StatusNotFound:
		return "", false, nil
	case status != http.StatusOK:
		return "", false, c.apiError(status, data, tooLarge)
	case tooLarge:
		return "", false, errors.New("the answer of the server is larger than 1 MiB")
	}
	return string(data), true, nil
}

// Stories reads the stories of a run.
func (c *Client) Stories(ctx context.Context, id string) ([]Story, error) {
	if !ValidRunID(id) {
		return nil, errors.New("not a valid run id")
	}
	var stories []Story
	err := c.do(ctx, http.MethodGet, "/api/runs/"+id+"/stories", nil, http.StatusOK, &stories)
	return stories, err
}

// fetch sends one request and reads at most maxBody+1 bytes of the answer, so
// every endpoint shares the same key handling and the same bound.
func (c *Client) fetch(ctx context.Context, method, path string, body any, accept string) (status int, data []byte, tooLarge bool, err error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, false, err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
	if err != nil {
		return 0, nil, false, c.scrub(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, false, c.scrub(err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return 0, nil, false, c.scrub(err)
	}
	return resp.StatusCode, data, len(data) > maxBody, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, want int, out any) error {
	status, data, tooLarge, err := c.fetch(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	if status != want {
		return c.apiError(status, data, tooLarge)
	}
	if tooLarge {
		return errors.New("the answer of the server is larger than 1 MiB")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("the answer of the server is not the JSON that was expected")
	}
	return nil
}

// scrub removes the key from a transport error's text and keeps the cause, so
// the caller can still tell a timeout from a refusal.
func (c *Client) scrub(err error) error {
	return &cleanError{msg: c.Clean(err.Error(), MaxDetail), cause: err}
}

type cleanError struct {
	msg   string
	cause error
}

func (e *cleanError) Error() string { return e.msg }
func (e *cleanError) Unwrap() error { return e.cause }

func (c *Client) apiError(status int, data []byte, tooLarge bool) error {
	e := &APIError{Status: status}
	if status >= 300 && status < 400 {
		// The redirect target is not shown: it is the word of the server, and
		// it is the place the key would have gone.
		e.Detail = "redirects are not followed"
		return e
	}
	if tooLarge {
		return e
	}
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Detail) == 0 {
		return e
	}
	var text string
	var object struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	switch {
	case json.Unmarshal(envelope.Detail, &text) == nil:
		e.Detail = c.Clean(text, MaxDetail)
	case json.Unmarshal(envelope.Detail, &object) == nil && (object.Message != "" || object.Code != ""):
		e.Detail = c.Clean(object.Message, MaxDetail)
		e.Code = c.Clean(object.Code, 64)
	default:
		// Some other shape, such as a list of validation problems.
		e.Detail = c.Clean(string(envelope.Detail), MaxDetail)
	}
	return e
}
