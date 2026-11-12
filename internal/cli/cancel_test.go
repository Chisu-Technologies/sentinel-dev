package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// cancelServer plays one run: the read answers readStatus and readBody, and
// the stop answers stopStatus and stopBody.
type cancelServer struct {
	*httptest.Server
	mu         sync.Mutex
	requests   []string
	keys       []string
	readStatus int
	readBody   string
	stopStatus int
	stopBody   string
}

func newCancelServer(t *testing.T) *cancelServer {
	t.Helper()
	c := &cancelServer{
		readStatus: http.StatusOK, readBody: `{"id":"run-7","status":"running"}`,
		stopStatus: http.StatusOK, stopBody: `{"id":"run-7","status":"stopped"}`,
	}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.requests = append(c.requests, r.Method+" "+r.URL.Path)
		c.keys = append(c.keys, r.Header.Get("Authorization"))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/runs/run-7":
			w.WriteHeader(c.readStatus)
			io.WriteString(w, c.readBody)
		case "POST /api/runs/run-7/stop":
			w.WriteHeader(c.stopStatus)
			io.WriteString(w, c.stopBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *cancelServer) sent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.requests, ", ")
}

func cancelVars(c *cancelServer) map[string]string {
	return map[string]string{"SENTINEL_URL": c.URL, "SENTINEL_API_KEY": verifyKey}
}

const readThenStop = "GET /api/runs/run-7, POST /api/runs/run-7/stop"

func TestCancelWrongArgumentsExitTwoWithoutARequest(t *testing.T) {
	c := newCancelServer(t)
	for _, args := range [][]string{
		{"cancel"},
		{"cancel", "run-7", "run-8"},
		{"cancel", "../run-7"},
		{"cancel", "run 7"},
		{"cancel", "run-7/stop"},
		{"cancel", strings.Repeat("a", 65)},
		{"cancel", "--nope", "run-7"},
	} {
		got := command(t, cancelVars(c), nil, args...)
		if got.code != ExitUsage || got.stdout != "" {
			t.Errorf("sentinel %q = %+v, want status 2 and no output", args, got)
		}
	}
	if sent := c.sent(); sent != "" {
		t.Errorf("a wrong command line sent: %s", sent)
	}
}

func TestCancelNamesTheMissingVariableAndExitsOne(t *testing.T) {
	c := newCancelServer(t)
	for _, name := range []string{"SENTINEL_URL", "SENTINEL_API_KEY"} {
		vars := cancelVars(c)
		delete(vars, name)
		got := command(t, vars, nil, "cancel", "run-7")
		if got.code != ExitFailed || !strings.Contains(got.stderr, name) {
			t.Errorf("without %s: %+v", name, got)
		}
	}
	if sent := c.sent(); sent != "" {
		t.Errorf("a missing variable sent: %s", sent)
	}
}

func TestCancelStopsARunningRunWithTheKeyOnBothRequests(t *testing.T) {
	c := newCancelServer(t)
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitOK || got.stdout != "Run run-7: stopped\n" || got.stderr != "" {
		t.Errorf("got %+v", got)
	}
	if sent := c.sent(); sent != readThenStop {
		t.Errorf("requests = %s, want %s", sent, readThenStop)
	}
	for i, key := range c.keys {
		if key != "Bearer "+verifyKey {
			t.Errorf("request %d carried Authorization %q", i, key)
		}
	}
}

func TestCancelSaysTheStopWaitsForTheMachineOfTheRun(t *testing.T) {
	c := newCancelServer(t)
	c.stopStatus, c.stopBody = http.StatusAccepted, `{"id":"run-7","status":"running","sync_status":"queued"}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitOK || got.stderr != "" {
		t.Errorf("got %+v", got)
	}
	for _, want := range []string{"Run run-7: the server accepted the stop.", "sentinel watch run-7"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", got.stdout, want)
		}
	}
	if strings.Contains(got.stdout, "stopped") {
		t.Errorf("stdout = %q claims the run stopped, and it is still running", got.stdout)
	}
}

func TestCancelLeavesARunThatAlreadyEndedAlone(t *testing.T) {
	for _, status := range []string{"completed", "blocked", "failed", "stopped"} {
		c := newCancelServer(t)
		c.readBody = `{"id":"run-7","status":"` + status + `"}`
		got := command(t, cancelVars(c), nil, "cancel", "run-7")
		want := "Run run-7 had already ended: " + status + ". Nothing was stopped.\n"
		if got.code != ExitOK || got.stdout != want {
			t.Errorf("%s: got %+v, want stdout %q", status, got, want)
		}
		if sent := c.sent(); sent != "GET /api/runs/run-7" {
			t.Errorf("%s: requests = %s, want only the read", status, sent)
		}
	}
}

func TestCancelSaysWhenTheRunEndedBeforeTheStop(t *testing.T) {
	c := newCancelServer(t)
	c.stopBody = `{"id":"run-7","status":"completed"}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitOK || got.stdout != "Run run-7 ended before it was stopped: completed.\n" {
		t.Errorf("got %+v", got)
	}
}

