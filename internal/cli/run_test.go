package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

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

func TestRunWithoutDryRunSaysItCannotSendYetAndTouchesNothing(t *testing.T) {
	fake := &execxtest.Fake{}
	got := runIn(t, fake, "", "run", "fix the flaky login test")
	if got.code != ExitFailed || got.stdout != "" {
		t.Errorf("sentinel run = %+v, want a failure with nothing on stdout", got)
	}
	if !strings.Contains(got.stderr, "cannot send a task yet") || !strings.Contains(got.stderr, "--dry-run") {
		t.Errorf("stderr = %q, want the reason and the way forward", got.stderr)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("ran %+v although nothing can be sent", fake.Calls)
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
