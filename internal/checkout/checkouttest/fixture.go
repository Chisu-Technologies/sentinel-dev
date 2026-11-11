// Package checkouttest builds real git repositories for tests: a working tree
// whose origin stands in for GitHub.
package checkouttest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs the real git in dir and returns its trimmed output. It fails the
// test when git fails.
func Git(t testing.TB, dir string, stdin []byte, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		// A fixed identity and no conversions, so the repository is the same
		// on every machine that runs the tests.
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false",
	}, args...)...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// Write creates or replaces a file under dir. name uses forward slashes.
func Write(t testing.TB, dir, name string, content []byte) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Fixture is a working tree and the remote it was pushed to.
type Fixture struct {
	// Remote is a bare repository standing in for GitHub.
	Remote string
	// Work is the working tree of the user, on branch main.
	Work string
	// Base is the one commit both have.
	Base string
}

// New makes a repository whose origin has one commit and whose origin URL
// reads as github.com/acme/widgets. Nothing connects to that URL: the code
// under test reads what the last push recorded.
func New(t testing.TB) Fixture {
	t.Helper()
	remote := t.TempDir()
	Git(t, remote, nil, "init", "-q", "--bare", "-b", "main")
	work := t.TempDir()
	Git(t, work, nil, "init", "-q", "-b", "main")
	Git(t, work, nil, "config", "core.autocrlf", "false")
	Write(t, work, "README.md", []byte("# widgets\n"))
	Write(t, work, "src/app.txt", []byte("one\ntwo\nthree\n"))
	Write(t, work, "src/old name.txt", []byte(strings.Repeat("a line that stays the same\n", 20)))
	Write(t, work, ".gitignore", []byte("ignored/\n*.log\n"))
	Git(t, work, nil, "add", "-A")
	Git(t, work, nil, "commit", "-q", "-m", "base")
	Git(t, work, nil, "remote", "add", "origin", remote)
	Git(t, work, nil, "push", "-q", "-u", "origin", "main")
	Git(t, work, nil, "remote", "set-head", "origin", "main")
	Git(t, work, nil, "remote", "set-url", "origin", "https://github.com/acme/widgets.git")
	return Fixture{Remote: remote, Work: work, Base: Git(t, work, nil, "rev-parse", "HEAD")}
}
