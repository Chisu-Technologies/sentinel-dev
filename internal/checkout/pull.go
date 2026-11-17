package checkout

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// PullResult says what Pull did.
type PullResult struct {
	Branch string
	Commit string
	// Created is true when the branch did not exist before.
	Created bool
}

// ErrUncommitted means the working tree has changes to tracked files. Pull
// switches branches, and it never stashes, merges or discards anything.
var ErrUncommitted = errors.New("this working tree has uncommitted changes")

// Pull fetches pull request number from the GitHub repository of dir into the
// local branch and switches to it. A branch that already exists only moves
// forward: a local commit the pull request does not have stops Pull before
// anything changes.
func Pull(ctx context.Context, runner execx.Runner, dir, repository string, number int, branch string) (PullResult, error) {
	r := repo{ctx: ctx, runner: runner, dir: dir}
	root, err := r.line("rev-parse", "--show-toplevel")
	if err != nil {
		return PullResult{}, errors.New("this directory is not inside a git repository")
	}
	r.dir = root
	local, remote, _, err := r.githubRemote()
	if err != nil {
		return PullResult{}, err
	}
	if !strings.EqualFold(local, repository) {
		return PullResult{}, fmt.Errorf("the pull request is in %s and this working tree is %s", repository, local)
	}
	if _, err := r.line("check-ref-format", "refs/heads/"+branch); err != nil {
		return PullResult{}, fmt.Errorf("%q is not a valid branch name", branch)
	}
	if changed, err := r.line("status", "--porcelain", "--untracked-files=no"); err != nil {
		return PullResult{}, err
	} else if changed != "" {
		return PullResult{}, ErrUncommitted
	}

	return r.fetchAndSwitch(remote, number, branch)
}

// fetchAndSwitch is the part of Pull that changes the repository. from is the
// remote to fetch from; tests pass a local path, as nothing here may reach
// GitHub.
func (r repo) fetchAndSwitch(from string, number int, branch string) (PullResult, error) {
	if _, err := r.line("fetch", "--quiet", "--no-tags", from, "refs/pull/"+strconv.Itoa(number)+"/head"); err != nil {
		return PullResult{}, fmt.Errorf("could not fetch pull request %d: %w", number, err)
	}
	commit, err := r.line("rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return PullResult{}, err
	}

	result := PullResult{Branch: branch, Commit: commit}
	if _, err := r.line("rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		result.Created = true
		_, err := r.line("switch", "--quiet", "-c", branch, commit)
		return result, err
	}
	// The branch exists: it may hold commits made after an earlier pull.
	if _, err := r.line("merge-base", "--is-ancestor", "refs/heads/"+branch, commit); err != nil {
		return PullResult{}, fmt.Errorf("branch %s has commits the pull request does not have; nothing was changed", branch)
	}
	if _, err := r.line("switch", "--quiet", branch); err != nil {
		return PullResult{}, err
	}
	_, err = r.line("merge", "--quiet", "--ff-only", commit)
	return result, err
}
