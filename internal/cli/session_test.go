package cli

import (
	"bytes"
	"context"
	"encoding/json"
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

const sessionKey = "sk-sentinel-session-0123456789"

// architect is a fake Architect. Each "METHOD path" answers its scripted
// bodies in turn, repeating the last one; every request is kept.
type architect struct {
	*httptest.Server
	mu       sync.Mutex
	script   map[string][]string
	status   map[string]int
	requests []sessionRequest
}

type sessionRequest struct {
	route string
	body  map[string]any
	auth  string
}

func newArchitect(t *testing.T, script map[string][]string) *architect {
	t.Helper()
	a := &architect{script: script, status: map[string]int{}}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		route := r.Method + " " + r.URL.Path
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.requests = append(a.requests, sessionRequest{route, body, r.Header.Get("Authorization")})
		answers := a.script[route]
		if len(answers) == 0 {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"detail":"no such route in the test"}`)
			return
		}
		if status := a.status[route]; status != 0 {
			w.WriteHeader(status)
		}
		io.WriteString(w, answers[0])
		if len(answers) > 1 {
			a.script[route] = answers[1:]
		}
	}))
	t.Cleanup(a.Close)
	return a
}

func (a *architect) sent(route string) []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	var bodies []map[string]any
	for _, r := range a.requests {
		if r.route == route {
			bodies = append(bodies, r.body)
		}
	}
	return bodies
}

func runSessionWith(t *testing.T, a *architect, runner execx.Runner, dir, input string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"session"}, args...), Env{
		Stdin:  strings.NewReader(input),
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: runner,
		Dir:    dir,
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string {
			return map[string]string{"SENTINEL_URL": a.URL, "SENTINEL_API_KEY": sessionKey}[name]
		},
		Sleep:    func(ctx context.Context, d time.Duration) error { return ctx.Err() },
		Terminal: func() bool { return true },
	})
	res := result{code, stdout.String(), stderr.String()}
	if strings.Contains(res.stdout+res.stderr, sessionKey) {
		t.Errorf("the API key reached the output:\n%s%s", res.stdout, res.stderr)
	}
	return res
}

const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// planJSON is a plan in state, with two tasks in the given states.
func planJSON(state string, stateRevision int, task1, task2, pr1 string, messages ...string) string {
	var msgs []map[string]string
	for i, m := range messages {
		role := "assistant"
		if i%2 == 1 {
			role = "user"
		}
		msgs = append(msgs, map[string]string{"role": role, "content": m})
	}
	plan := map[string]any{
		"id": "plan_1", "repo": "acme/widgets", "state": state, "state_revision": stateRevision,
		"revision": 3, "finalized_revision": 3, "task_set_sha256": sha, "messages": msgs,
		"plan": map[string]any{"stages": []any{map[string]any{"sprints": []any{map[string]any{"tasks": []any{
			map[string]any{"id": "task-1", "title": "Add the rate limiter", "_state": task1, "_pr_url": pr1},
			map[string]any{"id": "task-2", "title": "Document it", "_state": task2},
		}}}}}},
	}
	data, _ := json.Marshal(plan)
	return string(data)
}

func TestASessionNeedsATerminal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"session", "add a limiter"}, Env{
		Stdin: strings.NewReader("y\n"), Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(string) string { return "" },
	})
	if code != ExitUsage || !strings.Contains(stderr.String(), "sentinel run") {
		t.Errorf("status = %d, stderr = %q", code, stderr.String())
	}
}

func TestPlanModeTalksThePlanThroughAndRunsNothing(t *testing.T) {
	f := checkouttest.New(t)
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans": {planJSON("planning", 1, "", "", "")},
		"GET /api/org/architect/plans/plan_1": {
			planJSON("ready_for_approval", 2, "pending", "pending", "", "Two tasks: the limiter and its docs."),
			planJSON("ready_for_approval", 4, "pending", "pending", "", "Two tasks: the limiter and its docs.", "make it per user", "Per user it is."),
		},
		"POST /api/org/architect/plans/plan_1/message": {planJSON("planning", 3, "", "", "", "Two tasks: the limiter and its docs.", "make it per user")},
	})
	a.status["POST /api/org/architect/plans"] = http.StatusCreated

	got := runSessionWith(t, a, execx.OS{}, f.Work, "make it per user\n/quit\n", "add a rate limiter")

	if got.code != ExitOK {
		t.Fatalf("status = %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	created := a.sent("POST /api/org/architect/plans")
	if len(created) != 1 || created[0]["intent"] != "add a rate limiter" || created[0]["repo"] != "acme/widgets" {
		t.Errorf("created = %v", created)
	}
	msg := a.sent("POST /api/org/architect/plans/plan_1/message")
	if len(msg) != 1 || msg[0]["content"] != "make it per user" || msg[0]["expected_state_revision"] != float64(2) {
		t.Errorf("message = %v", msg)
	}
	if n := len(a.sent("POST /api/org/architect/plans/plan_1/approve")) + len(a.sent("POST /api/org/architect/plans/plan_1/execute")); n != 0 {
		t.Errorf("plan mode approved or ran something: %d requests", n)
	}
	for _, want := range []string{"Architect: Two tasks: the limiter and its docs.", "1. task-1  Add the rate limiter", "Architect: Per user it is.", "sentinel session --plan plan_1"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if strings.Count(got.stdout, "Two tasks") != 1 {
		t.Errorf("a message was shown twice:\n%s", got.stdout)
	}
	for _, r := range a.requests {
		if r.auth != "Bearer "+sessionKey {
			t.Errorf("%s went with Authorization %q", r.route, r.auth)
		}
	}
}

func TestEditModeAsksBeforeEachTask(t *testing.T) {
	f := checkouttest.New(t)
	const pr = "https://github.com/acme/widgets/pull/9"
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans":                {planJSON("ready_for_approval", 2, "pending", "pending", "")},
		"POST /api/org/architect/plans/plan_1/approve": {planJSON("approved", 3, "pending", "pending", "")},
		"POST /api/org/architect/plans/plan_1/execute": {planJSON("executing", 4, "queued", "pending", "")},
		"GET /api/org/architect/plans/plan_1": {
			planJSON("executing", 5, "running", "pending", ""),
			planJSON("executing", 6, "done", "pending", pr),
		},
	})

	got := runSessionWith(t, a, execx.OS{}, f.Work, "y\nn\n", "--mode", "edit", "add a rate limiter")

	if got.code != ExitOK {
		t.Fatalf("status = %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	approve := a.sent("POST /api/org/architect/plans/plan_1/approve")
	if len(approve) != 1 || approve[0]["expected_revision"] != float64(3) || approve[0]["expected_state_revision"] != float64(2) || approve[0]["expected_task_set_sha256"] != sha {
		t.Errorf("approve = %v", approve)
	}
	execute := a.sent("POST /api/org/architect/plans/plan_1/execute")
	if len(execute) != 1 || execute[0]["mode"] != "manual" || execute[0]["expected_state_revision"] != float64(3) {
		t.Fatalf("execute = %v", execute)
	}
	if ids, _ := execute[0]["task_ids"].([]any); len(ids) != 1 || ids[0] != "task-1" {
		t.Errorf("task_ids = %v", execute[0]["task_ids"])
	}
	for _, want := range []string{"Run task-1 «Add the rate limiter»? [y/n]", "task-1: done  " + pr, "Run task-2 «Document it»? [y/n]", "Left the session."} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
}

func TestEditModeSaysNoAndNothingIsApproved(t *testing.T) {
	f := checkouttest.New(t)
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans": {planJSON("ready_for_approval", 2, "pending", "pending", "")},
	})

	got := runSessionWith(t, a, execx.OS{}, f.Work, "n\n", "--mode", "edit", "add a rate limiter")

	if got.code != ExitOK || len(a.sent("POST /api/org/architect/plans/plan_1/approve")) != 0 {
		t.Errorf("status = %d, requests = %+v", got.code, a.requests)
	}
}

func TestAutoModeApprovesAndRunsEveryTaskOnce(t *testing.T) {
	f := checkouttest.New(t)
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans":                {planJSON("ready_for_approval", 2, "pending", "pending", "")},
		"POST /api/org/architect/plans/plan_1/approve": {planJSON("approved", 3, "pending", "pending", "")},
		"POST /api/org/architect/plans/plan_1/execute": {planJSON("executing", 4, "queued", "queued", "")},
		"GET /api/org/architect/plans/plan_1": {
			planJSON("executing", 5, "done", "pending", ""),
			planJSON("executing", 6, "done", "running", ""),
			planJSON("completed", 7, "done", "done", ""),
		},
	})

	got := runSessionWith(t, a, execx.OS{}, f.Work, "", "--mode", "auto", "add a rate limiter")

	if got.code != ExitOK {
		t.Fatalf("status = %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	execute := a.sent("POST /api/org/architect/plans/plan_1/execute")
	if len(execute) != 1 || execute[0]["mode"] != "auto" {
		t.Errorf("execute = %v", execute)
	}
	if !strings.Contains(got.stdout, "The plan is completed.") {
		t.Errorf("stdout:\n%s", got.stdout)
	}
}

func TestAQuestionOfTheArchitectIsAnsweredInTheTerminal(t *testing.T) {
	f := checkouttest.New(t)
	clarifying := `{"id":"plan_1","repo":"acme/widgets","state":"awaiting_clarification","state_revision":2,
		"pending_clarification":{"id":"clar_1","status":"pending","expected_revision":4,"can_answer":true,"continuation_token":"tok",
		"questions":[
			{"id":"scope","header":"Scope","question":"Per user or per IP?","answer_type":"single_select",
			 "choices":[{"id":"user","label":"Per user","tradeoff":"needs sign-in"},{"id":"ip","label":"Per IP","tradeoff":"simpler"}],
			 "recommended_choice_id":"ip"},
			{"id":"limit","header":"Limit","question":"How many per minute?","answer_type":"free_text"}]}}`
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans":                        {clarifying},
		"POST /api/org/architect/clarifications/clar_1/answer": {`{"accepted":true}`},
		"GET /api/org/architect/plans/plan_1":                  {planJSON("ready_for_approval", 3, "pending", "pending", "")},
	})

	got := runSessionWith(t, a, execx.OS{}, f.Work, "3\n\n60\n/quit\n", "add a rate limiter")

	if got.code != ExitOK {
		t.Fatalf("status = %d\nstdout: %s\nstderr: %s", got.code, got.stdout, got.stderr)
	}
	answers := a.sent("POST /api/org/architect/clarifications/clar_1/answer")
	if len(answers) != 1 {
		t.Fatalf("answers = %v", answers)
	}
	data, _ := json.Marshal(answers[0])
	if want := `{"answers":[{"question_id":"scope","value":"ip"},{"question_id":"limit","value":"60"}],"continuation_token":"tok","expected_revision":4}`; string(data) != want {
		t.Errorf("answer body = %s\nwant %s", data, want)
	}
	if !strings.Contains(got.stdout, "Per IP (recommended)") || !strings.Contains(got.stdout, "That is not an answer") {
		t.Errorf("stdout:\n%s", got.stdout)
	}
}

func TestAKeyMadeInThePanelIsToldToSignIn(t *testing.T) {
	f := checkouttest.New(t)
	a := newArchitect(t, map[string][]string{"POST /api/org/architect/plans": {`{"detail":"not authenticated"}`}})
	a.status["POST /api/org/architect/plans"] = http.StatusUnauthorized

	got := runSessionWith(t, a, execx.OS{}, f.Work, "", "add a rate limiter")

	if got.code != ExitFailed || !strings.Contains(got.stderr, "sentinel login") {
		t.Errorf("status = %d, stderr = %q", got.code, got.stderr)
	}
}

func TestABlockedPlanSendsThePersonToThePanel(t *testing.T) {
	f := checkouttest.New(t)
	a := newArchitect(t, map[string][]string{
		"POST /api/org/architect/plans": {`{"id":"plan_1","state":"blocked","state_revision":2,"blocker":{"message":"The model key is missing."}}`},
	})

	got := runSessionWith(t, a, execx.OS{}, f.Work, "", "add a rate limiter")

	if got.code != ExitFailed || !strings.Contains(got.stderr, "The model key is missing.") || !strings.Contains(got.stderr, "--plan plan_1") {
		t.Errorf("status = %d, stderr = %q", got.code, got.stderr)
	}
}
