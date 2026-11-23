package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout/checkouttest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

func runIn(t *testing.T, runner execx.Runner, dir string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Env{Stdout: &stdout, Stderr: &stderr, Runner: runner, Dir: dir})
	return result{code, stdout.String(), stderr.String()}
}

func TestRunNeedsExactlyOneTask(t *testing.T) {
	fake := &execxtest.Fake{}
	for _, args := range [][]string{{"run"}, {"run", "--dry-run"}, {"run", "--dry-run", "  "}, {"run", "--dry-run", "one", "two"}} {
		if got := runIn(t, fake, "", args...); got.code != ExitUsage {
			t.Errorf("sentinel %q: status = %d, want %d", args, got.code, ExitUsage)
		}
	}
	if len(fake.Calls) != 0 {
		t.Errorf("a rejected command line still ran %+v", fake.Calls)
	}
}

// sendTask runs `sentinel run` against a fake server. The waits between polls
// return at once.
func sendTask(t *testing.T, runner execx.Runner, dir string, vars map[string]string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"run"}, args...), Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: runner,
		Dir:    dir,
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string { return vars[name] },
		Sleep:  func(ctx context.Context, d time.Duration) error { return ctx.Err() },
	})
	res := result{code, stdout.String(), stderr.String()}
	if strings.Contains(res.stdout, verifyKey) || strings.Contains(res.stderr, verifyKey) {
		t.Errorf("the API key reached the output:\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
	}
	return res
}

const taskStory = `[{"state":"done","title":"T","pr_url":"https://github.com/acme/widgets/pull/31","cost_usd":0.84,
	"blocked_reason":null,"blocked_detail":null,
	"evidence":{"tests_passed":12,"tests_total":12},
	"diff_files":2,"diff_added":9,"diff_removed":1}]`

func TestRunNamesAMissingVariableBeforeReadingTheTree(t *testing.T) {
	fake := &execxtest.Fake{}
	got := sendTask(t, fake, "", map[string]string{}, "fix the flaky login test")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "SENTINEL_URL") {
		t.Errorf("sentinel run = %+v, want a failure naming SENTINEL_URL", got)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("read the tree although nothing could be sent: %+v", fake.Calls)
	}
}

