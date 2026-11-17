package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pullServer answers the read of run-7 with body, and its stories with
// stories.
func pullServer(t *testing.T, body, stories string) map[string]string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/runs/run-7":
			io.WriteString(w, body)
		case "GET /api/runs/run-7/stories":
			io.WriteString(w, stories)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return map[string]string{"SENTINEL_URL": srv.URL, "SENTINEL_API_KEY": verifyKey}
}

func TestPullWrongArgumentsExitTwo(t *testing.T) {
	vars := pullServer(t, `{}`, `[]`)
	for _, args := range [][]string{{"pull"}, {"pull", "run-7", "run-8"}, {"pull", "../run-7"}} {
		if got := command(t, vars, nil, args...); got.code != ExitUsage {
			t.Errorf("sentinel %q = %+v, want status 2", args, got)
		}
	}
}

func TestPullOfARunWithoutAPullRequestSaysSo(t *testing.T) {
	vars := pullServer(t, `{"id":"run-7","status":"running","pr_url":null}`, `[]`)
	got := command(t, vars, nil, "pull", "run-7")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "no pull request yet") || !strings.Contains(got.stderr, "sentinel watch run-7") {
		t.Errorf("got %+v", got)
	}

	vars = pullServer(t, `{"id":"run-7","status":"failed","pr_url":null}`, `[]`)
	got = command(t, vars, nil, "pull", "run-7")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "ended without a pull request: failed") {
		t.Errorf("got %+v", got)
	}
}

func TestPullFetchesOnlyAPullRequestOnGitHub(t *testing.T) {
	vars := pullServer(t, `{"id":"run-7","status":"completed","pr_url":"https://example.com/acme/widgets/pull/7"}`, `[]`)
	got := command(t, vars, nil, "pull", "run-7")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "cannot fetch") {
		t.Errorf("got %+v", got)
	}
}
