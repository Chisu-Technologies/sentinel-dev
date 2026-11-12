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
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

// listServer plays the list endpoint and remembers what it was asked.
type listServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	status   int
	body     string
}

func newListServer(t *testing.T, body string) *listServer {
	t.Helper()
	l := &listServer{body: body}
	l.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.requests = append(l.requests, r.Method+" "+r.URL.RequestURI())
		if l.status != 0 {
			w.WriteHeader(l.status)
		}
		io.WriteString(w, l.body)
	}))
	t.Cleanup(l.Close)
	return l
}

func (l *listServer) requestList() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.requests...)
}

// command runs one command line with a fake environment and checks, as every
// test of a command that talks to a server must, that the key stayed out of
// both streams.
func command(t *testing.T, vars map[string]string, sleep func(context.Context, time.Duration) error, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: &execxtest.Fake{},
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string { return vars[name] },
		Sleep:  sleep,
	})
	res := result{code, stdout.String(), stderr.String()}
	if strings.Contains(res.stdout, verifyKey) || strings.Contains(res.stderr, verifyKey) {
		t.Errorf("the API key reached the output:\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
	}
	return res
}

func listVars(l *listServer) map[string]string {
	return map[string]string{"SENTINEL_URL": l.URL, "SENTINEL_API_KEY": verifyKey}
}

const twoRuns = `[
	{"id":"run-2","repo":"acme/widgets","status":"completed","mode":"verify_pr","story":"Fix the login",
	 "pr_number":"12","pr_url":"https://github.com/acme/widgets/pull/12","total_cost_usd":0.314,
	 "started_at":"2026-01-02T03:04:05.123Z","finished_at":"2026-01-02T03:09:00Z","extra":"ignored"},
	{"id":"run-1","repo":"acme/gadgets","status":"running","story":null,"pr_number":null,"pr_url":null,
	 "total_cost_usd":0,"started_at":null,"finished_at":null}]`

func TestRunsWrongLimitOrArgumentsExitTwoWithoutARequest(t *testing.T) {
	l := newListServer(t, twoRuns)
	for _, args := range [][]string{
		{"runs", "--limit", "0"},
		{"runs", "--limit", "101"},
		{"runs", "--limit", "-3"},
		{"runs", "--limit", "many"},
		{"runs", "extra"},
		{"runs", "--nope"},
	} {
		got := command(t, listVars(l), nil, args...)
		if got.code != ExitUsage || got.stdout != "" {
			t.Errorf("sentinel %q = %+v, want status 2 and no output", args, got)
		}
	}
	if n := len(l.requestList()); n != 0 {
		t.Errorf("a wrong command line sent %d requests", n)
	}
}

func TestRunsAcceptsTheLimitsAtBothEnds(t *testing.T) {
	l := newListServer(t, `[]`)
	for _, limit := range []string{"1", "100"} {
		if got := command(t, listVars(l), nil, "runs", "--limit", limit); got.code != ExitOK {
			t.Errorf("--limit %s: %+v", limit, got)
		}
	}
	want := []string{"GET /api/runs?limit=1&offset=0", "GET /api/runs?limit=100&offset=0"}
	if got := l.requestList(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want %v", got, want)
	}
}

func TestRunsAsksForTwentyByDefault(t *testing.T) {
	l := newListServer(t, `[]`)
	command(t, listVars(l), nil, "runs")
	if got := l.requestList(); len(got) != 1 || got[0] != "GET /api/runs?limit=20&offset=0" {
		t.Errorf("requests = %v", got)
	}
}

func TestRunsNamesTheMissingVariableAndExitsOne(t *testing.T) {
	l := newListServer(t, twoRuns)
	for name, want := range map[string]string{"SENTINEL_URL": "SENTINEL_URL", "SENTINEL_API_KEY": "SENTINEL_API_KEY"} {
		vars := listVars(l)
		delete(vars, name)
		got := command(t, vars, nil, "runs")
		if got.code != ExitFailed || !strings.Contains(got.stderr, want) || got.stdout != "" {
			t.Errorf("without %s: %+v", name, got)
		}
	}
	if n := len(l.requestList()); n != 0 {
		t.Errorf("a missing variable sent %d requests", n)
	}
}

func TestRunsTableHasAHeaderAndOneLinePerRun(t *testing.T) {
	l := newListServer(t, twoRuns)
	got := command(t, listVars(l), nil, "runs")
	if got.code != ExitOK || got.stderr != "" {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two runs, got:\n%s", got.stdout)
	}
	for _, word := range []string{"ID", "STATUS", "REPOSITORY", "PR", "COST", "STARTED", "STORY"} {
		if !strings.Contains(lines[0], word) {
			t.Errorf("header %q lacks %s", lines[0], word)
		}
	}
	first := strings.Fields(lines[1])
	wantFirst := []string{"run-2", "completed", "acme/widgets", "#12", "$0.31", "2026-01-02T03:04", "Fix", "the", "login"}
	if strings.Join(first, " ") != strings.Join(wantFirst, " ") {
		t.Errorf("first line = %q", lines[1])
	}
	// A run with no pull request, no start and no story leaves those cells
	// empty, and the columns still line up.
	second := strings.Fields(lines[2])
	if strings.Join(second, " ") != "run-1 running acme/gadgets $0.00" {
		t.Errorf("second line = %q", lines[2])
	}
	if strings.Index(lines[1], "acme/widgets") != strings.Index(lines[2], "acme/gadgets") {
		t.Errorf("columns are not aligned:\n%s", got.stdout)
	}
}

