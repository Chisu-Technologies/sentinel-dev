package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout/checkouttest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

const verifyKey = "sk-live-9f8e7d6c5b4a39281706"

// fakeServer plays the Sentinel server: it creates a run, reports the statuses
// in order (the last one repeats) and then the stories.
type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	bodies   []string
	headers  []http.Header
	statuses []string
	stories  string
	// startStatus overrides the answer to the start request when not 0.
	startStatus int
	startBody   string
	polls       int
}

func newFakeServer(t *testing.T, stories string, statuses ...string) *fakeServer {
	t.Helper()
	f := &fakeServer{statuses: statuses, stories: stories}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(body))
		f.headers = append(f.headers, r.Header.Clone())
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/runs":
			if f.startStatus != 0 {
				w.WriteHeader(f.startStatus)
				io.WriteString(w, f.startBody)
				return
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"run-7","status":"`+f.statuses[0]+`"}`)
		case r.Method == "GET" && r.URL.Path == "/api/runs/run-7":
			i := min(f.polls+1, len(f.statuses)-1)
			f.polls++
			io.WriteString(w, `{"id":"run-7","status":"`+f.statuses[i]+`"}`)
		case r.Method == "GET" && r.URL.Path == "/api/runs/run-7/stories":
			io.WriteString(w, f.stories)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

const approvedStory = `[{"state":"done","title":"T","pr_url":"https://github.com/acme/widgets/pull/12","cost_usd":0.31,
	"blocked_reason":null,"blocked_detail":null,
	"evidence":{"verify_verdict":"APPROVED","tests_passed":6,"tests_total":6},
	"diff_files":3,"diff_added":40,"diff_removed":2}]`

// verify runs the command with a fake environment. sleeps counts the waits,
// which return at once.
func verifyRun(t *testing.T, runner execx.Runner, dir string, vars map[string]string, args ...string) (res result, sleeps int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"verify"}, args...), Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: runner,
		Dir:    dir,
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string { return vars[name] },
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleeps++
			if d != 5*time.Second {
				t.Errorf("polled every %v, want 5s", d)
			}
			return ctx.Err()
		},
	})
	res = result{code, stdout.String(), stderr.String()}
	if strings.Contains(res.stdout, verifyKey) || strings.Contains(res.stderr, verifyKey) {
		t.Errorf("the API key reached the output:\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
	}
	return res, sleeps
}

func serverVars(f *fakeServer) map[string]string {
	return map[string]string{"SENTINEL_URL": f.URL, "SENTINEL_API_KEY": verifyKey}
}

const prURL = "https://github.com/acme/widgets/pull/12"

func TestVerifyNeedsExactlyOnePullRequestAndSendsNothingOtherwise(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued")
	fake := &execxtest.Fake{}
	for _, args := range [][]string{
		{},
		{"12", "13"},
		{"abc"},
		{"0"},
		{"-5"},
		{"99999999999999999999"},
		{"http://github.com/acme/widgets/pull/12"},
		{"https://gitlab.com/acme/widgets/pull/12"},
		{"https://github.com.evil.example/acme/widgets/pull/12"},
		{"https://user@github.com/acme/widgets/pull/12"},
		{"https://github.com/acme/widgets/issues/12"},
		{"https://github.com/acme/widgets"},
		{"https://github.com/acme/widgets/pull/x"},
		{"https://github.com/../widgets/pull/12"},
		{"--timeout", "0s", "12"},
		{"--timeout", "soon", "12"},
		{"--base", "", "12"},
		{"--nope", "12"},
	} {
		got, _ := verifyRun(t, fake, "", serverVars(f), args...)
		if got.code != ExitUsage {
			t.Errorf("sentinel verify %q: status = %d, want %d (stderr %q)", args, got.code, ExitUsage, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("sentinel verify %q printed %q on stdout", args, got.stdout)
		}
	}
	if n := f.requestCount(); n != 0 {
		t.Errorf("a wrong command line sent %d requests", n)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("a wrong command line ran %+v", fake.Calls)
	}
}

func TestVerifyDryRunSendsNothingAndNeedsNoKey(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued")
	got, sleeps := verifyRun(t, &execxtest.Fake{}, "", map[string]string{"SENTINEL_URL": f.URL}, "--dry-run", "--base", "release", prURL)

	if got.code != ExitOK || sleeps != 0 {
		t.Fatalf("status = %d, sleeps = %d, stderr = %q", got.code, sleeps, got.stderr)
	}
	host := strings.TrimPrefix(f.URL, "http://")
	for _, want := range []string{
		"Repository    acme/widgets\n",
		"Pull request  #12\n",
		"Base          release\n",
		"Server        " + host + "\n",
		"Nothing was sent: this was a dry run.\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "http://") {
		t.Errorf("the dry run shows the full address, not the host:\n%s", got.stdout)
	}
	if n := f.requestCount(); n != 0 {
		t.Errorf("the dry run sent %d requests", n)
	}
}

func TestVerifyDryRunSaysWhenTheServerIsNotSet(t *testing.T) {
	got, _ := verifyRun(t, &execxtest.Fake{}, "", nil, "--dry-run", prURL)
	if got.code != ExitOK || !strings.Contains(got.stdout, "Server        SENTINEL_URL is not set\n") {
		t.Errorf("sentinel verify --dry-run = %+v", got)
	}
	if strings.Contains(got.stdout, "Base") {
		t.Errorf("a base nobody gave is shown:\n%s", got.stdout)
	}
}

func TestVerifyDryRunJSONIsOneObject(t *testing.T) {
	got, _ := verifyRun(t, &execxtest.Fake{}, "", map[string]string{"SENTINEL_URL": "https://sentinel.example.com/api-root"}, "--dry-run", "--json", prURL)
	var out map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil {
		t.Fatalf("the output does not parse: %v\n%s", err, got.stdout)
	}
	if got.code != ExitOK || out["repository"] != "acme/widgets" || out["pull_request"] != float64(12) || out["server_host"] != "sentinel.example.com" || out["dry_run"] != true {
		t.Errorf("status %d, output %v", got.code, out)
	}
}

func TestVerifyANumberTakesTheRepositoryFromTheCheckout(t *testing.T) {
	f := newFakeServer(t, approvedStory, "completed")
	checkout := checkouttest.New(t)

	got, _ := verifyRun(t, execx.OS{}, checkout.Work, serverVars(f), "12")

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	if want := `{"mode":"verify_pr","pr_number":12,"repo":"acme/widgets"}`; len(f.bodies) == 0 || f.bodies[0] != want {
		t.Errorf("start body = %v, want %s", f.bodies, want)
	}
}

func TestVerifyANumberOutsideARepositoryFailsWithTheReason(t *testing.T) {
	f := newFakeServer(t, approvedStory, "completed")
	got, _ := verifyRun(t, execx.OS{}, t.TempDir(), serverVars(f), "12")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "not inside a git repository") {
		t.Errorf("sentinel verify 12 outside a repository = %+v", got)
	}
	if n := f.requestCount(); n != 0 {
		t.Errorf("sent %d requests without a repository", n)
	}
}

func TestVerifyAURLWorksOutsideAnyRepository(t *testing.T) {
	f := newFakeServer(t, approvedStory, "completed")
	got, _ := verifyRun(t, execx.OS{}, t.TempDir(), serverVars(f), prURL+"/files?w=1#diff")
	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	if want := `{"mode":"verify_pr","pr_number":12,"repo":"acme/widgets"}`; f.bodies[0] != want {
		t.Errorf("start body = %s, want %s", f.bodies[0], want)
	}
}

func TestVerifyMissingEnvironmentExitsOneNamingTheVariable(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued")
	for name, tc := range map[string]struct {
		vars map[string]string
		want string
	}{
		"no URL":           {map[string]string{"SENTINEL_API_KEY": verifyKey}, "SENTINEL_URL"},
		"no key":           {map[string]string{"SENTINEL_URL": f.URL}, "SENTINEL_API_KEY"},
		"blank key":        {map[string]string{"SENTINEL_URL": f.URL, "SENTINEL_API_KEY": "  "}, "SENTINEL_API_KEY"},
		"nothing":          {nil, "SENTINEL_URL"},
		"insecure address": {map[string]string{"SENTINEL_URL": "http://sentinel.example.com", "SENTINEL_API_KEY": verifyKey}, "SENTINEL_URL"},
	} {
		got, _ := verifyRun(t, &execxtest.Fake{}, "", tc.vars, prURL)
		if got.code != ExitFailed || !strings.Contains(got.stderr, tc.want) || got.stdout != "" {
			t.Errorf("%s: %+v, want status 1 naming %s", name, got, tc.want)
		}
	}
	if n := f.requestCount(); n != 0 {
		t.Errorf("sent %d requests without a complete environment", n)
	}
}

func TestVerifyApprovedExitsZeroAndPrintsTheTable(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued", "running", "running", "completed")
	got, sleeps := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), "--base", "release", prURL)

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	want := "Repository    acme/widgets\n" +
		"Pull request  #12 " + prURL + "\n" +
		"Run           run-7 (completed)\n" +
		"Verdict       APPROVED\n" +
		"Tests         6 of 6 passed\n" +
		"Changes       3 files, +40 -2\n" +
		"Cost          $0.31\n"
	if got.stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", got.stdout, want)
	}
	// One line when the run is created and one for each change of status.
	if want := "Run run-7 created (queued).\nRun run-7: running\nRun run-7: completed\n"; got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if sleeps != 3 {
		t.Errorf("waited %d times, want 3", sleeps)
	}
	if want := `{"base_ref":"release","mode":"verify_pr","pr_number":12,"repo":"acme/widgets"}`; f.bodies[0] != want {
		t.Errorf("start body = %s", f.bodies[0])
	}
	for _, h := range f.headers {
		if h.Get("Authorization") != "Bearer "+verifyKey || h.Get("User-Agent") != "sentinel/v0.0.0-test" || h.Get("Accept") != "application/json" {
			t.Errorf("headers = %v", h)
		}
	}
}

func TestVerifyTheTableLeavesOutWhatIsNotKnown(t *testing.T) {
	story := `[{"state":"done","pr_url":null,"cost_usd":0,"evidence":{"verify_verdict":"APPROVED","tests_total":0},"diff_files":0}]`
	f := newFakeServer(t, story, "completed")
	got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL)
	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	for _, absent := range []string{"Tests", "Changes", "Reason"} {
		if strings.Contains(got.stdout, absent) {
			t.Errorf("stdout has %s although it is not known:\n%s", absent, got.stdout)
		}
	}
	if !strings.Contains(got.stdout, "Pull request  #12 "+prURL+"\n") || !strings.Contains(got.stdout, "Cost          $0.00\n") {
		t.Errorf("stdout = %s", got.stdout)
	}
}

func TestVerifyChangesRequestedExitsOneWithTheReason(t *testing.T) {
	story := `[{"state":"blocked","cost_usd":1.5,"blocked_reason":"verify","blocked_detail":"Sentinel verification verdict: CHANGES_REQUESTED",
		"evidence":{"verify_verdict":"CHANGES_REQUESTED","tests_passed":4,"tests_total":6},"diff_files":1,"diff_added":2,"diff_removed":0}]`
	f := newFakeServer(t, story, "blocked")
	got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL)

	if got.code != ExitFailed {
		t.Errorf("status = %d, want 1", got.code)
	}
	for _, want := range []string{
		"Run           run-7 (blocked)\n",
		"Verdict       CHANGES_REQUESTED\n",
		"Tests         4 of 6 passed\n",
		"Changes       1 file, +2 -0\n",
		"Reason        Sentinel verification verdict: CHANGES_REQUESTED\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
}

func TestVerifyARunThatEndsFailedExitsOne(t *testing.T) {
	// Even a story that claims to be done: a failed run verified nothing.
	for _, status := range []string{"failed", "stopped", "cancelled", "zombie"} {
		f := newFakeServer(t, `[{"state":"done","cost_usd":0.1}]`, "running", status)
		got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL)
		if got.code != ExitFailed || !strings.Contains(got.stdout, "Run           run-7 ("+status+")\n") || !strings.Contains(got.stdout, "Verdict       UNKNOWN\n") {
			t.Errorf("%s: %+v", status, got)
		}
	}

	// And one that has no stories at all.
	f := newFakeServer(t, `[]`, "failed")
	if got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL); got.code != ExitFailed {
		t.Errorf("a failed run without stories: %+v", got)
	}
}

func TestVerifyTheVerdictFallsBackFromEvidenceToStateToBlockedDetailToUnknown(t *testing.T) {
	for name, tc := range map[string]struct {
		story, verdict, reason string
		code                   int
	}{
		"evidence": {`[{"state":"blocked","evidence":{"verify_verdict":"INCONCLUSIVE"},"blocked_detail":"Sentinel verification verdict: CHANGES_REQUESTED"}]`, "INCONCLUSIVE", "Sentinel verification verdict: CHANGES_REQUESTED", ExitFailed},
		"state":    {`[{"state":"done","evidence":{"verify_verdict":""}}]`, "APPROVED", "", ExitOK},
		"detail":   {`[{"state":"blocked","blocked_detail":"Sentinel verification verdict: CHANGES_REQUESTED"}]`, "CHANGES_REQUESTED", "Sentinel verification verdict: CHANGES_REQUESTED", ExitFailed},
		"unknown":  {`[{"state":"blocked","blocked_detail":"the sandbox ran out of disk"}]`, "UNKNOWN", "the sandbox ran out of disk", ExitFailed},
		"empty":    {`[]`, "UNKNOWN", "", ExitFailed},
	} {
		f := newFakeServer(t, tc.story, "completed")
		got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), "--json", prURL)
		var out struct{ Verdict, Reason string }
		if err := json.Unmarshal([]byte(got.stdout), &out); err != nil {
			t.Fatalf("%s: the output does not parse: %v\n%s", name, err, got.stdout)
		}
		if out.Verdict != tc.verdict || out.Reason != tc.reason || got.code != tc.code {
			t.Errorf("%s: verdict %q, reason %q, status %d; want %q, %q, %d", name, out.Verdict, out.Reason, got.code, tc.verdict, tc.reason, tc.code)
		}
	}
}

func TestVerifyNoWaitStopsAfterTheRunIsCreated(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued", "completed")
	got, sleeps := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), "--no-wait", prURL)

	if got.code != ExitOK || sleeps != 0 {
		t.Fatalf("status = %d, sleeps = %d, stderr = %q", got.code, sleeps, got.stderr)
	}
	if !strings.Contains(got.stdout, "Run           run-7 (queued)\n") || strings.Contains(got.stdout, "Verdict") {
		t.Errorf("stdout = %s", got.stdout)
	}
	if len(f.requests) != 1 || f.requests[0] != "POST /api/runs" {
		t.Errorf("requests = %v, want only the start", f.requests)
	}
}

func TestVerifyATimeoutExitsOneAndNamesTheRun(t *testing.T) {
	f := newFakeServer(t, approvedStory, "queued", "running")
	var stdout, stderr bytes.Buffer
	calls := 0
	code := Run(context.Background(), []string{"verify", "--timeout", "10m", prURL}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(name string) string { return serverVars(f)[name] },
		// The clock runs out on the third wait, as the real timer would.
		Sleep: func(ctx context.Context, d time.Duration) error {
			if calls++; calls == 3 {
				return context.DeadlineExceeded
			}
			return nil
		},
	})
	if code != ExitFailed || stdout.String() != "" {
		t.Errorf("status = %d, stdout = %q", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "Gave up waiting after 10m0s.") || !strings.Contains(stderr.String(), "Run run-7 continues on the server.") {
		t.Errorf("stderr = %q, want the timeout and the run id", stderr.String())
	}
	if strings.Contains(stderr.String(), verifyKey) {
		t.Error("the key reached stderr")
	}
}

func TestVerifyARealTimeoutEndsTheWaitWithoutTheFakeClock(t *testing.T) {
	f := newFakeServer(t, approvedStory, "running")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"verify", "--timeout", "20ms", prURL}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(name string) string { return serverVars(f)[name] },
	})
	if code != ExitFailed || !strings.Contains(stderr.String(), "Gave up waiting after 20ms.") || !strings.Contains(stderr.String(), "run-7") {
		t.Errorf("status = %d, stderr = %q", code, stderr.String())
	}
}

func TestVerifyInterruptingSaysTheRunContinues(t *testing.T) {
	f := newFakeServer(t, approvedStory, "running")
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"verify", prURL}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(name string) string { return serverVars(f)[name] },
		Sleep: func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	if code != ExitFailed || !strings.Contains(stderr.String(), "Interrupted.") || !strings.Contains(stderr.String(), "Run run-7 continues on the server.") {
		t.Errorf("status = %d, stderr = %q", code, stderr.String())
	}
}

func TestVerifyJSONPrintsOneObjectAndNothingElseOnStdout(t *testing.T) {
	f := newFakeServer(t, approvedStory, "running", "completed")
	got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), "--json", prURL)

	dec := json.NewDecoder(strings.NewReader(got.stdout))
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout does not parse: %v\n%s", err, got.stdout)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		t.Errorf("stdout carries more than one document:\n%s", got.stdout)
	}
	want := map[string]any{
		"repository": "acme/widgets", "pull_request": float64(12), "pull_request_url": prURL,
		"run_id": "run-7", "status": "completed", "verdict": "APPROVED", "cost_usd": 0.31,
		"tests_passed": float64(6), "tests_total": float64(6),
	}
	if got.code != ExitOK || len(out) != len(want) {
		t.Errorf("status %d, output %v", got.code, out)
	}
	for k, v := range want {
		if out[k] != v {
			t.Errorf("%s = %v, want %v", k, out[k], v)
		}
	}
	// Progress is for people, and goes elsewhere.
	if !strings.Contains(got.stderr, "created") {
		t.Errorf("stderr = %q, want the progress", got.stderr)
	}
}

func TestVerifyJSONKeepsTheExitStatusWhenTheVerdictIsNotApproval(t *testing.T) {
	f := newFakeServer(t, `[{"state":"blocked","blocked_detail":"Sentinel verification verdict: CHANGES_REQUESTED","cost_usd":2}]`, "blocked")
	got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), "--json", prURL)
	var out map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil {
		t.Fatalf("stdout does not parse: %v\n%s", err, got.stdout)
	}
	if got.code != ExitFailed || out["verdict"] != "CHANGES_REQUESTED" || out["reason"] == "" {
		t.Errorf("status %d, output %v", got.code, out)
	}
	if _, has := out["tests_total"]; has {
		t.Errorf("tests_total is there although no test count is known: %v", out)
	}
}

func TestVerifyAServerErrorExitsOneAndNeverEchoesTheKey(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"unauthorized echoing the key": {401, `{"detail":"Invalid key ` + verifyKey + `"}`, "check SENTINEL_API_KEY"},
		"object detail echoing it":     {422, `{"detail":{"code":"bad","message":"Bearer ` + verifyKey + `"}}`, "[redacted]"},
		"a redirect":                   {307, ``, "redirects are not followed"},
		"plain text":                   {502, verifyKey, "502"},
	} {
		f := newFakeServer(t, approvedStory, "queued")
		f.startStatus, f.startBody = tc.status, tc.body
		got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL)
		if got.code != ExitFailed || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
			t.Errorf("%s: %+v, want status 1 and %q on stderr", name, got, tc.want)
		}
	}
}

func TestVerifyDoesNotFollowARedirectWithTheKey(t *testing.T) {
	var sawKey bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = sawKey || r.Header.Get("Authorization") != ""
	}))
	defer other.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"verify", prURL}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(name string) string {
			return map[string]string{"SENTINEL_URL": redirector.URL, "SENTINEL_API_KEY": verifyKey}[name]
		},
		// A client that follows redirects, as the default one does.
		HTTP: &http.Client{},
	})
	if code != ExitFailed || sawKey {
		t.Errorf("status = %d, the other host received the key: %v", code, sawKey)
	}
}

func TestVerifyShowsOnlyCleanTextFromTheServer(t *testing.T) {
	story := `[{"state":"blocked","cost_usd":0,"blocked_detail":"Sentinel verification verdict: CHANGES_REQUESTED\u001b[2J\u001b]0;pwned\u0007 key ` + verifyKey + `"}]`
	f := newFakeServer(t, story, "blocked")
	got, _ := verifyRun(t, &execxtest.Fake{}, "", serverVars(f), prURL)
	if strings.ContainsAny(got.stdout+got.stderr, "\x1b\x07") {
		t.Errorf("the output carries a control character: %q", got.stdout)
	}
	if !strings.Contains(got.stdout, "[redacted]") {
		t.Errorf("stdout = %q, want the echoed key replaced", got.stdout)
	}
}
