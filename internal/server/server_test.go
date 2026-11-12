package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testKey = "sk-test-0123456789abcdef"

func newClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := New(url, testKey, nil, "sentinel/v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStartSendsTheExactMethodPathBodyAndHeaders(t *testing.T) {
	var got struct {
		method, path, body string
		header             http.Header
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got.method, got.path, got.body, got.header = r.Method, r.URL.Path, string(data), r.Header
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"run_1","status":"queued","extra":true}`)
	}))
	defer srv.Close()

	run, err := newClient(t, srv.URL+"/").StartVerify(context.Background(), StartRequest{Repository: "acme/widgets", PullNumber: 12})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/api/runs" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if want := `{"mode":"verify_pr","pr_number":12,"repo":"acme/widgets"}`; got.body != want {
		t.Errorf("body = %s, want %s", got.body, want)
	}
	for name, want := range map[string]string{
		"Authorization": "Bearer " + testKey,
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"User-Agent":    "sentinel/v0.0.0-test",
	} {
		if got.header.Get(name) != want {
			t.Errorf("header %s = %q, want %q", name, got.header.Get(name), want)
		}
	}
	if run.ID != "run_1" || run.Status != "queued" {
		t.Errorf("run = %+v", run)
	}
}

func TestStartSendsTheBaseOnlyWhenGiven(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"r","status":"queued"}`)
	}))
	defer srv.Close()
	if _, err := newClient(t, srv.URL).StartVerify(context.Background(), StartRequest{Repository: "a/b", PullNumber: 1, BaseRef: "release"}); err != nil {
		t.Fatal(err)
	}
	if want := `{"base_ref":"release","mode":"verify_pr","pr_number":1,"repo":"a/b"}`; body != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func TestPathPrefixOfTheServerAddressIsKept(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		io.WriteString(w, `{"id":"r","status":"running"}`)
	}))
	defer srv.Close()
	if _, err := newClient(t, srv.URL+"/sentinel/").Run(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	if path != "/sentinel/api/runs/r" {
		t.Errorf("path = %q", path)
	}
}

func TestMapsTheBodiesOfRunAndStories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/runs/abc-1":
			io.WriteString(w, `{"id":"abc-1","status":"completed"}`)
		case "/api/runs/abc-1/stories":
			io.WriteString(w, `[{"state":"done","title":"T","pr_url":null,"cost_usd":0.31,
				"blocked_reason":null,"blocked_detail":null,
				"evidence":{"verify_verdict":"APPROVED","tests_passed":6,"tests_total":6},
				"diff_files":3,"diff_added":40,"diff_removed":2}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := newClient(t, srv.URL)

	run, err := c.Run(context.Background(), "abc-1")
	if err != nil || run.Status != "completed" {
		t.Fatalf("run = %+v, %v", run, err)
	}
	stories, err := c.Stories(context.Background(), "abc-1")
	if err != nil || len(stories) != 1 {
		t.Fatalf("stories = %+v, %v", stories, err)
	}
	s := stories[0]
	if s.PRURL != nil || s.CostUSD != 0.31 || s.DiffFiles != 3 || s.DiffAdded != 40 || s.DiffRemoved != 2 {
		t.Errorf("story = %+v", s)
	}
	if s.Evidence == nil || s.Evidence.VerifyVerdict != "APPROVED" || *s.Evidence.TestsPassed != 6 || *s.Evidence.TestsTotal != 6 {
		t.Errorf("evidence = %+v", s.Evidence)
	}
}

func TestMapsAnErrorBodyWithAStringDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"detail":"Run not found"}`)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Run(context.Background(), "gone")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Detail != "Run not found" || apiErr.Code != "" {
		t.Fatalf("err = %#v", err)
	}
}

func TestMapsAnErrorBodyWithAnObjectDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"detail":{"code":"repo_not_connected","message":"Connect the repository first"}}`)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).StartVerify(context.Background(), StartRequest{Repository: "a/b", PullNumber: 1})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "repo_not_connected" || apiErr.Detail != "Connect the repository first" {
		t.Fatalf("err = %#v", err)
	}
	if !strings.Contains(err.Error(), "Connect the repository first (repo_not_connected)") {
		t.Errorf("message = %q", err)
	}
}

func TestAnUnauthorizedAnswerSaysToCheckTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"detail":"Invalid API key"}`)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Run(context.Background(), "r")
	if err == nil || !strings.Contains(err.Error(), "check SENTINEL_API_KEY") {
		t.Errorf("err = %v, want the hint", err)
	}
}

func TestErrorDetailIsCutAndStrippedOfControlCharacters(t *testing.T) {
	long := strings.Repeat("x", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		body, _ := json.Marshal(map[string]string{"detail": "bad\x1b[31m red\nline\r\x00" + long})
		w.Write(body)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Run(context.Background(), "r")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal(err)
	}
	if strings.ContainsAny(apiErr.Detail, "\x1b\n\r\x00") {
		t.Errorf("detail still has control characters: %q", apiErr.Detail)
	}
	if n := len([]rune(apiErr.Detail)); n > MaxDetail+3 {
		t.Errorf("detail has %d characters, want at most %d", n, MaxDetail)
	}
}

func TestRefusesAddressesThatCouldLeakTheKey(t *testing.T) {
	for _, raw := range []string{
		"http://example.com",
		"http://192.0.2.1",
		"http://localhost.example.com",
		"ftp://example.com",
		"https://user:hunter2@example.com",
		"https://user@example.com",
		"https://example.com?x=1",
		"https://example.com/?",
		"https://example.com#frag",
		"https://example.com#",
		"",
		"example.com",
		"https://",
	} {
		if _, err := New(raw, testKey, nil, "ua"); err == nil {
			t.Errorf("New(%q) accepted the address", raw)
		} else if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("the error repeats the credentials: %v", err)
		}
	}
}

