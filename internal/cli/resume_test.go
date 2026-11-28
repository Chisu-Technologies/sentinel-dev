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
	// replyTaken is whether the server says it took the reply.
	replyTaken bool
	// pausedStories and resumedStories are the stories of run-1 and run-2.
	pausedStories  string
	resumedStories string
	resumedStatus  string
}

// noSleep makes waiting for a run instant.
func noSleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func newResumeServer(t *testing.T, echo string) *resumeServer {
	t.Helper()
	s := &resumeServer{
		echo: echo, paused: pausedRun, pausedStories: `[]`, resumedStatus: "completed",
		resumedStories: `[{"state":"done","title":"Fix the login","pr_url":"https://github.com/acme/widgets/pull/9","cost_usd":0.4}]`,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/runs/run-1":
			io.WriteString(w, s.paused)
		case "GET /api/runs/run-1/stories":
			io.WriteString(w, s.pausedStories)
		case "POST /api/runs":
			json.Unmarshal(body, &s.start)
			w.WriteHeader(http.StatusCreated)
			taken := ""
			if s.replyTaken {
				taken = `,"reply_taken":true`
			}
			io.WriteString(w, `{"id":"run-2","status":"running","resumed_from":"`+s.echo+`"`+taken+`}`)
		case "POST /api/runs/run-2/stop":
			io.WriteString(w, `{"id":"run-2","status":"stopped"}`)
		case "GET /api/runs/run-2":
			io.WriteString(w, `{"id":"run-2","status":"`+s.resumedStatus+`"}`)
		case "GET /api/runs/run-2/stories":
			io.WriteString(w, s.resumedStories)
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

const askedStories = `[{"state":"blocked","title":"Fix the login","blocked_reason":"needs_input","blocked_detail":"Which database do I test against?"}]`

func TestResumeSendsWhatThePersonWrote(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.replyTaken = true
	got := command(t, s.vars(), noSleep, "resume", "--reply", "Use the test database.", "run-1")

	if got.code != ExitOK || !strings.Contains(got.stdout, "https://github.com/acme/widgets/pull/9") {
		t.Fatalf("got %+v", got)
	}
	if s.start["reply"] != "Use the test database." {
		t.Errorf("start = %v", s.start)
	}
}

func TestResumeWithoutAReplySendsNone(t *testing.T) {
	s := newResumeServer(t, "run-1")
	command(t, s.vars(), noSleep, "resume", "run-1")
	if _, sent := s.start["reply"]; sent {
		t.Errorf("start = %v", s.start)
	}
}

func TestResumeOnAServerThatDroppedTheReplyStopsThatRun(t *testing.T) {
	s := newResumeServer(t, "run-1")
	got := command(t, s.vars(), noSleep, "resume", "--reply", "Use the test database.", "run-1")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "does not take a reply yet") || !strings.Contains(got.stderr, "run-2 was stopped") {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(s.sent(), "POST /api/runs/run-2/stop") {
		t.Errorf("requests = %s", s.sent())
	}
}

func TestResumeOfARunPausedWithAQuestionAsksForTheAnswer(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.pausedStories = askedStories
	got := command(t, s.vars(), noSleep, "resume", "run-1")

	if got.code != ExitUsage || !strings.Contains(got.stderr, "Which database do I test against?") || !strings.Contains(got.stderr, `sentinel resume run-1 --reply "`) {
		t.Errorf("got %+v", got)
	}
	if strings.Contains(s.sent(), "POST") {
		t.Errorf("requests = %s", s.sent())
	}
}

func TestResumeOfARunPausedWithAQuestionGoesOnWithTheAnswer(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.pausedStories = askedStories
	s.replyTaken = true
	got := command(t, s.vars(), noSleep, "resume", "--reply", "The test one.", "run-1")
	if got.code != ExitOK || s.start["reply"] != "The test one." {
		t.Errorf("got %+v, start = %v", got, s.start)
	}
}

func TestARunPausedWithAQuestionSaysHowToAnswerIt(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.resumedStatus = "blocked"
	s.resumedStories = askedStories
	got := command(t, s.vars(), noSleep, "resume", "run-1")

	if got.code != ExitFailed {
		t.Fatalf("got %+v", got)
	}
	for _, want := range []string{"Question", "Which database do I test against?", `sentinel resume run-2 --reply "<your answer>"`} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "Reason") {
		t.Errorf("a question is not shown as a reason:\n%s", got.stdout)
	}
}

func TestResumeRefusesAReplyThatSaysNothingOrIsTooLong(t *testing.T) {
	s := newResumeServer(t, "run-1")
	for _, reply := range []string{"", "   ", strings.Repeat("x", 4001)} {
		if got := command(t, s.vars(), noSleep, "resume", "--reply", reply, "run-1"); got.code != ExitUsage {
			t.Errorf("--reply of %d characters = %+v", len(reply), got)
		}
	}
	if sent := s.sent(); sent != "" {
		t.Errorf("requests = %s", sent)
	}
}

func TestResumeOfARunThatCannotBeContinuedSaysWhy(t *testing.T) {
	s := newResumeServer(t, "run-1")
	s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/runs/run-1":
			io.WriteString(w, pausedRun)
		case "GET /api/runs/run-1/stories":
			io.WriteString(w, `[]`)
		case "POST /api/runs":
			w.WriteHeader(http.StatusConflict)
			io.WriteString(w, `{"detail":{"code":"resume_run_not_paused","message":"No story of this run ended in a way a resume can continue"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	got := command(t, s.vars(), noSleep, "resume", "run-1")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "cannot be continued") || strings.Contains(got.stderr, "did not pause at its budget") {
		t.Errorf("got %+v", got)
	}
}
