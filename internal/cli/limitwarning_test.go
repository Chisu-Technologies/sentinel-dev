package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

func TestTheHookAnswersWithAnAllowedNotificationAndNothingElse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{limitWarningCommand}, Env{
		Stdin:  strings.NewReader(`{"hook_event_name":"StopFailure","error":"rate_limit","transcript_path":"/x.jsonl"}`),
		Stdout: &stdout, Stderr: &stderr, Build: buildinfo.Info{Version: "v0.0.0-test"},
	})
	if code != ExitOK {
		t.Fatalf("status = %d, stderr = %q", code, stderr.String())
	}
	var answer map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &answer); err != nil {
		t.Fatalf("not JSON: %q", stdout.String())
	}
	seq := answer["terminalSequence"]
	// OSC 9 with a BEL terminator is on Claude Code's allowlist; anything else
	// (another key, CSI, a second sequence) would make it drop the field.
	if len(answer) != 1 || !strings.HasPrefix(seq, "\x1b]9;") || !strings.HasSuffix(seq, "\x07") || strings.Count(seq, "\x1b") != 1 {
		t.Errorf("answer = %q", answer)
	}
	if !strings.Contains(seq, "sentinel run --from-claude-code") {
		t.Errorf("the notification does not say what to run: %q", seq)
	}
}

func TestAgentsAddsTheLimitHookKeepingClaudeCodesOtherSettings(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	before := `{"model":"opus","hooks":{"StopFailure":[{"matcher":"server_error","hooks":[{"type":"command","command":"notify-me"}]}],"PreToolUse":[]}}`
	write(t, settings, before)
	runner := &execxtest.Fake{Results: map[string]execx.Result{"claude mcp get sentinel": {ExitCode: 0}}}

	res, out := agents(t, runner, home)

	if res.code != ExitOK || statusOf(out)["Claude Code limit warning"] != "added" {
		t.Fatalf("exit %d, status %v", res.code, statusOf(out))
	}
	var doc struct {
		Model string `json:"model"`
		Hooks struct {
			StopFailure []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type, Command string
				} `json:"hooks"`
			} `json:"StopFailure"`
			PreToolUse []any `json:"PreToolUse"`
		} `json:"hooks"`
	}
	data, _ := os.ReadFile(settings)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc.Hooks.StopFailure
	if doc.Model != "opus" || doc.Hooks.PreToolUse == nil || len(groups) != 2 || groups[0].Matcher != "server_error" {
		t.Fatalf("the other settings were not kept: %s", data)
	}
	if g := groups[1]; g.Matcher != "rate_limit" || len(g.Hooks) != 1 || g.Hooks[0].Type != "command" || g.Hooks[0].Command != testExe+" "+limitWarningCommand {
		t.Errorf("hook = %+v", g)
	}
	if backup, _ := os.ReadFile(settings + ".before-sentinel"); string(backup) != before {
		t.Errorf("backup = %q", backup)
	}
}

func TestTheLimitHookIsNotAddedWithoutClaudeCodeOrOverAFileItCannotRead(t *testing.T) {
	home := t.TempDir()
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true}}
	if _, out := agents(t, runner, home); statusOf(out)["Claude Code limit warning"] != "not_installed" {
		t.Errorf("without claude: %v", statusOf(out))
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Error("a folder for Claude Code was made without Claude Code")
	}

	settings := filepath.Join(home, ".claude", "settings.json")
	write(t, settings, "{ // a comment\n}")
	runner = &execxtest.Fake{Results: map[string]execx.Result{"claude mcp get sentinel": {ExitCode: 0}}}
	if _, out := agents(t, runner, home); statusOf(out)["Claude Code limit warning"] != "failed" {
		t.Errorf("unreadable file: %v", statusOf(out))
	}
	if data, _ := os.ReadFile(settings); string(data) != "{ // a comment\n}" {
		t.Errorf("an unreadable file was rewritten: %q", data)
	}
}

func TestTheHookCommandQuotesAPathWithASpace(t *testing.T) {
	if got := limitHookCommand(`C:\Program Files\Sentinel\sentinel.exe`); got != `"C:\Program Files\Sentinel\sentinel.exe" claude-code-limit` {
		t.Errorf("got %s", got)
	}
}
