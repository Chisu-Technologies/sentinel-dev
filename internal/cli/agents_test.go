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

const testExe = "/opt/sentinel/bin/sentinel"

func agents(t *testing.T, runner *execxtest.Fake, home string, args ...string) (result, []agentResult) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"agents", "--json"}, args...), Env{
		Stdout: &stdout, Stderr: &stderr, Runner: runner,
		Build:   buildinfo.Info{Version: "v0.0.0-test"},
		HomeDir: home, Executable: testExe,
	})
	var out []agentResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("not JSON (%v): %s / %s", err, stdout.String(), stderr.String())
	}
	return result{code, stdout.String(), stderr.String()}, out
}

func statusOf(results []agentResult) map[string]string {
	m := map[string]string{}
	for _, r := range results {
		m[r.Agent] = r.Status
	}
	return m
}

func lines(calls []execx.Command) []string {
	var out []string
	for _, c := range calls {
		out = append(out, strings.Join(append([]string{c.Name}, c.Args...), " "))
	}
	return out
}

func TestAgentsAddsSentinelWithEachAgentsOwnCommand(t *testing.T) {
	home := t.TempDir()
	runner := &execxtest.Fake{Results: map[string]execx.Result{
		"claude mcp get sentinel": {ExitCode: 1, Stderr: "No MCP server found"},
	}}

	res, out := agents(t, runner, home)

	if res.code != ExitOK {
		t.Fatalf("exit %d: %s", res.code, res.stderr)
	}
	want := map[string]string{"Claude Code": "added", "Codex": "added", "Gemini CLI": "added", "Cursor": "not_installed"}
	if got := statusOf(out); !equalMaps(got, want) {
		t.Errorf("status = %v, want %v", got, want)
	}
	calls := strings.Join(lines(runner.Calls), "\n")
	for _, wantCall := range []string{
		"claude mcp add --transport stdio --scope user sentinel -- " + testExe + " mcp",
		"codex mcp add sentinel -- " + testExe + " mcp",
		"gemini mcp add --scope user sentinel " + testExe + " mcp",
	} {
		if !strings.Contains(calls, wantCall) {
			t.Errorf("missing call %q in:\n%s", wantCall, calls)
		}
	}
}

func TestAgentsLeavesAnAgentThatAlreadyHasSentinel(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".codex", "config.toml"), "model = \"x\"\n\n[mcp_servers.sentinel]\ncommand = \"sentinel\"\n")
	write(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":{"sentinel":{"command":"sentinel","args":["mcp"]}}}`)
	runner := &execxtest.Fake{Results: map[string]execx.Result{"claude mcp get sentinel": {ExitCode: 0}}}

	_, out := agents(t, runner, home)

	want := map[string]string{"Claude Code": "already", "Codex": "already", "Gemini CLI": "already", "Cursor": "not_installed"}
	if got := statusOf(out); !equalMaps(got, want) {
		t.Errorf("status = %v, want %v", got, want)
	}
	for _, call := range lines(runner.Calls) {
		if strings.Contains(call, " add ") {
			t.Errorf("nothing should be added, but ran %q", call)
		}
	}
}

func TestAgentsDryRunChangesNothing(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	write(t, cursor, `{"mcpServers":{"other":{"command":"x"}}}`)
	runner := &execxtest.Fake{Missing: map[string]bool{"codex": true, "gemini": true},
		Results: map[string]execx.Result{"claude mcp get sentinel": {ExitCode: 1}}}

	_, out := agents(t, runner, home, "--dry-run")

	if got := statusOf(out); got["Claude Code"] != "would_add" || got["Cursor"] != "would_add" || got["Codex"] != "not_installed" {
		t.Errorf("status = %v", got)
	}
	for _, call := range lines(runner.Calls) {
		if strings.Contains(call, " add ") {
			t.Errorf("a dry run ran %q", call)
		}
	}
	if data, _ := os.ReadFile(cursor); string(data) != `{"mcpServers":{"other":{"command":"x"}}}` {
		t.Errorf("a dry run changed Cursor's file: %s", data)
	}
}

func TestAgentsAddsSentinelToCursorKeepingTheRestAndABackup(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	original := `{"mcpServers":{"other":{"command":"x"}},"keep":true}`
	write(t, cursor, original)
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true, "codex": true, "gemini": true}}

	res, out := agents(t, runner, home)

	if res.code != ExitOK || statusOf(out)["Cursor"] != "added" {
		t.Fatalf("exit %d, %v", res.code, out)
	}
	var doc map[string]any
	data, _ := os.ReadFile(cursor)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Cursor's file is not JSON any more: %s", data)
	}
	servers := doc["mcpServers"].(map[string]any)
	sentinel := servers["sentinel"].(map[string]any)
	if servers["other"] == nil || doc["keep"] != true {
		t.Errorf("the other settings were lost: %s", data)
	}
	if sentinel["command"] != testExe || sentinel["type"] != "stdio" {
		t.Errorf("sentinel entry = %v", sentinel)
	}
	if backup, _ := os.ReadFile(cursor + ".before-sentinel"); string(backup) != original {
		t.Errorf("backup = %q, want the original file", backup)
	}
}

func TestAgentsNeverRewritesACursorFileItCannotRead(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	write(t, cursor, "{ // a comment Cursor may accept\n}")
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true, "codex": true, "gemini": true}}

	res, out := agents(t, runner, home)

	if res.code != ExitFailed || statusOf(out)["Cursor"] != "failed" {
		t.Errorf("exit %d, %v", res.code, out)
	}
	if data, _ := os.ReadFile(cursor); !strings.Contains(string(data), "a comment") {
		t.Errorf("the file was rewritten: %s", data)
	}
}

func TestAgentsReportsAnAgentWhoseCommandFails(t *testing.T) {
	runner := &execxtest.Fake{
		Missing: map[string]bool{"codex": true, "gemini": true},
		Results: map[string]execx.Result{
			"claude mcp get sentinel": {ExitCode: 1},
			"claude mcp add --transport stdio --scope user sentinel -- " + testExe + " mcp": {ExitCode: 1, Stderr: "permission denied\nmore"},
		},
	}

	res, out := agents(t, runner, t.TempDir())

	if res.code != ExitFailed || out[0].Status != "failed" || out[0].Detail != "permission denied" {
		t.Errorf("exit %d, %+v", res.code, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
