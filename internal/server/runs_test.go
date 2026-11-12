package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListSendsThePathAndQueryAndMapsTheFields(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		gotAuth, gotAccept = r.Header.Get("Authorization"), r.Header.Get("Accept")
		io.WriteString(w, `[
			{"id":"run-2","repo":"acme/widgets","status":"completed","mode":"verify_pr","story":"Fix login",
			 "pr_number":"12","pr_url":"https://github.com/acme/widgets/pull/12","total_cost_usd":0.31,
			 "started_at":"2026-01-02T03:04:05Z","finished_at":"2026-01-02T03:09:00Z","unknown":1},
			{"id":"run-1","repo":"acme/widgets","status":"queued","story":null,"pr_number":null,"pr_url":null,
			 "total_cost_usd":0,"started_at":null,"finished_at":null}]`)
	}))
	defer srv.Close()

	runs, err := newClient(t, srv.URL).Runs(context.Background(), 20, 40)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/runs" || gotQuery != "limit=20&offset=40" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "Bearer "+testKey || gotAccept != "application/json" {
		t.Errorf("headers: Authorization %q, Accept %q", gotAuth, gotAccept)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %+v", runs)
	}
	first := runs[0]
	if first.ID != "run-2" || first.Repo != "acme/widgets" || first.Status != "completed" || first.Mode != "verify_pr" ||
		first.TotalCostUSD != 0.31 {
		t.Errorf("first = %+v", first)
	}
	if first.Story == nil || *first.Story != "Fix login" || first.PRNumber == nil || *first.PRNumber != "12" ||
		first.PRURL == nil || *first.PRURL != "https://github.com/acme/widgets/pull/12" ||
		first.StartedAt == nil || *first.StartedAt != "2026-01-02T03:04:05Z" ||
		first.FinishedAt == nil || *first.FinishedAt != "2026-01-02T03:09:00Z" {
		t.Errorf("first pointers = %+v", first)
	}
	second := runs[1]
	if second.Mode != "" || second.Story != nil || second.PRNumber != nil || second.PRURL != nil || second.StartedAt != nil || second.FinishedAt != nil {
		t.Errorf("second = %+v, want absent and null fields to stay empty", second)
	}
}

func TestLogReturnsTheTextAndAsksForPlainText(t *testing.T) {
	var gotPath, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept = r.URL.Path, r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "line one\nline two\n")
	}))
	defer srv.Close()

	text, found, err := newClient(t, srv.URL).Log(context.Background(), "run-7")
	if err != nil || !found || text != "line one\nline two\n" {
		t.Fatalf("Log = %q, %v, %v", text, found, err)
	}
	if gotPath != "/api/runs/run-7/log" || gotAccept != "text/plain" {
		t.Errorf("request path %q, Accept %q", gotPath, gotAccept)
	}
}

func TestLogTreatsANotFoundAsNoLogYet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer srv.Close()
	text, found, err := newClient(t, srv.URL).Log(context.Background(), "run-7")
	if err != nil || found || text != "" {
		t.Errorf("Log = %q, %v, %v, want no text, not found and no error", text, found, err)
	}
}

