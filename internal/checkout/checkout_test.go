package checkout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout/checkouttest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// These tests use the real git: the package is a thin layer over it, and what
// matters is that the patch it produces rebuilds the working tree elsewhere.

func build(t *testing.T, dir string) Package {
	t.Helper()
	pkg, err := Build(context.Background(), execx.OS{}, dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return pkg
}

// tree reads every file git would consider part of dir: tracked files that
// still exist, and new files that are not ignored.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	listed := checkouttest.Git(t, dir, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	for _, name := range strings.Split(listed, "\x00") {
		if name == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if errors.Is(err, os.ErrNotExist) {
			continue // deleted in the working tree
		}
		if err != nil {
			t.Fatal(err)
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return files
}

// rebuild does what a server will do: take the base commit from the remote and
// apply the patch on top.
func rebuild(t *testing.T, f checkouttest.Fixture, pkg Package) string {
	t.Helper()
	dst := t.TempDir()
	checkouttest.Git(t, dst, nil, "clone", "-q", f.Remote, ".")
	checkouttest.Git(t, dst, nil, "config", "core.autocrlf", "false")
	checkouttest.Git(t, dst, nil, "checkout", "-q", "--detach", pkg.BaseCommit)
	if len(pkg.Patch) > 0 {
		checkouttest.Git(t, dst, pkg.Patch, "apply", "--index", "--whitespace=nowarn", "-")
	}
	return dst
}

func assertRebuilds(t *testing.T, f checkouttest.Fixture, pkg Package) {
	t.Helper()
	want, got := tree(t, f.Work), tree(t, rebuild(t, f, pkg))
	if len(want) == 0 {
		t.Fatal("the working tree lists no files; the comparison would prove nothing")
	}
	for name, sum := range want {
		if got[name] != sum {
			t.Errorf("%q differs after applying the patch (or is missing)", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%q exists after applying the patch but not in the working tree", name)
		}
	}
}

func TestACleanTreeNeedsNoPatch(t *testing.T) {
	f := checkouttest.New(t)
	pkg := build(t, f.Work)
	if pkg.Repository != "acme/widgets" || pkg.BaseCommit != f.Base {
		t.Errorf("package = %s @ %s, want acme/widgets @ %s", pkg.Repository, pkg.BaseCommit, f.Base)
	}
	if len(pkg.Patch) != 0 || len(pkg.Files) != 0 {
		t.Errorf("a clean tree produced a patch of %d bytes touching %v", len(pkg.Patch), pkg.Files)
	}
}

func TestThePatchRebuildsTheWorkingTree(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, f checkouttest.Fixture)
		files  []string
	}{
		{"a change that is not staged", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "src/app.txt", []byte("one\n2\nthree\n"))
		}, []string{"src/app.txt"}},
		{"a staged change with a later edit on top", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "src/app.txt", []byte("staged\n"))
			checkouttest.Git(t, f.Work, nil, "add", "src/app.txt")
			checkouttest.Write(t, f.Work, "src/app.txt", []byte("staged\nand then edited\n"))
		}, []string{"src/app.txt"}},
		{"a new file, and an ignored one that stays behind", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "src/new.txt", []byte("new\n"))
			checkouttest.Write(t, f.Work, "ignored/secret.txt", []byte("never sent\n"))
			checkouttest.Write(t, f.Work, "debug.log", []byte("never sent\n"))
		}, []string{"src/new.txt"}},
		{"a deleted file", func(t *testing.T, f checkouttest.Fixture) {
			if err := os.Remove(filepath.Join(f.Work, "README.md")); err != nil {
				t.Fatal(err)
			}
		}, []string{"README.md"}},
		{"a renamed file", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Git(t, f.Work, nil, "mv", "src/old name.txt", "src/new name.txt")
		}, []string{"src/new name.txt"}},
		{"a binary file", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "logo.bin", []byte{0, 1, 2, 3, 0, 255, 254, 0, 10, 13})
		}, []string{"logo.bin"}},
		{"a name outside ASCII", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "docs/señal.md", []byte("hola\n"))
		}, []string{"docs/señal.md"}},
		{"commits that were never pushed, with more work on top", func(t *testing.T, f checkouttest.Fixture) {
			checkouttest.Write(t, f.Work, "src/app.txt", []byte("committed locally\n"))
			checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "local only")
			checkouttest.Write(t, f.Work, "src/second.txt", []byte("committed locally too\n"))
			checkouttest.Git(t, f.Work, nil, "add", "-A")
			checkouttest.Git(t, f.Work, nil, "commit", "-q", "-m", "local only, again")
			checkouttest.Write(t, f.Work, "src/third.txt", []byte("not even committed\n"))
		}, []string{"src/app.txt", "src/second.txt", "src/third.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := checkouttest.New(t)
			tc.change(t, f)
			pkg := build(t, f.Work)
			if pkg.BaseCommit != f.Base {
				t.Errorf("base = %s, want the commit the remote has, %s", pkg.BaseCommit, f.Base)
			}
			got := append([]string(nil), pkg.Files...)
			sort.Strings(got)
			if strings.Join(got, "|") != strings.Join(tc.files, "|") {
				t.Errorf("files = %q, want %q", got, tc.files)
			}
			if bytes.Contains(pkg.Patch, []byte("never sent")) {
				t.Error("the patch carries the content of an ignored file")
			}
			assertRebuilds(t, f, pkg)
		})
	}
}