func TestRunSendsTheTaskAndThePatchAndWaitsForThePullRequest(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("one\nTWO\nthree\n"))
	srv := newFakeServer(t, taskStory, "queued", "running", "completed")
	srv.echoPatch = true

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "--base", "release", "fix the flaky login test")

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	want := "Task          fix the flaky login test\n" +
		"Repository    acme/widgets\n" +
		"Run           run-7 (completed)\n" +
		"Pull request  https://github.com/acme/widgets/pull/31\n" +
		"Verdict       APPROVED\n" +
		"Tests         12 of 12 passed\n" +
		"Changes       2 files, +9 -1\n" +
		"Cost          $0.84\n"
	if got.stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", got.stdout, want)
	}
	var start struct {
		Repo       string `json:"repo"`
		Mode       string `json:"mode"`
		BaseRef    string `json:"base_ref"`
		BaseCommit string `json:"base_commit"`
		Patch      string `json:"patch"`
		Findings   []struct {
			Description string `json:"description"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(srv.bodies[0]), &start); err != nil {
		t.Fatal(err)
	}
	if start.Repo != "acme/widgets" || start.Mode != "review" || start.BaseRef != "release" ||
		start.BaseCommit != f.Base || !strings.Contains(start.Patch, "+TWO") ||
		len(start.Findings) != 1 || start.Findings[0].Description != "fix the flaky login test" {
		t.Errorf("start body = %s", srv.bodies[0])
	}
}

func TestRunOnACleanTreeSendsNoPatch(t *testing.T) {
	f := checkouttest.New(t)
	srv := newFakeServer(t, taskStory, "queued", "completed")

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "add a health check")

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	if strings.Contains(srv.bodies[0], `"patch"`) {
		t.Errorf("start body = %s", srv.bodies[0])
	}
}

func TestRunStopsTheRunWhenTheServerIgnoredThePatch(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("changed\n"))
	srv := newFakeServer(t, taskStory, "queued", "completed")

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "fix it")

	if got.code != ExitFailed || !strings.Contains(got.stderr, "does not take local work yet") {
		t.Errorf("sentinel run = %+v", got)
	}
	if strings.Join(srv.requests, ",") != "POST /api/runs,POST /api/runs/run-7/stop" {
		t.Errorf("requests = %v", srv.requests)
	}
}

func TestRunABlockedTaskExitsOneWithTheReason(t *testing.T) {
	f := checkouttest.New(t)
	srv := newFakeServer(t, `[{"state":"blocked","title":"T","pr_url":null,"cost_usd":0,
		"blocked_reason":"local_work_rejected","blocked_detail":"the local work changes infra/deploy.sh, which this organization does not allow",
		"evidence":null,"diff_files":0,"diff_added":0,"diff_removed":0}]`, "queued", "blocked")

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "fix it")

	if got.code != ExitFailed || !strings.Contains(got.stdout, "Reason        the local work changes infra/deploy.sh") {
		t.Errorf("sentinel run = %+v", got)
	}
}

// A story cut by the server's time limit, or by a stop, reads no cost of its
// own: the work that would have written it is what was cut. Measured on
// 2026-11-22: the command printed $0.00 for a run whose model calls cost $2.76.
func TestRunShowsWhatTheRunCostWhenItsStoryWasCutShort(t *testing.T) {
	f := checkouttest.New(t)
	srv := newFakeServer(t, `[{"state":"blocked","title":"T","pr_url":null,"cost_usd":0,
		"blocked_reason":"activity_error","blocked_detail":"The processing activity failed: activity StartToClose timeout",
		"evidence":null,"diff_files":0,"diff_added":0,"diff_removed":0}]`, "queued", "blocked")
	srv.runCost = "2.76"

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "--json", "fix it")

	var out runResult
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil || out.CostUSD == nil || *out.CostUSD != 2.76 {
		t.Errorf("stdout = %s (%v), want cost_usd 2.76", got.stdout, err)
	}
}

func TestRunNoWaitStopsAfterTheRunIsCreated(t *testing.T) {
	f := checkouttest.New(t)
	srv := newFakeServer(t, taskStory, "queued")

	got := sendTask(t, execx.OS{}, f.Work, serverVars(srv), "--no-wait", "--json", "fix it")

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	var out runResult
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil || out.RunID != "run-7" || out.Status != "queued" || out.BaseCommit != f.Base {
		t.Errorf("stdout = %s (%v)", got.stdout, err)
	}
	if srv.requestCount() != 1 {
		t.Errorf("requests = %v", srv.requests)
	}
}

func TestDryRunShowsWhatATaskWouldSend(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("one\nTOP-SECRET-LINE\nthree\n"))
	checkouttest.Write(t, f.Work, "src/new.txt", []byte("new\n"))

	got := runIn(t, execx.OS{}, f.Work, "run", "--dry-run", "fix the flaky login test")

	if got.code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", got.code, got.stderr)
	}
	for _, want := range []string{
		"Task         fix the flaky login test\n",
		"Repository   acme/widgets\n",
		"Base commit  " + f.Base + "\n",
		"Patch        2 files, ",
		"  src/app.txt\n",
		"  src/new.txt\n",
		"Nothing was sent: this was a dry run.\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "TOP-SECRET-LINE") {
		t.Error("the dry run printed the content of a file")
	}
}

func TestDryRunJSONDescribesThePatchWithoutCarryingIt(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("one\nTOP-SECRET-LINE\nthree\n"))

	got := runIn(t, execx.OS{}, f.Work, "run", "--dry-run", "--json", "tidy up")

	var out struct {
		Task       string   `json:"task"`
		Repository string   `json:"repository"`
		BaseCommit string   `json:"base_commit"`
		Files      []string `json:"files"`
		PatchBytes int      `json:"patch_bytes"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil {
		t.Fatalf("the output does not parse: %v\n%s", err, got.stdout)
	}
	if got.code != ExitOK || out.Task != "tidy up" || out.Repository != "acme/widgets" || out.BaseCommit != f.Base {
		t.Errorf("status %d, output %+v", got.code, out)
	}
	if len(out.Files) != 1 || out.Files[0] != "src/app.txt" || out.PatchBytes == 0 {
		t.Errorf("files = %q, patch_bytes = %d", out.Files, out.PatchBytes)
	}
	if strings.Contains(got.stdout, "TOP-SECRET-LINE") {
		t.Error("the JSON carries the content of a file")
	}
}

func TestDryRunOnACleanTreeSaysThereIsNoPatch(t *testing.T) {
	f := checkouttest.New(t)
	got := runIn(t, execx.OS{}, f.Work, "run", "--dry-run", "tidy up")
	if got.code != ExitOK || !strings.Contains(got.stdout, "Patch        none") {
		t.Errorf("sentinel run --dry-run on a clean tree = %+v", got)
	}
}

func TestDryRunOutsideARepositoryFailsWithTheReason(t *testing.T) {
	got := runIn(t, execx.OS{}, t.TempDir(), "run", "--dry-run", "tidy up")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "not inside a git repository") {
		t.Errorf("sentinel run --dry-run outside a repository = %+v", got)
	}
}
