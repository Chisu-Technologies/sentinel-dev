package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

// mcpSession sends lines to `sentinel mcp` and returns its answers by id. It
// fails the test if anything that is not one JSON-RPC message per line reaches
// stdout, or if the key reaches either stream.
func mcpSession(t *testing.T, vars map[string]string, lines ...string) (map[string]map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"mcp"}, Env{
		Stdin:  strings.NewReader(strings.Join(lines, "\n") + "\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: &execxtest.Fake{},
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string { return vars[name] },
	})
	if code != ExitOK {
		t.Fatalf("sentinel mcp exited %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), verifyKey) || strings.Contains(stderr.String(), verifyKey) {
		t.Fatalf("the API key reached the output")
	}
	answers := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("stdout carried a line that is not JSON-RPC: %q", line)
		}
		id, _ := json.Marshal(msg["id"])
		answers[string(id)] = msg
	}
	return answers, stderr.String()
}

func toolText(t *testing.T, answer map[string]any) (string, bool) {
	t.Helper()
	result, ok := answer["result"].(map[string]any)
	if !ok {
		t.Fatalf("not a tool result: %v", answer)
	}
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

func call(id int, tool string, args string) string {
	return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
}

func TestMCPHandshakeListsTheToolsAndAnswersOnlyRequests(t *testing.T) {
	answers, _ := mcpSession(t, nil,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/list"}`,
	)
	if len(answers) != 3 {
		t.Fatalf("want 3 answers (the notification gets none), got %d: %v", len(answers), answers)
	}
	init := answers["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" {
		t.Errorf("the server should answer with the version the client asked for, got %v", init["protocolVersion"])
	}
	var names []string
	for _, tool := range answers["2"]["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	want := "sentinel_verify_pull_request sentinel_run_task sentinel_run_status sentinel_list_runs sentinel_list_repos sentinel_brain sentinel_stop_run sentinel_session_open sentinel_session_call"
	if strings.Join(names, " ") != want {
		t.Errorf("tools = %v, want %s", names, want)
	}
	if code := answers["3"]["error"].(map[string]any)["code"].(float64); code != -32601 {
		t.Errorf("an unknown method should be -32601, got %v", code)
	}
}

func TestMCPListRunsRunsTheCommandWithJSON(t *testing.T) {
	l := newListServer(t, twoRuns)
	answers, _ := mcpSession(t, listVars(l), call(1, "sentinel_list_runs", `{"limit":2}`))

	text, isError := toolText(t, answers["1"])
	if isError {
		t.Fatalf("unexpected tool error: %s", text)
	}
	var runs []runSummary
	if err := json.Unmarshal([]byte(text), &runs); err != nil || len(runs) != 2 || runs[0].ID != "run-2" {
		t.Fatalf("want the two runs as JSON, got %q (%v)", text, err)
	}
	if got := l.requestList(); len(got) != 1 || !strings.HasPrefix(got[0], "GET /api/runs?") || !strings.Contains(got[0], "limit=2") {
		t.Errorf("requests = %v", got)
	}
}

func TestMCPRefusesAnArgumentTheToolDoesNotTake(t *testing.T) {
	l := newListServer(t, twoRuns)
	answers, _ := mcpSession(t, listVars(l), call(1, "sentinel_list_runs", `{"limt":2}`))

	text, isError := toolText(t, answers["1"])
	if !isError || !strings.Contains(text, "limt") {
		t.Errorf("a misspelt argument should be refused by name, got %q (isError=%v)", text, isError)
	}
	if got := l.requestList(); len(got) != 0 {
		t.Errorf("nothing should reach the server, got %v", got)
	}
}

func TestMCPAnArgumentThatLooksLikeAFlagStaysAnArgument(t *testing.T) {
	l := newListServer(t, twoRuns)
	answers, _ := mcpSession(t, listVars(l), call(1, "sentinel_stop_run", `{"run_id":"--help"}`))

	// Read as a flag, it would print the help of cancel and send nothing.
	text, _ := toolText(t, answers["1"])
	if strings.Contains(text, "Usage:") {
		t.Errorf(`"--help" was read as a flag: %q`, text)
	}
	got := l.requestList()
	if len(got) == 0 || !strings.Contains(got[0], "/api/runs/--help") {
		t.Errorf(`"--help" should reach the server as the run id, requests = %v`, got)
	}
}

func TestMCPRunStatusGivesTheRunAndItsVerdict(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/runs/run-2":
			io.WriteString(w, `{"id":"run-2","repo":"acme/widgets","status":"completed","mode":"verify_pr","total_cost_usd":0.31}`)
		case "/api/runs/run-2/stories":
			io.WriteString(w, `[{"state":"done","title":"Fix the login","pr_url":"https://github.com/acme/widgets/pull/12",
				"cost_usd":0.31,"evidence":{"verify_verdict":"APPROVED","tests_passed":41,"tests_total":41}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	vars := map[string]string{"SENTINEL_URL": srv.URL, "SENTINEL_API_KEY": verifyKey}

	answers, _ := mcpSession(t, vars, call(1, "sentinel_run_status", `{"run_id":"run-2"}`))

	text, isError := toolText(t, answers["1"])
	if isError {
		t.Fatalf("unexpected tool error: %s", text)
	}
	var status struct {
		Run     runSummary    `json:"run"`
		Stories []storyStatus `json:"stories"`
	}
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("not JSON: %q", text)
	}
	if status.Run.Status != "completed" || len(status.Stories) != 1 {
		t.Fatalf("status = %+v", status)
	}
	s := status.Stories[0]
	if s.Verdict != "APPROVED" || s.TestsPassed == nil || *s.TestsPassed != 41 || s.PullRequest == nil {
		t.Errorf("story = %+v", s)
	}
}

// brainServer answers like a Sentinel with two repositories: acme/widgets,
// which the Brain read, and acme/empty, which it has not read yet.
func brainServer(t *testing.T) (map[string]string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		switch r.URL.Path {
		case "/api/org/repos":
			io.WriteString(w, `[{"id":"r1","full_name":"acme/widgets","context_status":"ready","context_version":3,"digest_status":"ready"},
				{"id":"r2","full_name":"acme/empty","context_status":"none","digest_status":"none"}]`)
		case "/api/org/repos/r1/context":
			artifact, _ := json.Marshal(map[string]any{
				"commit_sha": "abc123",
				"index": map[string]any{"artifacts": []any{
					map[string]any{"path": "README.md", "kind": "markdown", "headings": []string{"Widgets", "Install"}},
					map[string]any{"path": "src/widgets/core.py", "kind": "python"},
				}},
				"digest": map[string]any{"src/widgets": "Builds and renders widgets."},
			})
			json.NewEncoder(w).Encode(map[string]any{"repo": "acme/widgets", "status": "ready", "version": 3,
				"commit_sha": "abc123", "built_at": "2026-11-19T10:00:00Z", "artifact_json": string(artifact)})
		case "/api/org/repos/r2/context":
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"detail":"no context artifact"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return map[string]string{"SENTINEL_URL": srv.URL, "SENTINEL_API_KEY": verifyKey}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func TestMCPListsTheRepositoriesAndWhatTheBrainKnowsOfOne(t *testing.T) {
	vars, seen := brainServer(t)
	answers, _ := mcpSession(t, vars,
		call(1, "sentinel_list_repos", `{}`),
		call(2, "sentinel_brain", `{"repo":"ACME/widgets"}`),
		call(3, "sentinel_brain", `{"repo":"acme/empty"}`),
		call(4, "sentinel_brain", `{"repo":"acme/missing"}`),
	)

	repos, isError := toolText(t, answers["1"])
	if isError || !strings.Contains(repos, `"repo": "acme/widgets"`) || !strings.Contains(repos, `"brain": "ready"`) {
		t.Errorf("repositories = %s (isError=%v)", repos, isError)
	}
	brain, isError := toolText(t, answers["2"])
	var got struct {
		Repo   string `json:"repo"`
		Commit string `json:"commit_sha"`
		Files  []struct {
			Path     string   `json:"path"`
			Headings []string `json:"headings"`
		} `json:"files"`
		FilesTotal int               `json:"files_total"`
		Digest     map[string]string `json:"digest"`
	}
	if err := json.Unmarshal([]byte(brain), &got); isError || err != nil {
		t.Fatalf("brain = %s (isError=%v, %v)", brain, isError, err)
	}
	if got.Repo != "acme/widgets" || got.Commit != "abc123" || got.FilesTotal != 2 ||
		got.Files[0].Path != "README.md" || got.Files[0].Headings[1] != "Install" ||
		got.Digest["src/widgets"] != "Builds and renders widgets." {
		t.Errorf("brain = %+v", got)
	}
	if text, isError := toolText(t, answers["3"]); isError || !strings.Contains(text, "has not read acme/empty yet") {
		t.Errorf("a repository not read yet = %q (isError=%v)", text, isError)
	}
	if text, isError := toolText(t, answers["4"]); !isError || !strings.Contains(text, "sentinel_list_repos") {
		t.Errorf("an unknown repository = %q (isError=%v)", text, isError)
	}
	for _, req := range seen() {
		if !strings.HasPrefix(req, "GET ") {
			t.Errorf("a read-only tool sent %s", req)
		}
	}
}

func TestMCPWithoutAServerSaysWhatIsMissing(t *testing.T) {
	answers, _ := mcpSession(t, nil, call(1, "sentinel_run_status", `{"run_id":"run-2"}`))

	text, isError := toolText(t, answers["1"])
	if !isError || !strings.Contains(text, "SENTINEL_URL") {
		t.Errorf("want the missing setting named, got %q", text)
	}
}
