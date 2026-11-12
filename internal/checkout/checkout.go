// Package checkout packages a working tree so a server can rebuild it: a
// commit GitHub already has, plus one patch with everything that is only on
// this machine (commits not pushed, staged and unstaged changes, new files
// that are not ignored).
//
// Nothing is pushed and nothing in the repository changes: the patch is made
// from a throwaway copy of the index, so the real index, the branch and the
// working tree stay as they were.
package checkout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// MaxPatchBytes is the largest patch a task may carry. A patch is for the work
// in progress of one task; a tree that differs from GitHub by more than this
// should be pushed first.
const MaxPatchBytes = 3 << 20

// Package is what a server needs to rebuild this working tree.
type Package struct {
	// Repository is the GitHub repository, as "owner/name".
	Repository string `json:"repository"`
	// BaseCommit is a commit GitHub is known to have. The patch applies to it.
	BaseCommit string `json:"base_commit"`
	// Patch turns BaseCommit into the working tree. It is empty when the tree
	// has nothing that GitHub lacks.
	Patch []byte `json:"-"`
	// Files lists the paths the patch touches.
	Files []string `json:"files"`
}

// ErrPatchTooLarge means the working tree differs from GitHub by more than
// MaxPatchBytes.
var ErrPatchTooLarge = errors.New("the changes are too large to send as a patch")

// repo runs git commands in one directory.
type repo struct {
	ctx    context.Context
	runner execx.Runner
	dir    string
	// env is added to every command; it is how the throwaway index is chosen.
	env []string
}

// output runs git and returns its standard output, at most limit bytes when
// limit is positive.
func (r repo) output(limit int64, args ...string) (string, error) {
	res, err := r.runner.Run(r.ctx, execx.Command{Name: "git", Args: args, Dir: r.dir, Env: r.env, StdoutLimit: limit})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git %s: %s", args[0], firstLine(res.Stderr))
	}
	return res.Stdout, nil
}

// line runs git and returns its output without the surrounding whitespace.
func (r repo) line(args ...string) (string, error) {
	out, err := r.output(0, args...)
	return strings.TrimSpace(out), err
}

// Build packages the working tree that dir belongs to.
func Build(ctx context.Context, runner execx.Runner, dir string) (Package, error) {
	r := repo{ctx: ctx, runner: runner, dir: dir}

	root, err := r.line("rev-parse", "--show-toplevel")
	if err != nil {
		return Package{}, errors.New("this directory is not inside a git repository")
	}
	r.dir = root
	if _, err := r.line("rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
		return Package{}, errors.New("this repository has no commit yet")
	}

	repository, remote, tracksRemote, err := r.githubRemote()
	if err != nil {
		return Package{}, err
	}

	base, err := r.baseCommit(remote, tracksRemote)
	if err != nil {
		return Package{}, err
	}
	patch, files, err := r.patchFrom(base)
	if err != nil {
		return Package{}, err
	}
	return Package{Repository: repository, BaseCommit: base, Patch: patch, Files: files}, nil
}

// Repository returns the GitHub repository ("owner/name") that the working tree
// of dir belongs to. It reads local git configuration and contacts nobody.
func Repository(ctx context.Context, runner execx.Runner, dir string) (string, error) {
	r := repo{ctx: ctx, runner: runner, dir: dir}
	root, err := r.line("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("this directory is not inside a git repository")
	}
	r.dir = root
	repository, _, _, err := r.githubRemote()
	return repository, err
}

// githubRemote resolves the remote the current branch belongs to and the GitHub
// repository it points at. tracksRemote says whether the branch configures that
// remote, as opposed to falling back to origin.
func (r repo) githubRemote() (repository, remote string, tracksRemote bool, err error) {
	// The branch says which remote it belongs to. "." means it tracks another
	// local branch, which says nothing about GitHub.
	remote, tracksRemote = "origin", false
	if branch, err := r.line("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		if configured, err := r.line("config", "--get", "branch."+branch+".remote"); err == nil && configured != "" && configured != "." {
			remote, tracksRemote = configured, true
		}
	}
	url, err := r.line("remote", "get-url", remote)
	if err != nil {
		return "", "", false, fmt.Errorf("this repository has no remote named %q; Sentinel opens the pull request on GitHub, so it needs one", remote)
	}
	repository, err = ParseGitHubRemote(url)
	if err != nil {
		return "", "", false, fmt.Errorf("remote %q: %w", remote, err)
	}
	return repository, remote, tracksRemote, nil
}

// baseCommit finds a commit of HEAD that the remote is known to have. It
// prefers the branch this one tracks, then the default branch of the remote,
// and only then any other branch of the remote: the first two are the least
// likely to have been deleted on GitHub since the last fetch. It reads what
// that fetch recorded and contacts nobody.
func (r repo) baseCommit(remote string, tracksRemote bool) (string, error) {
	refs := []string{"refs/remotes/" + remote + "/HEAD"}
	if tracksRemote {
		refs = append([]string{"@{upstream}"}, refs...)
	}
	for _, ref := range refs {
		if _, err := r.line("rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
			continue
		}
		if base, err := r.line("merge-base", "HEAD", ref); err == nil && base != "" {
			return base, nil
		}
	}

	// The commits of HEAD that no branch of the remote has, and the commits
	// where they start. A line that begins with "-" is such a starting point:
	// the remote has it.
	out, err := r.line("rev-list", "--boundary", "HEAD", "--not", "--remotes="+remote)
	if err == nil {
		if out == "" {
			// Nothing is missing on the remote: HEAD itself is there.
			return r.line("rev-parse", "HEAD")
		}
		for _, line := range strings.Split(out, "\n") {
			if base, found := strings.CutPrefix(line, "-"); found {
				return base, nil
			}
		}
	}
	return "", fmt.Errorf("no commit of this branch is known to be on GitHub; push the branch, or run `git fetch %s`", remote)
}

