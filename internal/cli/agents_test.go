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
	return agentsWithEnv(t, runner, home, nil, args...)
}

// agentsWithEnv is agents with environment variables, for the Claude app,
// whose folder Windows names in LOCALAPPDATA or APPDATA.
func agentsWithEnv(t *testing.T, runner *execxtest.Fake, home string, vars map[string]string, args ...string) (result, []agentResult) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"agents", "--json"}, args...), Env{
		Stdout: &stdout, Stderr: &stderr, Runner: runner,
		Build:   buildinfo.Info{Version: "v0.0.0-test"},
		HomeDir: home, Executable: testExe,
		Getenv: func(name string) string { return vars[name] },
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
	want := map[string]string{"Claude Code": "added", "Codex": "added", "Gemini CLI": "added", "Cursor": "not_installed", "Claude app": "not_installed", "Claude Code limit warning": "added"}
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
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"hooks":{"StopFailure":[{"matcher":"rate_limit","hooks":[{"type":"command","command":"/old/path/sentinel claude-code-limit"}]}]}}`)
	runner := &execxtest.Fake{Results: map[string]execx.Result{"claude mcp get sentinel": {ExitCode: 0}}}

	_, out := agents(t, runner, home)

	want := map[string]string{"Claude Code": "already", "Codex": "already", "Gemini CLI": "already", "Cursor": "not_installed", "Claude app": "not_installed", "Claude Code limit warning": "already"}
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

// Measured on 2026-11-19 on a Windows 11 PC with the Codex desktop app and no
// codex command: ~/.codex/config.toml held the app's own MCP server, written
// with literal strings for its Windows paths.
func TestAgentsAddsSentinelToTheCodexAppWithoutItsCommand(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".codex", "config.toml")
	original := "model = \"gpt-5\"\n\n[mcp_servers.node_repl]\ncommand = 'C:\\Codex\\node_repl.exe'\n"
	write(t, config, original)
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true, "codex": true, "gemini": true}}

	res, out := agents(t, runner, home)

	if res.code != ExitOK || statusOf(out)["Codex"] != "added" {
		t.Fatalf("exit %d, %v", res.code, out)
	}
	data, _ := os.ReadFile(config)
	want := original + "\n[mcp_servers.sentinel]\ncommand = '" + testExe + "'\nargs = [\"mcp\"]\n"
	if string(data) != want {
		t.Errorf("config.toml =\n%s\nwant\n%s", data, want)
	}
	if backup, _ := os.ReadFile(config + ".before-sentinel"); string(backup) != original {
		t.Errorf("backup = %q, want the original file", backup)
	}
	if _, again := agents(t, runner, home); statusOf(again)["Codex"] != "already" {
		t.Errorf("second run: %v", again)
	}
	for _, call := range lines(runner.Calls) {
		if strings.HasPrefix(call, "codex ") {
			t.Errorf("there is no codex command, but ran %q", call)
		}
	}
}

func TestTomlStringKeepsAWindowsPathAndEscapesWhatALiteralCannotHold(t *testing.T) {
	if got := tomlString(`C:\Users\a b\sentinel.exe`); got != `'C:\Users\a b\sentinel.exe'` {
		t.Errorf("literal = %s", got)
	}
	if got := tomlString(`C:\it's\sentinel.exe`); got != `"C:\\it's\\sentinel.exe"` {
		t.Errorf("basic = %s", got)
	}
}

// Measured on 2026-11-19 on a Windows 11 PC with the Claude app from the
// Microsoft Store: there is no %APPDATA%\Claude, and the app keeps its own
// settings in %LOCALAPPDATA%\Claude-Data\<package>\claude_desktop_config.json.
func TestAgentsAddsSentinelToTheClaudeAppFromTheStoreKeepingItsSettings(t *testing.T) {
	home, local := t.TempDir(), t.TempDir()
	config := filepath.Join(local, "Claude-Data", "claude_pzs8sxrjxfjjc", "claude_desktop_config.json")
	original := `{"preferences":{"theme":"dark"},"coworkUserFilesPath":"C:/x"}`
	write(t, config, original)
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true, "codex": true, "gemini": true}}

	res, out := agentsWithEnv(t, runner, home, map[string]string{"LOCALAPPDATA": local})

	if res.code != ExitOK || statusOf(out)["Claude app"] != "added" {
		t.Fatalf("exit %d, %v", res.code, out)
	}
	var doc map[string]any
	data, _ := os.ReadFile(config)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("the app's file is not JSON any more: %s", data)
	}
	if doc["preferences"] == nil || doc["coworkUserFilesPath"] != "C:/x" {
		t.Errorf("the app's settings were lost: %s", data)
	}
	sentinel := doc["mcpServers"].(map[string]any)["sentinel"].(map[string]any)
	if sentinel["command"] != testExe || sentinel["args"].([]any)[0] != "mcp" {
		t.Errorf("sentinel entry = %v", sentinel)
	}
	if backup, _ := os.ReadFile(config + ".before-sentinel"); string(backup) != original {
		t.Errorf("backup = %q, want the original file", backup)
	}

	// A second run leaves it as it is.
	_, again := agentsWithEnv(t, runner, home, map[string]string{"LOCALAPPDATA": local})
	if statusOf(again)["Claude app"] != "already" {
		t.Errorf("second run: %v", again)
	}
}

func TestAgentsFindsTheClaudeAppOfTheOtherWindowsInstallerAndOfMacOS(t *testing.T) {
	runner := &execxtest.Fake{Missing: map[string]bool{"claude": true, "codex": true, "gemini": true}}

	roaming := t.TempDir()
	if err := os.MkdirAll(filepath.Join(roaming, "Claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, out := agentsWithEnv(t, runner, t.TempDir(), map[string]string{"APPDATA": roaming})
	if statusOf(out)["Claude app"] != "added" {
		t.Errorf("APPDATA install: %v", out)
	}
	if _, err := os.Stat(filepath.Join(roaming, "Claude", "claude_desktop_config.json")); err != nil {
		t.Errorf("no config written under APPDATA: %v", err)
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "Application Support", "Claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, mac := agents(t, runner, home)
	if statusOf(mac)["Claude app"] != "added" {
		t.Errorf("macOS install: %v", mac)
	}
}

// A connected agent is useless if the person cannot tell how to use it: the
// text output names the agents that have Sentinel, what to ask, and the tools.
func TestAgentsSaysWhatToAskTheConnectedAgents(t *testing.T) {
	home := t.TempDir()
	runner := &execxtest.Fake{Results: map[string]execx.Result{
		"claude mcp get sentinel": {ExitCode: 1, Stderr: "No MCP server found"},
	}}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"agents"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: runner,
		Build: buildinfo.Info{Version: "v0.0.0-test"}, HomeDir: home, Executable: testExe,
		Getenv: func(string) string { return "" },
	})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Ready in Claude Code, Codex, Gemini CLI.",
		`"Send this change to Sentinel"`,
		"sentinel_run_task", "sentinel_verify_pull_request", "sentinel_run_status",
		"/mcp lists the sentinel server",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Ready in Claude Code limit warning") || strings.Contains(out, ", Claude Code limit warning") {
		t.Errorf("the hook was named as an agent:\n%s", out)
	}
}
