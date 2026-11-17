package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type haltServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	keys     []string
	status   int
	body     string
}

func newHaltServer(t *testing.T, status int, body string) *haltServer {
	t.Helper()
	h := &haltServer{status: status, body: body}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.requests = append(h.requests, r.Method+" "+r.URL.Path)
		h.keys = append(h.keys, r.Header.Get("Authorization"))
		if r.Method+" "+r.URL.Path != "POST /api/governance/runs-halt/stop" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(h.status)
		io.WriteString(w, h.body)
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *haltServer) sent() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.requests, ", ")
}

func haltVars(h *haltServer) map[string]string {
	return map[string]string{"SENTINEL_URL": h.URL, "SENTINEL_API_KEY": verifyKey}
}

func TestStopWithoutYesExplainsAndSendsNothing(t *testing.T) {
	h := newHaltServer(t, http.StatusOK, `{"runs_halted":true,"stopped":2}`)
	got := command(t, haltVars(h), nil, "stop")
	if got.code != ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "sentinel stop --yes") {
		t.Errorf("got %+v", got)
	}
	if sent := h.sent(); sent != "" {
		t.Errorf("an unconfirmed stop sent: %s", sent)
	}
}

func TestStopRefusesArguments(t *testing.T) {
	h := newHaltServer(t, http.StatusOK, `{}`)
	got := command(t, haltVars(h), nil, "stop", "--yes", "run-7")
	if got.code != ExitUsage || !strings.Contains(got.stderr, "sentinel cancel <run id>") {
		t.Errorf("got %+v", got)
	}
	if sent := h.sent(); sent != "" {
		t.Errorf("sent: %s", sent)
	}
}

func TestStopStopsEveryRunWithTheKey(t *testing.T) {
	h := newHaltServer(t, http.StatusOK, `{"runs_halted":true,"stopped":2}`)
	got := command(t, haltVars(h), nil, "stop", "--yes")
	want := "Stopped 2 runs. No new run starts until the owner allows runs again in the panel.\n"
	if got.code != ExitOK || got.stdout != want || got.stderr != "" {
		t.Errorf("got %+v", got)
	}
	if h.keys[0] != "Bearer "+verifyKey {
		t.Errorf("Authorization = %q", h.keys[0])
	}
}

func TestStopOfOneRunSaysRun(t *testing.T) {
	h := newHaltServer(t, http.StatusOK, `{"runs_halted":true,"stopped":1}`)
	got := command(t, haltVars(h), nil, "stop", "--yes")
	if !strings.HasPrefix(got.stdout, "Stopped 1 run.") {
		t.Errorf("got %+v", got)
	}
}

func TestStopHandedToTheMachineOfTheRuns(t *testing.T) {
	h := newHaltServer(t, http.StatusAccepted, `{"runs_halted":true,"sync_status":"pending"}`)
	got := command(t, haltVars(h), nil, "stop", "--yes")
	if got.code != ExitOK || !strings.HasPrefix(got.stdout, "Every run is stopping") {
		t.Errorf("got %+v", got)
	}
}

func TestStopOnAServerWithoutTheEndpointPointsToThePanel(t *testing.T) {
	h := newHaltServer(t, http.StatusOK, `{}`)
	h.Config.Handler = http.NotFoundHandler()
	got := command(t, haltVars(h), nil, "stop", "--yes")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "from its panel") {
		t.Errorf("got %+v", got)
	}
}

func TestStopWithAWrongKeySaysWhatTheServerSaid(t *testing.T) {
	h := newHaltServer(t, http.StatusUnauthorized, `{"detail":"not authenticated"}`)
	got := command(t, haltVars(h), nil, "stop", "--yes")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "not authenticated") || strings.Contains(got.stderr, verifyKey) {
		t.Errorf("got %+v", got)
	}
}
