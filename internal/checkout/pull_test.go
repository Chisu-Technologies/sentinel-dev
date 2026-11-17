package checkout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout/checkouttest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// openPullRequest pushes a commit on top of the base to refs/pull/7/head of
// the fixture's remote, as GitHub keeps a pull request, and returns it.
func openPullRequest(t *testing.T, f checkouttest.Fixture, content string) string {
	t.Helper()
	author := t.TempDir()
	checkouttest.Git(t, author, nil, "clone", "-q", f.Remote, ".")
	checkouttest.Git(t, author, nil, "config", "core.autocrlf", "false")
	checkouttest.Write(t, author, "src/app.txt", []byte(content))
	checkouttest.Git(t, author, nil, "commit", "-q", "-am", "the change of the run")
	checkouttest.Git(t, author, nil, "push", "-q", "origin", "HEAD:refs/pull/7/head")
	return checkouttest.Git(t, author, nil, "rev-parse", "HEAD")
}

func localRepo(dir string) repo {
	return repo{ctx: context.Background(), runner: execx.OS{}, dir: dir}
}

func TestPullCreatesTheBranchAtThePullRequestAndSwitchesToIt(t *testing.T) {
	f := checkouttest.New(t)
	want := openPullRequest(t, f, "one\ntwo\nthree\nfour\n")

	got, err := localRepo(f.Work).fetchAndSwitch(f.Remote, 7, "sentinel/run-7")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || got.Commit != want {
		t.Errorf("got %+v, want a new branch at %s", got, want)
	}
	if branch := checkouttest.Git(t, f.Work, nil, "symbolic-ref", "--short", "HEAD"); branch != "sentinel/run-7" {
		t.Errorf("on branch %q", branch)
	}
}

func TestPullMovesAnExistingBranchForward(t *testing.T) {
	f := checkouttest.New(t)
	first := openPullRequest(t, f, "first round\n")
	if _, err := localRepo(f.Work).fetchAndSwitch(f.Remote, 7, "sentinel/run-7"); err != nil {
		t.Fatal(err)
	}
	checkouttest.Git(t, f.Work, nil, "switch", "-q", "main")

	// A second round on the same pull request.
	author := t.TempDir()
	checkouttest.Git(t, author, nil, "clone", "-q", f.Remote, ".")
	checkouttest.Git(t, author, nil, "fetch", "-q", "origin", "refs/pull/7/head")
	checkouttest.Git(t, author, nil, "switch", "-q", "--detach", first)
	checkouttest.Write(t, author, "src/app.txt", []byte("second round\n"))
	checkouttest.Git(t, author, nil, "commit", "-q", "-am", "round two")
	checkouttest.Git(t, author, nil, "push", "-q", "origin", "HEAD:refs/pull/7/head")
	second := checkouttest.Git(t, author, nil, "rev-parse", "HEAD")

	got, err := localRepo(f.Work).fetchAndSwitch(f.Remote, 7, "sentinel/run-7")
	if err != nil {
		t.Fatal(err)
	}
	if got.Created || got.Commit != second {
		t.Errorf("got %+v, want the existing branch moved to %s", got, second)
	}
	if head := checkouttest.Git(t, f.Work, nil, "rev-parse", "HEAD"); head != second {
		t.Errorf("HEAD = %s, want %s", head, second)
	}
}

func TestPullNeverDropsALocalCommitOnItsBranch(t *testing.T) {
	f := checkouttest.New(t)
	openPullRequest(t, f, "from the run\n")
	if _, err := localRepo(f.Work).fetchAndSwitch(f.Remote, 7, "sentinel/run-7"); err != nil {
		t.Fatal(err)
	}
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("my own edit\n"))
	checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "mine")
	mine := checkouttest.Git(t, f.Work, nil, "rev-parse", "HEAD")
	checkouttest.Git(t, f.Work, nil, "switch", "-q", "main")

	_, err := localRepo(f.Work).fetchAndSwitch(f.Remote, 7, "sentinel/run-7")
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("err = %v", err)
	}
	if tip := checkouttest.Git(t, f.Work, nil, "rev-parse", "sentinel/run-7"); tip != mine {
		t.Errorf("the branch moved to %s", tip)
	}
	if branch := checkouttest.Git(t, f.Work, nil, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Errorf("switched to %q", branch)
	}
}

func TestPullRefusesUncommittedChangesBeforeFetching(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("work in progress\n"))

	_, err := Pull(context.Background(), execx.OS{}, f.Work, "acme/widgets", 7, "sentinel/run-7")
	if !errors.Is(err, ErrUncommitted) {
		t.Fatalf("err = %v, want ErrUncommitted", err)
	}
	if got := checkouttest.Git(t, f.Work, nil, "show", "HEAD:src/app.txt"); got == "work in progress" {
		t.Error("the change was committed")
	}
}

func TestPullRefusesAPullRequestOfAnotherRepository(t *testing.T) {
	f := checkouttest.New(t)

	_, err := Pull(context.Background(), execx.OS{}, f.Work, "acme/gadgets", 7, "sentinel/run-7")
	if err == nil || !strings.Contains(err.Error(), "acme/gadgets") || !strings.Contains(err.Error(), "acme/widgets") {
		t.Fatalf("err = %v", err)
	}
}