func TestBuildWorksFromASubdirectory(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "README.md", []byte("# widgets, edited from below\n"))
	pkg := build(t, filepath.Join(f.Work, "src"))
	if len(pkg.Files) != 1 || pkg.Files[0] != "README.md" {
		t.Errorf("files = %q, want the change made outside the subdirectory", pkg.Files)
	}
	assertRebuilds(t, f, pkg)
}

func TestRepositoryNamesTheGitHubRepositoryOfTheCheckout(t *testing.T) {
	f := checkouttest.New(t)
	got, err := Repository(context.Background(), execx.OS{}, filepath.Join(f.Work, "src"))
	if err != nil || got != "acme/widgets" {
		t.Errorf("Repository from a subdirectory = %q, %v", got, err)
	}

	// The remote the branch is configured for wins over origin.
	checkouttest.Git(t, f.Work, nil, "remote", "add", "fork", "git@github.com:someone/widgets.git")
	checkouttest.Git(t, f.Work, nil, "config", "branch.main.remote", "fork")
	if got, err := Repository(context.Background(), execx.OS{}, f.Work); err != nil || got != "someone/widgets" {
		t.Errorf("Repository with a configured remote = %q, %v", got, err)
	}
}

func TestRepositoryRefusesWhatIsNotAGitHubCheckout(t *testing.T) {
	if _, err := Repository(context.Background(), execx.OS{}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Errorf("outside a repository: %v", err)
	}
	f := checkouttest.New(t)
	checkouttest.Git(t, f.Work, nil, "remote", "set-url", "origin", "https://oauth2:glpat-NotARealToken@gitlab.com/acme/widgets.git")
	_, err := Repository(context.Background(), execx.OS{}, f.Work)
	if err == nil || !strings.Contains(err.Error(), "not a repository on github.com") || strings.Contains(err.Error(), "glpat") {
		t.Errorf("a remote that is not GitHub: %v", err)
	}
}

func TestBuildLeavesTheRepositoryAsItWas(t *testing.T) {
	f := checkouttest.New(t)
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("staged\n"))
	checkouttest.Git(t, f.Work, nil, "add", "src/app.txt")
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("staged\nthen edited\n"))
	checkouttest.Write(t, f.Work, "src/new.txt", []byte("untracked\n"))

	snapshot := func() string {
		index, err := os.ReadFile(filepath.Join(f.Work, ".git", "index"))
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x %s %s %s", sha256.Sum256(index),
			checkouttest.Git(t, f.Work, nil, "status", "--porcelain=v1"),
			checkouttest.Git(t, f.Work, nil, "rev-parse", "HEAD"),
			checkouttest.Git(t, f.Work, nil, "diff", "--cached", "--stat"))
	}
	before := snapshot()
	pkg := build(t, f.Work)
	if after := snapshot(); after != before {
		t.Errorf("the repository changed:\nbefore: %s\nafter:  %s", before, after)
	}
	if len(pkg.Files) != 2 {
		t.Errorf("files = %q, want the staged file and the new one", pkg.Files)
	}
}