func TestLogReportsOtherStatusesAsErrorsWithoutTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"detail":"broken `+testKey+`"}`)
	}))
	defer srv.Close()
	_, _, err := newClient(t, srv.URL).Log(context.Background(), "run-7")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 500 || strings.Contains(err.Error(), testKey) {
		t.Errorf("err = %v", err)
	}
}

func TestLogRefusesARunIDThatIsNotSafeInAPath(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer srv.Close()
	if _, _, err := newClient(t, srv.URL).Log(context.Background(), "../x"); err == nil || requests.Load() != 0 {
		t.Errorf("err = %v after %d requests", err, requests.Load())
	}
}

func TestLogRefusesAnAnswerLargerThanTheLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Repeat("x", maxBody+1))
	}))
	defer srv.Close()
	if _, _, err := newClient(t, srv.URL).Log(context.Background(), "run-7"); err == nil {
		t.Error("an answer over 1 MiB was accepted")
	}
}

func TestLogNeverFollowsARedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		io.WriteString(w, "secret")
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/log", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	// A client that would follow redirects: the package must override it.
	c, err := New(srv.URL, testKey, &http.Client{}, "ua")
	if err != nil {
		t.Fatal(err)
	}
	text, found, err := c.Log(context.Background(), "run-7")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTemporaryRedirect || found || text != "" {
		t.Fatalf("Log = %q, %v, %v, want an APIError for the 307", text, found, err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the redirect was followed")
	}
}

func TestCleanTextKeepsNewlinesAndTabsAndRemovesOtherControlCharacters(t *testing.T) {
	c := newClient(t, "https://sentinel.example.com")
	in := "a\tb\nc\x1b[31mred\x1b[0m\r\x00d\x07e\u0085f"
	if got, want := c.CleanText(in), "a\tb\nc[31mred[0mdef"; got != want {
		t.Errorf("CleanText = %q, want %q", got, want)
	}
}

func TestCleanTextRedactsTheKeyEvenWhenControlCharactersSplitIt(t *testing.T) {
	c := newClient(t, "https://sentinel.example.com")
	in := "token " + testKey + "\nsplit " + testKey[:8] + "\x1b\x00" + testKey[8:] + "\n"
	got := c.CleanText(in)
	if strings.Contains(got, testKey) || strings.Count(got, "[redacted]") != 2 {
		t.Errorf("CleanText = %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("the newlines were lost: %q", got)
	}
}

func TestCleanStillRemovesNewlinesAndTabs(t *testing.T) {
	c := newClient(t, "https://sentinel.example.com")
	if got := c.Clean("a\nb\tc", 10); got != "abc" {
		t.Errorf("Clean = %q: single-line cleaning must not keep layout characters", got)
	}
}

func TestStopPostsToTheRunWithTheKeyAndNoBody(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, `{"id":"run-7","status":"stopped","finished_at":"2026-01-02T03:09:00Z"}`)
	}))
	defer srv.Close()

	run, err := newClient(t, srv.URL).Stop(context.Background(), "run-7")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/runs/run-7/stop" || gotAuth != "Bearer "+testKey {
		t.Errorf("request = %s %s, Authorization %q", gotMethod, gotPath, gotAuth)
	}
	if len(gotBody) != 0 || gotType != "" {
		t.Errorf("the stop carried a body: %q (Content-Type %q)", gotBody, gotType)
	}
	if run.ID != "run-7" || run.Status != StatusStopped || run.FinishedAt == nil {
		t.Errorf("run = %+v", run)
	}
}

// A server that hands the stop to the machine of the run answers 202 with the
// status the run still has.
func TestStopReadsTheRunOfAnAcceptedStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"id":"run-7","status":"running","sync_status":"queued"}`)
	}))
	defer srv.Close()
	run, err := newClient(t, srv.URL).Stop(context.Background(), "run-7")
	if err != nil || run.Status != "running" {
		t.Errorf("Stop = %+v, %v", run, err)
	}
}

func TestStopReturnsTheAnswerOfARefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"detail":{"code":"tenant_runtime_offline","message":"Tenant runtime is offline"}}`)
	}))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Stop(context.Background(), "run-7")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != "tenant_runtime_offline" ||
		apiErr.Detail != "Tenant runtime is offline" {
		t.Errorf("err = %v", err)
	}
}

func TestStopWithAnIDThatIsNotValidSendsNothing(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer srv.Close()
	if _, err := newClient(t, srv.URL).Stop(context.Background(), "../x"); err == nil || requests.Load() != 0 {
		t.Errorf("err = %v after %d requests", err, requests.Load())
	}
}

func TestStopNeverFollowsARedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/stop", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	// A client that would follow redirects: the package must override it. A
	// 307 keeps the method, so following it would stop a run somewhere else
	// with the key in the header.
	c, err := New(srv.URL, testKey, &http.Client{}, "ua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Stop(context.Background(), "run-7")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTemporaryRedirect {
		t.Fatalf("err = %v, want an APIError for the 307", err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the redirect was followed")
	}
}