// patchFrom writes the difference between base and the working tree as one
// patch that `git apply` accepts, and lists the files it touches.
func (r repo) patchFrom(base string) ([]byte, []string, error) {
	// A new file is copied into the repository in full before it is compared,
	// so files too large for any patch are refused before git touches them.
	if err := r.refuseLargeNewFiles(); err != nil {
		return nil, nil, err
	}

	scratch, err := os.MkdirTemp("", "sentinel-index-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(scratch)
	index := filepath.Join(scratch, "index")
	// From here on git reads and writes this index instead of the real one.
	r.env = []string{"GIT_INDEX_FILE=" + index}

	// Starting from a copy of the real index keeps what git already knows
	// about each file, so only changed files are read again. Without a real
	// index to copy, the tree of HEAD is the starting point.
	if !r.copyRealIndex(index) {
		if _, err := r.output(0, "read-tree", "HEAD"); err != nil {
			return nil, nil, err
		}
	}
	if _, err := r.output(0, "add", "--all"); err != nil {
		return nil, nil, err
	}
	// The flags pin the format against whatever the user configured: prefixes,
	// external diff programs and text conversions would all yield a patch the
	// server cannot apply.
	patch, err := r.output(MaxPatchBytes,
		"diff", "--cached", "--binary", "--full-index", "-M",
		"--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
		base, "--")
	if errors.Is(err, execx.ErrOutputTooLarge) {
		return nil, nil, fmt.Errorf("%w: more than %d MiB differ from the commit GitHub has; push your branch first", ErrPatchTooLarge, MaxPatchBytes>>20)
	}
	if err != nil {
		return nil, nil, err
	}
	names, err := r.output(0, "diff", "--cached", "--name-only", "-z", "-M", base, "--")
	if err != nil {
		return nil, nil, err
	}
	return []byte(patch), splitNul(names), nil
}

// copyRealIndex copies the index of the repository to dst. It reports false
// when there is none to copy, which is normal right after a clone without
// checkout and harmless otherwise.
func (r repo) copyRealIndex(dst string) bool {
	// Asked without the throwaway index in the environment: this is the one
	// command that must see the real one.
	real, err := repo{ctx: r.ctx, runner: r.runner, dir: r.dir}.line("rev-parse", "--git-path", "index")
	if err != nil || real == "" {
		return false
	}
	if !filepath.IsAbs(real) {
		real = filepath.Join(r.dir, real)
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return false
	}
	return os.WriteFile(dst, data, 0o600) == nil
}

// refuseLargeNewFiles stops when the files git does not track yet would not
// fit in a patch, and names the largest ones.
func (r repo) refuseLargeNewFiles() error {
	out, err := r.output(0, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	type sized struct {
		path string
		size int64
	}
	var files []sized
	var total int64
	for _, path := range splitNul(out) {
		info, err := os.Lstat(filepath.Join(r.dir, filepath.FromSlash(path)))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, sized{path, info.Size()})
		total += info.Size()
	}
	if total <= MaxPatchBytes {
		return nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })
	var largest []string
	for _, f := range files[:min(3, len(files))] {
		largest = append(largest, fmt.Sprintf("%s (%.1f MiB)", f.path, float64(f.size)/(1<<20)))
	}
	return fmt.Errorf("%w: new files add up to %.1f MiB, over the %d MiB limit; the largest are %s. Add them to .gitignore, or commit and push them",
		ErrPatchTooLarge, float64(total)/(1<<20), MaxPatchBytes>>20, strings.Join(largest, ", "))
}

func splitNul(s string) []string {
	var parts []string
	for _, part := range strings.Split(s, "\x00") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

var (
	// git@github.com:owner/name.git
	scpRemote = regexp.MustCompile(`(?i)^(?:[a-z0-9._-]+@)?github\.com:([^/]+)/([^/]+?)(?:\.git)?/?$`)
	// https://github.com/owner/name.git, ssh://git@github.com/owner/name, with
	// optional credentials and port, which are dropped.
	urlRemote = regexp.MustCompile(`(?i)^(?:https|ssh|git)://(?:[^@/]*@)?github\.com(?::\d+)?/([^/]+)/([^/]+?)(?:\.git)?/?$`)
	nameChars = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// ParseGitHubRemote turns the URL of a git remote into "owner/name". The
// error never repeats the URL: it can carry a token.
func ParseGitHubRemote(url string) (string, error) {
	url = strings.TrimSpace(url)
	for _, pattern := range []*regexp.Regexp{scpRemote, urlRemote} {
		m := pattern.FindStringSubmatch(url)
		if m == nil {
			continue
		}
		if owner, name := m[1], m[2]; validName(owner) && validName(name) {
			return owner + "/" + name, nil
		}
	}
	return "", errors.New("its URL is not a repository on github.com, the only host Sentinel opens pull requests on for now")
}

func validName(s string) bool {
	return nameChars.MatchString(s) && s != "." && s != ".."
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
