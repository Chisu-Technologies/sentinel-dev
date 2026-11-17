package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const pausedRun = `{"id":"run-1","repo":"acme/widgets","status":"blocked",
 "findings":[{"id":"TASK-1","title":"Fix the login","description":"Fix the login test","severity":"medium","category":"general"}]}`

// resumeServer plays a paused run-1 and the run-2 that resumes it.
type resumeServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	start    map[string]any
	// echo is what the server names as the run it resumed.
	echo   string
	paused string
}

// noSleep makes waiting for a run instant.
func noSleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func newResumeServer(t *testing.T, echo string) *resumeServer {
	t.Helper()
	s := &resumeServer{echo: echo, paused: pausedRun}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/runs/run-1":
			io.WriteString(w, s.paused)
		case "POST /api/runs":
			json.Unmarshal(body, &s.start)
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"run-2","status":"running","resumed_from":"`+s.echo+`"}`)
		case "POST /api/runs/run-2/stop":
			io.WriteString(w, `{"id":"run-2","status":"stopped"}`)
		case "GET /api/runs/run-2":
			io.WriteString(w, `{"id":"run-2","status":"completed"}`)
		case "GET /api/runs/run-2/stories":
			io.WriteString(w, `[{"state":"done","title":"Fix the login","pr_url":"https://github.com/acme/widgets/pull/9","cost_usd":0.4}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *resumeServer) vars() map[string]string {
	return map[string]string{"SENTINEL_URL": s.URL, "SENTINEL_API_KEY": verifyKey}
}

func (s *resumeServer) sent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.requests, ", ")
}

func TestResumeContinuesThePausedRunWithItsTaskAndANewLimit(t *testing.T) {
	s := newResumeServer(t, "run-1")
	got := command(t, s.vars(), noSleep, "resume", "--budget", "3", "run-1")

	if got.code != ExitOK || !strings.Contains(got.stdout, "https://github.com/acme/widgets/pull/9") || !strings.Contains(got.stdout, "Fix the login") {
		t.Fatalf("got %+v", got)
	}
	if s.start["resume_run_id"] != "run-1" || s.start["budget_usd_per_story"] != 3.0 || s.start["repo"] != "acme/widgets" || s.start["mode"] != "review" {
		t.Errorf("start = %v", s.start)
	}
	if findings, _ := s.start["findings"].([]any); len(findings) != 1 {
		t.Errorf("findings = %v", s.start["findings"])
	}
	if _, sent := s.start["patch"]; sent {
		t.Error("a resume sent a patch")
	}
}

func TestResumeWithoutBudgetLeavesTheLimitToTheServer(t *testing.T) {
	s := newResumeServer(t, "run-1")
	command(t, s.vars(), noSleep, "resume", "run-1")
	if _, sent := s.start["budget_usd_per_story"]; sent {
		t.Errorf("start = %v", s.start)
	}
}

func TestResumeOnAServerThatStartedTheTaskAfreshStopsThatRun(t *testing.T) {
	s := newResumeServer(t, "")
	got := command(t, s.vars(), noSleep, "resume", "run-1")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "does not resume runs yet") || !strings.Contains(got.stderr, "run-2 was stopped") {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(s.sent(), "POST /api/runs/run-2/stop") {
		t.Errorf("requests = %s", s.sent())
	}
}

func TestResumeOfARunStillGoingSendsNothing(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.paused = `{"id":"run-1","repo":"acme/widgets","status":"running","findings":[{"id":"TASK-1"}]}`
	got := command(t, s.vars(), noSleep, "resume", "run-1")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "has not ended") {
		t.Errorf("got %+v", got)
	}
	if strings.Contains(s.sent(), "POST") {
		t.Errorf("requests = %s", s.sent())
	}
}

func TestResumeOfARunWithSeveralTasksSendsNothing(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.paused = `{"id":"run-1","repo":"acme/widgets","status":"blocked","findings":[{"id":"A"},{"id":"B"}]}`
	got := command(t, s.vars(), noSleep, "resume", "run-1")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "2 tasks") {
		t.Errorf("got %+v", got)
	}
	if strings.Contains(s.sent(), "POST") {
		t.Errorf("requests = %s", s.sent())
	}
}

func TestResumeWrongArgumentsExitTwo(t *testing.T) {
	s := newResumeServer(t, "run-1")
	for _, args := range [][]string{{"resume"}, {"resume", "run-1", "run-2"}, {"resume", "--budget", "-1", "run-1"}, {"resume", "../x"}} {
		if got := command(t, s.vars(), noSleep, args...); got.code != ExitUsage {
			t.Errorf("sentinel %q = %+v", args, got)
		}
	}
	if sent := s.sent(); sent != "" {
		t.Errorf("requests = %s", sent)
	}
}