func TestABranchThatTracksALocalBranchFallsBackToTheRemoteDefault(t *testing.T) {
	f := checkouttest.New(t)
	// Local main is one commit ahead of GitHub, and topic tracks local main.
	// Its upstream is therefore a commit GitHub does not have.
	checkouttest.Write(t, f.Work, "README.md", []byte("# widgets, not pushed\n"))
	checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "on local main only")
	checkouttest.Git(t, f.Work, nil, "checkout", "-q", "-b", "topic", "--track", "main")
	checkouttest.Write(t, f.Work, "src/app.txt", []byte("on topic\n"))
	checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "topic work")

	pkg := build(t, f.Work)

	if pkg.BaseCommit != f.Base {
		t.Errorf("base = %s, want %s: a local branch is not proof that GitHub has a commit", pkg.BaseCommit, f.Base)
	}
	assertRebuilds(t, f, pkg)
}

func TestWithoutAnUpstreamOrARemoteDefaultAnyBranchOfTheRemoteServes(t *testing.T) {
	f := checkouttest.New(t)
	// A repository made with `git init` and `git remote add` has neither.
	checkouttest.Git(t, f.Work, nil, "branch", "--unset-upstream")
	checkouttest.Git(t, f.Work, nil, "remote", "set-head", "origin", "--delete")

	t.Run("with nothing new, the base is HEAD", func(t *testing.T) {
		if pkg := build(t, f.Work); pkg.BaseCommit != f.Base || len(pkg.Files) != 0 {
			t.Errorf("package = %s touching %q, want %s and no files", pkg.BaseCommit, pkg.Files, f.Base)
		}
	})
	t.Run("with local commits, the base is where they start", func(t *testing.T) {
		checkouttest.Git(t, f.Work, nil, "checkout", "-q", "-b", "topic")
		checkouttest.Write(t, f.Work, "src/app.txt", []byte("first local commit\n"))
		checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "one")
		checkouttest.Write(t, f.Work, "src/app.txt", []byte("second local commit\n"))
		checkouttest.Git(t, f.Work, nil, "commit", "-q", "-am", "two")
		pkg := build(t, f.Work)
		if pkg.BaseCommit != f.Base {
			t.Errorf("base = %s, want %s", pkg.BaseCommit, f.Base)
		}
		assertRebuilds(t, f, pkg)
	})
}

