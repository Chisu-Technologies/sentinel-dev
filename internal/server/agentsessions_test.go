package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlyARunThatAsksForItSendsTheQAToTheAgent(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"R1","status":"running"}`)
	}))
	defer srv.Close()
	client := newClient(t, srv.URL)
	ctx := context.Background()

	for _, byAgent := range []bool{true, false} {
		if _, err := client.StartVerify(ctx, StartRequest{Repository: "acme/demo", PullNumber: 3, QAByAgent: byAgent}); err != nil {
			t.Fatal(err)
		}
		if _, err := client.StartTask(ctx, TaskRequest{Repository: "acme/demo", Task: "fix it", BaseCommit: "abc", QAByAgent: byAgent}); err != nil {
			t.Fatal(err)
		}
	}

	for i, body := range bodies {
		runner, sent := body["qa_runner"]
		if want := i < 2; sent != want || (sent && runner != "agent") {
			t.Errorf("request %d: qa_runner = %v (sent %v), want sent %v", i, runner, sent, want)
		}
	}
}

func TestAServerWithoutAgentSessionsListsNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"detail":"Not Found"}`)
	}))
	defer srv.Close()

	sessions, err := newClient(t, srv.URL).AgentSessions(context.Background(), "R1")

	if err != nil || len(sessions) != 0 {
		t.Errorf("a hosted or older server has no mailbox: want no sessions and no error, got %v, %v", sessions, err)
	}
}
