package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout/checkouttest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

// A transcript as Claude Code writes it: the person's requests among tool
// results, Claude's answers, injected notes, a subagent's turn, a slash
// command and a line that is not JSON.
const transcript = `{"type":"user","message":{"role":"user","content":"make the login test stop flaking"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ASSISTANT-ANSWER"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"TOOL-OUTPUT"}]}}
{"type":"user","isMeta":true,"message":{"role":"user","content":"META-NOTE"}}
{"type":"user","isSidechain":true,"message":{"role":"user","content":"SUBAGENT-PROMPT"}}
{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>"}}
not json at all
{"type":"user","message":{"role":"user","content":"[Request interrupted by user]"}}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"also cover the retry path\nwith a test"}]}}
`

// claudeCodeSession writes a session for dir under home and returns its file.
func claudeCodeSession(t *testing.T, home, dir, id, body string) string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, ".claude", "projects", claudeCodeProjectDir(abs))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, id+".jsonl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClaudeCodeProjectDirMatchesClaudeCodesNaming(t *testing.T) {
	// The folder Claude Code made on Windows for this very repository's sibling.
	got := claudeCodeProjectDir(`C:\Users\Jean\Desktop\Chisu Technologies\Sentinel`)
	if want := "C--Users-Jean-Desktop-Chisu-Technologies-Sentinel"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := claudeCodeProjectDir("/home/ana/código"); got != "-home-ana-c-digo" {
		t.Errorf("a non-ASCII letter: got %q", got)
	}
}

func TestTheTaskTakesOnlyThePersonsRequestsNewestAsTitle(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	claudeCodeSession(t, home, dir, "s1", transcript)

	task, session, err := claudeCodeTask(Env{HomeDir: home}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if session != "s1" {
		t.Errorf("session = %q", session)
	}
	want := "also cover the retry path\n\nThis continues a Claude Code session in this folder. What the person asked there, oldest first:\n" +
		"\n- make the login test stop flaking" +
		"\n- also cover the retry path\n  with a test"
	if task != want {
		t.Errorf("task =\n%s\nwant\n%s", task, want)
	}
	for _, never := range []string{"ASSISTANT-ANSWER", "TOOL-OUTPUT", "META-NOTE", "SUBAGENT-PROMPT", "/clear", "interrupted"} {
		if strings.Contains(task, never) {
			t.Errorf("the task carries %q", never)
		}
	}
}

func TestTheNewestSessionIsTheOneRead(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	old := claudeCodeSession(t, home, dir, "old", `{"type":"user","message":{"role":"user","content":"OLD-TASK"}}`+"\n")
	claudeCodeSession(t, home, dir, "new", `{"type":"user","message":{"role":"user","content":"new task"}}`+"\n")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	task, session, err := claudeCodeTask(Env{HomeDir: home}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if session != "new" || strings.Contains(task, "OLD-TASK") {
		t.Errorf("session = %q, task = %q", session, task)
	}
}

func TestClaudeConfigDirAndItsProjectName(t *testing.T) {
	config, dir := t.TempDir(), t.TempDir()
	folder := filepath.Join(config, "projects", "chosen")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "s.jsonl"), []byte(`{"type":"user","message":{"role":"user","content":"from the chosen folder"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"CLAUDE_CONFIG_DIR": config, "CLAUDE_CODE_PROJECT_DIR_NAME": "chosen"}

	task, _, err := claudeCodeTask(Env{Getenv: func(n string) string { return vars[n] }}, dir)
	if err != nil || !strings.HasPrefix(task, "from the chosen folder\n") {
		t.Fatalf("task = %q, err = %v", task, err)
	}
}

func TestALongPathIsFoundByItsFirst200Characters(t *testing.T) {
	projects := t.TempDir()
	name := strings.Repeat("a", 250)
	if err := os.MkdirAll(filepath.Join(projects, name[:200]+"-k3h9x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeCodeProjectFolder(projects, name); err != nil {
		t.Errorf("one folder with the prefix: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projects, name[:200]+"-other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeCodeProjectFolder(projects, name); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("two folders with the prefix: err = %v", err)
	}
}

func TestNoSessionOrNothingReadableIsRefusedWithWhatToDo(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	if _, _, err := claudeCodeTask(Env{HomeDir: home}, dir); err == nil || !strings.Contains(err.Error(), `sentinel run "<task>"`) {
		t.Errorf("no session: err = %v", err)
	}
	claudeCodeSession(t, home, dir, "s", `{"type":"assistant","message":{"role":"assistant","content":"only Claude"}}`+"\n")
	if _, _, err := claudeCodeTask(Env{HomeDir: home}, dir); err == nil || !strings.Contains(err.Error(), "no request Sentinel can read") {
		t.Errorf("nothing readable: err = %v", err)
	}
	if _, _, err := claudeCodeTask(Env{}, dir); err == nil || !strings.Contains(err.Error(), "Claude Code's folder") {
		t.Errorf("no home: err = %v", err)
	}
}

func TestTheTaskIsBoundedAndKeepsTheNewestRequests(t *testing.T) {
	var requests []string
	for i := 0; i < 40; i++ {
		requests = append(requests, fmt.Sprintf("request-%02d %s", i, strings.Repeat("x", 150)))
	}
	requests = append(requests, "the newest one")
	task := composeClaudeCodeTask(requests)
	if n := len([]rune(task)); n > maxClaudeCodeTask {
		t.Errorf("task has %d characters, more than %d", n, maxClaudeCodeTask)
	}
	if !strings.HasSuffix(task, "- the newest one") {
		t.Error("the newest request was dropped")
	}
	if strings.Contains(task, requests[0]) {
		t.Error("the oldest request was kept over newer ones")
	}

	huge := composeClaudeCodeTask([]string{strings.Repeat("y", 10000)})
	if n := len([]rune(huge)); n > maxClaudeCodeTask || !strings.HasSuffix(huge, "…") {
		t.Errorf("one huge request: %d characters, ends %q", n, huge[len(huge)-8:])
	}
}

func TestRunFromClaudeCodeDryRunShowsTheTakenTask(t *testing.T) {
	f := checkouttest.New(t)
	home := t.TempDir()
	claudeCodeSession(t, home, f.Work, "s9", transcript)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"run", "--from-claude-code", "--dry-run"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: execx.OS{}, Dir: f.Work, HomeDir: home,
		Build: buildinfo.Info{Version: "v0.0.0-test"},
	})
	if code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Task         also cover the retry path\n") {
		t.Errorf("stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Claude Code session s9") {
		t.Errorf("stderr does not name the session: %q", stderr.String())
	}
}

func TestRunFromClaudeCodeTakesNoTaskArgument(t *testing.T) {
	fake := &execxtest.Fake{}
	if got := runIn(t, fake, "", "run", "--from-claude-code", "a task"); got.code != ExitUsage {
		t.Errorf("status = %d, want %d", got.code, ExitUsage)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("a rejected command line still ran %+v", fake.Calls)
	}
}
