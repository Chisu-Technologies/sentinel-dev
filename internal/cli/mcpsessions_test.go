package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// sessionServer plays the mailbox of a run whose QA the user's agent answers:
// the first answer to a call is "running", as when the tests take a while.
type sessionServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newSessionServer(t *testing.T) *sessionServer {
	t.Helper()
	s := &sessionServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, r.Method+" "+r.URL.Path+" "+string(raw))
		s.mu.Unlock()
		switch r.URL.Path {
		case "/api/org/agent-sessions/S1/open":
			io.WriteString(w, `{"token":"tok-123456789012345","role":"reviewer-a","run_id":"R1","instructions":"Review the change.",`+
				`"tools":[{"name":"submit_verdict","description":"Verdict","input_schema":{"type":"object"}}]}`)
		case "/api/org/agent-sessions/S1/call":
			io.WriteString(w, `{"status":"running","call":1}`)
		case "/api/org/agent-sessions/S1/result":
			io.WriteString(w, `{"status":"answered","call":1,"result":{"messages":[{"role":"tool","content":"ok"}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestASubagentOpensItsSessionAndWaitsOutARunningCall(t *testing.T) {
	s := newSessionServer(t)
	vars := map[string]string{"SENTINEL_URL": s.URL, "SENTINEL_API_KEY": verifyKey}
	answers, _ := mcpSession(t, vars,
		call(1, "sentinel_session_open", `{"session_id":"S1","model":"claude-opus"}`),
		call(2, "sentinel_session_call", `{"session_id":"S1","token":"tok-123456789012345","tool":"submit_verdict","input":{"approved":true}}`),
		call(3, "sentinel_session_call", `{"session_id":"S1","token":"tok-123456789012345"}`),
	)

	opened, isError := toolText(t, answers["1"])
	if isError || !strings.Contains(opened, `"instructions": "Review the change."`) || !strings.Contains(opened, "submit_verdict") {
		t.Fatalf("open should hand over the instructions and the tools, got %q", opened)
	}
	called, isError := toolText(t, answers["2"])
	var answer struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if isError || json.Unmarshal([]byte(called), &answer) != nil || answer.Status != "answered" {
		t.Fatalf("a call that said running should be waited out, got %q", called)
	}
	text, isError := toolText(t, answers["3"])
	if !isError || !strings.Contains(text, "either the tool") {
		t.Errorf("a call with neither a tool nor a call number should be refused, got %q", text)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	want := []string{
		`POST /api/org/agent-sessions/S1/open {"model":"claude-opus"}`,
		`POST /api/org/agent-sessions/S1/call {"input":{"approved":true},"name":"submit_verdict","token":"tok-123456789012345"}`,
		`POST /api/org/agent-sessions/S1/result {"call":1,"token":"tok-123456789012345"}`,
	}
	if strings.Join(s.bodies, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(s.bodies, "\n"), strings.Join(want, "\n"))
	}
}

func TestACallStillRunningAfterTheWaitSaysSo(t *testing.T) {
	old := sessionCallWait
	sessionCallWait = 0
	t.Cleanup(func() { sessionCallWait = old })
	s := newSessionServer(t)
	vars := map[string]string{"SENTINEL_URL": s.URL, "SENTINEL_API_KEY": verifyKey}
	start := time.Now()
	answers, _ := mcpSession(t, vars,
		call(1, "sentinel_session_call", `{"session_id":"S1","token":"tok-123456789012345","tool":"submit_verdict","input":{}}`),
	)
	text, isError := toolText(t, answers["1"])
	if isError || !strings.Contains(text, `"status": "running"`) || !strings.Contains(text, `"call": 1`) {
		t.Fatalf("past the wait the subagent should be told to ask again with call 1, got %q", text)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the wait should be bounded")
	}
}