func TestAcceptsHTTPSAndHTTPOnLoopbackAndTrimsOneSlash(t *testing.T) {
	for raw, wantBase := range map[string]string{
		"https://example.com":        "https://example.com",
		"https://example.com/":       "https://example.com",
		"https://example.com/sen/":   "https://example.com/sen",
		"https://example.com:8443/x": "https://example.com:8443/x",
		"http://localhost:8000":      "http://localhost:8000",
		"http://127.0.0.1:8000/":     "http://127.0.0.1:8000",
		"http://[::1]:8000":          "http://[::1]:8000",
	} {
		c, err := New(raw, testKey, nil, "ua")
		if err != nil {
			t.Errorf("New(%q): %v", raw, err)
			continue
		}
		if c.base != wantBase {
			t.Errorf("New(%q).base = %q, want %q", raw, c.base, wantBase)
		}
	}
}

func TestDoesNotFollowARedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/runs", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	// A client that would follow redirects, as http.DefaultClient does: the
	// package must override it.
	c, err := New(srv.URL, testKey, &http.Client{}, "ua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.StartVerify(context.Background(), StartRequest{Repository: "a/b", PullNumber: 1})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTemporaryRedirect {
		t.Fatalf("err = %#v, want an APIError for the 307", err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the redirect was followed")
	}
	if strings.Contains(err.Error(), other.URL) {
		t.Errorf("the error repeats the redirect target: %v", err)
	}
}

func TestTheKeyIsNeverInAnError(t *testing.T) {
	// The server echoes the key back in every shape an error can take.
	for name, body := range map[string]string{
		"string":      `{"detail":"bad key ` + testKey + `"}`,
		"object":      `{"detail":{"code":"` + testKey + `","message":"Bearer ` + testKey + `"}}`,
		"list":        `{"detail":[{"msg":"` + testKey + `"}]}`,
		"split":       `{"detail":"` + testKey[:8] + `\u0000` + testKey[8:] + `"}`,
		"not json":    testKey,
		"empty":       ``,
		"wrong shape": `{"detail":42}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, body)
			}))
			defer srv.Close()
			_, err := newClient(t, srv.URL).Run(context.Background(), "r")
			if err == nil || strings.Contains(err.Error(), testKey) {
				t.Errorf("err = %v", err)
			}
		})
	}

	// A transport error carries the URL, never the header; it is still scrubbed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := newClient(t, url).Run(context.Background(), "r")
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("err = %v", err)
	}

	// A key that cannot be a header value is refused without echoing it.
	if _, err := New("https://example.com", "bad\nkey", nil, "ua"); err == nil || strings.Contains(err.Error(), "bad") {
		t.Errorf("err = %v", err)
	}
}

func TestRejectsARunIdThatCouldChangeThePath(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"../admin","status":"queued"}`)
	}))
	defer srv.Close()
	c := newClient(t, srv.URL)

	if _, err := c.StartVerify(context.Background(), StartRequest{Repository: "a/b", PullNumber: 1}); err == nil {
		t.Error("StartVerify accepted the id ../admin")
	}
	requests.Store(0)
	for _, id := range []string{"", "../admin", "a/b", "a b", "a?x=1", "a#", "run.1", strings.Repeat("a", 65), "id\n"} {
		if _, err := c.Run(context.Background(), id); err == nil {
			t.Errorf("Run accepted the id %q", id)
		}
		if _, err := c.Stories(context.Background(), id); err == nil {
			t.Errorf("Stories accepted the id %q", id)
		}
	}
	if requests.Load() != 0 {
		t.Errorf("%d requests went out for invalid ids", requests.Load())
	}
	for _, id := range []string{"a", "Run_1-b", strings.Repeat("a", 64)} {
		if !ValidRunID(id) {
			t.Errorf("ValidRunID(%q) = false", id)
		}
	}
}

func TestReadsAtMostOneMebibyteOfAnAnswer(t *testing.T) {
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("a", 64<<10))
		io.WriteString(w, `{"id":"r","status":"`)
		for i := 0; i < 160; i++ { // 10 MiB, if anyone keeps reading
			n, err := w.Write(chunk)
			sent.Add(int64(n))
			if err != nil {
				return
			}
		}
		io.WriteString(w, `"}`)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Run(context.Background(), "r")
	if err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("err = %v, want the size limit", err)
	}
}

func TestVerdictFallsBackFromEvidenceToStateToBlockedDetail(t *testing.T) {
	str := func(s string) *string { return &s }
	for name, tc := range map[string]struct {
		story Story
		want  string
	}{
		"evidence wins":         {Story{State: "done", Evidence: &Evidence{VerifyVerdict: "CHANGES_REQUESTED"}}, "CHANGES_REQUESTED"},
		"empty evidence":        {Story{State: "done", Evidence: &Evidence{}}, "APPROVED"},
		"done":                  {Story{State: "done"}, "APPROVED"},
		"blocked detail":        {Story{State: "blocked", BlockedDetail: str("Sentinel verification verdict: INCONCLUSIVE")}, "INCONCLUSIVE"},
		"blocked other detail":  {Story{State: "blocked", BlockedDetail: str("tests failed")}, "UNKNOWN"},
		"prefix with no value":  {Story{State: "blocked", BlockedDetail: str("Sentinel verification verdict: ")}, "UNKNOWN"},
		"nothing":               {Story{}, "UNKNOWN"},
		"state done beats text": {Story{State: "done", BlockedDetail: str("Sentinel verification verdict: CHANGES_REQUESTED")}, "APPROVED"},
	} {
		if got := tc.story.Verdict(); got != tc.want {
			t.Errorf("%s: Verdict() = %q, want %q", name, got, tc.want)
		}
	}
}