func TestBuildRefusesWhatItCannotPackage(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		_, err := Build(context.Background(), execx.OS{}, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("no commit yet", func(t *testing.T) {
		dir := t.TempDir()
		checkouttest.Git(t, dir, nil, "init", "-q", "-b", "main")
		_, err := Build(context.Background(), execx.OS{}, dir)
		if err == nil || !strings.Contains(err.Error(), "no commit yet") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("no remote", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Git(t, f.Work, nil, "remote", "remove", "origin")
		_, err := Build(context.Background(), execx.OS{}, f.Work)
		if err == nil || !strings.Contains(err.Error(), `no remote named "origin"`) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a remote that is not GitHub, without repeating its token", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Git(t, f.Work, nil, "remote", "set-url", "origin", "https://oauth2:glpat-NotARealToken@gitlab.com/acme/widgets.git")
		_, err := Build(context.Background(), execx.OS{}, f.Work)
		if err == nil || !strings.Contains(err.Error(), "not a repository on github.com") {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(err.Error(), "glpat") || strings.Contains(err.Error(), "gitlab.com") {
			t.Errorf("the error repeats the remote URL: %v", err)
		}
	})
	t.Run("nothing known to be on GitHub", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Git(t, f.Work, nil, "checkout", "-q", "--orphan", "fresh")
		checkouttest.Git(t, f.Work, nil, "commit", "-q", "--allow-empty", "-m", "unrelated history")
		_, err := Build(context.Background(), execx.OS{}, f.Work)
		if err == nil || !strings.Contains(err.Error(), "no commit of this branch is known to be on GitHub") {
			t.Errorf("err = %v", err)
		}
	})
}

// noise returns n bytes of text that barely compresses, so a diff of it is
// about as large as the text.
func noise(n int) []byte {
	var out bytes.Buffer
	sum := sha256.Sum256([]byte("sentinel"))
	for out.Len() < n {
		sum = sha256.Sum256(sum[:])
		fmt.Fprintf(&out, "%x\n", sum)
	}
	return out.Bytes()[:n]
}

func TestChangesTooLargeForAPatchAreRefused(t *testing.T) {
	t.Run("a large new file is named before git copies it", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Write(t, f.Work, "dump/data.sql", noise(MaxPatchBytes+1024))
		checkouttest.Write(t, f.Work, "small.txt", []byte("small\n"))
		before := checkouttest.Git(t, f.Work, nil, "count-objects", "-v")
		_, err := Build(context.Background(), execx.OS{}, f.Work)
		if !errors.Is(err, ErrPatchTooLarge) {
			t.Fatalf("err = %v, want ErrPatchTooLarge", err)
		}
		if !strings.Contains(err.Error(), "dump/data.sql (3.0 MiB)") {
			t.Errorf("err = %v, want the largest file named with its size", err)
		}
		if after := checkouttest.Git(t, f.Work, nil, "count-objects", "-v"); after != before {
			t.Errorf("git stored objects although the file was refused:\nbefore: %s\nafter:  %s", before, after)
		}
	})
	t.Run("a large change to a tracked file stops at the limit", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Write(t, f.Work, "src/app.txt", noise(MaxPatchBytes+1024))
		_, err := Build(context.Background(), execx.OS{}, f.Work)
		if !errors.Is(err, ErrPatchTooLarge) {
			t.Fatalf("err = %v, want ErrPatchTooLarge", err)
		}
		if !strings.Contains(err.Error(), "push your branch first") {
			t.Errorf("err = %v, want it to say what to do", err)
		}
	})
	t.Run("just under the limit is accepted", func(t *testing.T) {
		f := checkouttest.New(t)
		checkouttest.Write(t, f.Work, "src/big.txt", noise(MaxPatchBytes/2))
		pkg := build(t, f.Work)
		if len(pkg.Patch) <= MaxPatchBytes/2 || len(pkg.Patch) > MaxPatchBytes {
			t.Errorf("patch is %d bytes, want between half the limit and the limit", len(pkg.Patch))
		}
		assertRebuilds(t, f, pkg)
	})
}

func TestParseGitHubRemote(t *testing.T) {
	valid := map[string]string{
		"https://github.com/acme/widgets.git":                   "acme/widgets",
		"https://github.com/acme/widgets":                       "acme/widgets",
		"https://github.com/acme/widgets/":                      "acme/widgets",
		"https://x-access-token:ghs_NotReal@github.com/a/b.git": "a/b",
		"git@github.com:acme/widgets.git":                       "acme/widgets",
		"github.com:acme/widgets":                               "acme/widgets",
		"ssh://git@github.com/acme/widgets.git":                 "acme/widgets",
		"ssh://git@github.com:22/acme/widgets.git":              "acme/widgets",
		"git://github.com/acme/widgets.git":                     "acme/widgets",
		"https://GitHub.com/Acme/Widgets.git":                   "Acme/Widgets",
		"https://github.com/acme/widgets.js.git":                "acme/widgets.js",
	}
	for url, want := range valid {
		got, err := ParseGitHubRemote(url)
		if err != nil || got != want {
			t.Errorf("ParseGitHubRemote(%q) = %q, %v; want %q", url, got, err, want)
		}
	}
	invalid := []string{
		"",
		"/srv/git/widgets.git",
		"https://gitlab.com/acme/widgets.git",
		"https://github.com.evil.example/acme/widgets.git",
		"https://evil.example/github.com/acme/widgets.git",
		"https://github.com/acme",
		"https://github.com/acme/widgets/tree/main",
		"https://github.com/acme/..",
		"https://github.com/ac me/widgets",
		"git@github.com:acme/wid gets.git",
		"https://notgithub.com/acme/widgets",
	}
	for _, url := range invalid {
		if got, err := ParseGitHubRemote(url); err == nil {
			t.Errorf("ParseGitHubRemote(%q) = %q, want an error", url, got)
		}
	}
}