func TestCancelOfARunTheServerDoesNotKnowExitsOneWithoutAStop(t *testing.T) {
	c := newCancelServer(t)
	c.readStatus, c.readBody = http.StatusNotFound, `{"detail":"not found"}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitFailed || got.stdout != "" || !strings.Contains(got.stderr, "Run run-7 was not found on this server.") {
		t.Errorf("got %+v", got)
	}
	if sent := c.sent(); sent != "GET /api/runs/run-7" {
		t.Errorf("requests = %s, want only the read", sent)
	}
}

func TestCancelWithAKeyTheServerRefusesPointsAtTheKey(t *testing.T) {
	c := newCancelServer(t)
	c.readStatus, c.readBody = http.StatusUnauthorized, `{"detail":"not authenticated"}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "check SENTINEL_API_KEY") {
		t.Errorf("got %+v", got)
	}
	if sent := c.sent(); sent != "GET /api/runs/run-7" {
		t.Errorf("requests = %s, want only the read", sent)
	}
}

// A server that reads the run with the key and answers 401 to the stop is one
// that stops runs only from its panel. The key is right, so the message must
// not send the user to check it.
func TestCancelOnAServerThatStopsOnlyFromItsPanelDoesNotBlameTheKey(t *testing.T) {
	c := newCancelServer(t)
	c.stopStatus, c.stopBody = http.StatusUnauthorized, `{"detail":"not authenticated"}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitFailed || got.stdout != "" {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(got.stderr, "stops runs only from its panel") {
		t.Errorf("stderr = %q, want it to say where this server stops runs", got.stderr)
	}
	if strings.Contains(got.stderr, "check SENTINEL_API_KEY") {
		t.Errorf("stderr = %q blames a key the server had just accepted", got.stderr)
	}
	if sent := c.sent(); sent != readThenStop {
		t.Errorf("requests = %s, want %s", sent, readThenStop)
	}
}

func TestCancelShowsTheReasonAndCodeOfARefusedStop(t *testing.T) {
	c := newCancelServer(t)
	c.stopStatus = http.StatusConflict
	c.stopBody = `{"detail":{"code":"tenant_runtime_offline","message":"Tenant runtime is offline"}}`
	got := command(t, cancelVars(c), nil, "cancel", "run-7")
	if got.code != ExitFailed || got.stdout != "" {
		t.Errorf("got %+v", got)
	}
	for _, want := range []string{"409", "Tenant runtime is offline", "tenant_runtime_offline"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
		}
	}
}

// The only statuses cancel prints are the ones this client knows. Anything
// else the server puts in the field stays off the terminal.
func TestCancelNeverPrintsAStatusItDoesNotKnow(t *testing.T) {
	for _, tc := range []struct{ name, readBody, stopBody string }{
		{"after the stop", `{"id":"run-7","status":"running"}`, `{"id":"run-7","status":"completed\u001b[2J\u0007"}`},
		{"before the stop", `{"id":"run-7","status":"\u001b[31mfailed"}`, `{"id":"run-7","status":"\u001b]0;owned\u0007"}`},
	} {
		c := newCancelServer(t)
		c.readBody, c.stopBody = tc.readBody, tc.stopBody
		got := command(t, cancelVars(c), nil, "cancel", "run-7")
		if strings.ContainsAny(got.stdout+got.stderr, "\x1b\x07") || strings.Contains(got.stdout, "owned") {
			t.Errorf("%s: text of the server reached the terminal: %q %q", tc.name, got.stdout, got.stderr)
		}
		if got.code != ExitOK || !strings.Contains(got.stdout, "the server accepted the stop") {
			t.Errorf("%s: got %+v, want a run of unknown status reported as still going", tc.name, got)
		}
	}
}