func TestRunsCutsTheStoryToSixtyCharactersAndCleansIt(t *testing.T) {
	long := strings.Repeat("abcdefghij", 8)
	body, _ := json.Marshal([]map[string]any{{
		"id": "run-1", "repo": "a/b", "status": "queued", "total_cost_usd": 0,
		"story": "\x1b[31m" + long + "\x00",
	}})
	l := newListServer(t, string(body))
	got := command(t, listVars(l), nil, "runs")
	lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
	// The escape is removed first, then the rest is cut: 57 characters and "...".
	want := ("[31m" + long)[:57] + "..."
	if !strings.HasSuffix(lines[1], "  "+want) || len([]rune(want)) != 60 {
		t.Errorf("line = %q, want it to end with the 60 character cell %q", lines[1], want)
	}
	if strings.ContainsAny(got.stdout, "\x1b\x00") {
		t.Errorf("a control character reached stdout: %q", got.stdout)
	}
}

func TestRunsWithNoRunsSaysSo(t *testing.T) {
	l := newListServer(t, `[]`)
	got := command(t, listVars(l), nil, "runs")
	if got.code != ExitOK || got.stdout != "No runs yet.\n" {
		t.Errorf("got %+v", got)
	}
}

func TestRunsJSONPrintsOnlyTheListedFields(t *testing.T) {
	l := newListServer(t, twoRuns)
	got := command(t, listVars(l), nil, "runs", "--json")
	var out []map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil || got.code != ExitOK {
		t.Fatalf("status %d, the output does not parse: %v\n%s", got.code, err, got.stdout)
	}
	if len(out) != 2 {
		t.Fatalf("out = %v", out)
	}
	for _, want := range []string{"id", "repo", "status", "mode", "story", "pr_number", "pr_url", "total_cost_usd", "started_at", "finished_at"} {
		if _, ok := out[0][want]; !ok {
			t.Errorf("the first run lacks %q: %v", want, out[0])
		}
	}
	if _, ok := out[0]["extra"]; ok {
		t.Errorf("a field that is not listed was printed: %v", out[0])
	}
	if out[0]["id"] != "run-2" || out[0]["pr_number"] != "12" || out[0]["total_cost_usd"] != 0.314 {
		t.Errorf("first = %v", out[0])
	}
	// Null stays null and an absent mode stays absent.
	if v, ok := out[1]["story"]; !ok || v != nil {
		t.Errorf("story of the second run = %v, %v", v, ok)
	}
	if _, ok := out[1]["mode"]; ok {
		t.Errorf("an absent mode was printed: %v", out[1])
	}
}

func TestRunsJSONOfNoRunsIsAnEmptyList(t *testing.T) {
	l := newListServer(t, `[]`)
	got := command(t, listVars(l), nil, "runs", "--json")
	if strings.TrimSpace(got.stdout) != "[]" {
		t.Errorf("stdout = %q, want []", got.stdout)
	}
}

func TestRunsServerErrorExitsOne(t *testing.T) {
	l := newListServer(t, `{"detail":"nope"}`)
	l.status = http.StatusUnauthorized
	got := command(t, listVars(l), nil, "runs")
	if got.code != ExitFailed || got.stdout != "" || !strings.Contains(got.stderr, "401") || !strings.Contains(got.stderr, "SENTINEL_API_KEY") {
		t.Errorf("got %+v", got)
	}
}

func TestRunsNeverPrintsTheKeyWhateverTheServerEchoes(t *testing.T) {
	echo := `[{"id":"run-1","repo":"` + verifyKey + `","status":"` + verifyKey[:6] + `\u001b` + verifyKey[6:] +
		`","story":"key ` + verifyKey + `","pr_number":null,"pr_url":"` + verifyKey + `","total_cost_usd":1}]`
	l := newListServer(t, echo)
	for _, args := range [][]string{{"runs"}, {"runs", "--json"}} {
		got := command(t, listVars(l), nil, args...) // command fails the test if the key shows
		if got.code != ExitOK || !strings.Contains(got.stdout, "[redacted]") {
			t.Errorf("sentinel %q = %+v", args, got)
		}
	}
	l.status, l.body = http.StatusForbidden, `{"detail":"`+verifyKey+`"}`
	command(t, listVars(l), nil, "runs")
}
