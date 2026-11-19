package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// agentResult is one line of `sentinel agents`.
type agentResult struct {
	Agent  string `json:"agent"`
	Status string `json:"status"` // added, already, would_add, not_installed, failed
	Detail string `json:"detail,omitempty"`
}

// mcpServerName is the name every agent knows Sentinel by.
const mcpServerName = "sentinel"

// runAgents adds `sentinel mcp` to the coding agents installed on this
// machine. Where an agent has its own command for it (claude, codex, gemini),
// that command does the writing, so its configuration keeps the format the
// agent expects; Cursor and the Claude app have none, so their JSON file is
// edited after a backup. An agent that already has Sentinel is left as it is.
func runAgents(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "say what would be added, and change nothing")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if code := parse(fs, args, env, "agents [--dry-run] [--json]"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel agents takes no arguments")
		return ExitUsage
	}
	exe := env.Executable
	if exe == "" {
		exe = "sentinel"
	}

	results := []agentResult{
		cliAgent(ctx, env, *dry, "Claude Code", "claude",
			func() (bool, error) {
				res, err := env.Runner.Run(ctx, execx.Command{Name: "claude", Args: []string{"mcp", "get", mcpServerName}})
				return err == nil && res.ExitCode == 0, err
			},
			[]string{"mcp", "add", "--transport", "stdio", "--scope", "user", mcpServerName, "--", exe, "mcp"}),
		cliAgent(ctx, env, *dry, "Codex", "codex",
			func() (bool, error) {
				return fileMentions(env, ".codex/config.toml", "[mcp_servers."+mcpServerName+"]")
			},
			[]string{"mcp", "add", mcpServerName, "--", exe, "mcp"}),
		cliAgent(ctx, env, *dry, "Gemini CLI", "gemini",
			func() (bool, error) { return jsonHasServer(env, ".gemini/settings.json") },
			[]string{"mcp", "add", "--scope", "user", mcpServerName, exe, "mcp"}),
		cursorAgent(env, *dry, exe),
		claudeAppAgent(env, *dry, exe),
		claudeCodeLimitAgent(env, *dry, exe),
	}

	failed := false
	for _, r := range results {
		failed = failed || r.Status == "failed"
	}
	code := ExitOK
	if failed {
		code = ExitFailed
	}
	if *asJSON {
		if c := writeJSON(env, results); c != ExitOK {
			return c
		}
		return code
	}
	labels := map[string]string{
		"added": "added", "already": "already has Sentinel", "would_add": "would be added",
		"not_installed": "not installed", "failed": "failed",
	}
	for _, r := range results {
		line := fmt.Sprintf("%-12s %s", r.Agent, labels[r.Status])
		if r.Detail != "" {
			line += ": " + r.Detail
		}
		fmt.Fprintln(env.Stdout, line)
	}
	switch {
	case *dry:
		fmt.Fprintln(env.Stdout, "\nNothing was changed: this was a dry run.")
	default:
		fmt.Fprintln(env.Stdout, "\nRestart an agent that is open so it loads Sentinel.")
	}
	return code
}

// cliAgent adds Sentinel with the agent's own command.
func cliAgent(ctx context.Context, env Env, dry bool, label, program string, has func() (bool, error), addArgs []string) agentResult {
	if _, err := env.Runner.LookPath(program); err != nil {
		return agentResult{Agent: label, Status: "not_installed"}
	}
	present, err := has()
	if err != nil {
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	}
	if present {
		return agentResult{Agent: label, Status: "already"}
	}
	if dry {
		return agentResult{Agent: label, Status: "would_add", Detail: program + " " + strings.Join(addArgs, " ")}
	}
	res, err := env.Runner.Run(ctx, execx.Command{Name: program, Args: addArgs})
	if err != nil {
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	}
	if res.ExitCode != 0 {
		return agentResult{Agent: label, Status: "failed", Detail: firstLine(res.Stderr, res.Stdout)}
	}
	return agentResult{Agent: label, Status: "added"}
}

// cursorAgent edits ~/.cursor/mcp.json, the global file Cursor reads; Cursor
// has no command to add a server.
func cursorAgent(env Env, dry bool, exe string) agentResult {
	const label = "Cursor"
	if env.HomeDir == "" {
		return agentResult{Agent: label, Status: "not_installed"}
	}
	dir := filepath.Join(env.HomeDir, ".cursor")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return agentResult{Agent: label, Status: "not_installed"}
	}
	return editJSONSettings(label, filepath.Join(dir, "mcp.json"), dry, func(doc map[string]any) bool {
		servers, _ := doc["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		if _, ok := servers[mcpServerName]; ok {
			return false
		}
		servers[mcpServerName] = map[string]any{"type": "stdio", "command": exe, "args": []string{"mcp"}}
		doc["mcpServers"] = servers
		return true
	})
}

// claudeAppAgent adds Sentinel to the Claude desktop app, which reads local MCP
// servers from claude_desktop_config.json and has no command to add one. The
// app installed from the Microsoft Store keeps its data, that file included,
// in %LOCALAPPDATA%\Claude-Data\<package>; the other Windows installer uses
// %APPDATA%\Claude, and macOS ~/Library/Application Support/Claude. The first
// of those folders that exists is the app's.
func claudeAppAgent(env Env, dry bool, exe string) agentResult {
	const label = "Claude app"
	path := claudeAppConfig(env)
	if path == "" {
		return agentResult{Agent: label, Status: "not_installed"}
	}
	return editJSONSettings(label, path, dry, func(doc map[string]any) bool {
		servers, _ := doc["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		if _, ok := servers[mcpServerName]; ok {
			return false
		}
		servers[mcpServerName] = map[string]any{"command": exe, "args": []string{"mcp"}}
		doc["mcpServers"] = servers
		return true
	})
}

func claudeAppConfig(env Env) string {
	const file = "claude_desktop_config.json"
	isDir := func(p string) bool {
		info, err := os.Stat(p)
		return err == nil && info.IsDir()
	}
	if local := env.getenv("LOCALAPPDATA"); local != "" {
		matches, _ := filepath.Glob(filepath.Join(local, "Claude-Data", "*", file))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	if roaming := env.getenv("APPDATA"); roaming != "" && isDir(filepath.Join(roaming, "Claude")) {
		return filepath.Join(roaming, "Claude", file)
	}
	if env.HomeDir != "" {
		mac := filepath.Join(env.HomeDir, "Library", "Application Support", "Claude")
		if isDir(mac) {
			return filepath.Join(mac, file)
		}
	}
	return ""
}

// editJSONSettings changes an agent's JSON settings file, which the agent has
// no command for. add changes the document and says whether it did; false
// means Sentinel is already there. The file is copied aside before it is
// rewritten, and a file that is not plain JSON is never rewritten: the
// person's settings stay.
func editJSONSettings(label, path string, dry bool, add func(doc map[string]any) bool) agentResult {
	doc := map[string]any{}
	original, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		original = nil
	case err != nil:
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	case len(bytes.TrimSpace(original)) > 0:
		if err := json.Unmarshal(original, &doc); err != nil {
			return agentResult{Agent: label, Status: "failed", Detail: path + " is not JSON; add Sentinel to it by hand"}
		}
	}
	if !add(doc) {
		return agentResult{Agent: label, Status: "already"}
	}
	if dry {
		return agentResult{Agent: label, Status: "would_add", Detail: "edit " + path}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	}
	mode := fs.FileMode(0o644)
	if original != nil {
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
		backup := path + ".before-sentinel"
		if err := os.WriteFile(backup, original, mode); err != nil {
			return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
		}
	}
	if err := os.WriteFile(path, append(data, '\n'), mode); err != nil {
		return agentResult{Agent: label, Status: "failed", Detail: err.Error()}
	}
	detail := ""
	if original != nil {
		detail = "the previous file is " + path + ".before-sentinel"
	}
	return agentResult{Agent: label, Status: "added", Detail: detail}
}

// fileMentions reports whether a file under the home folder holds a line.
func fileMentions(env Env, rel, line string) (bool, error) {
	if env.HomeDir == "" {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(env.HomeDir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return true, nil
		}
	}
	return false, nil
}

// jsonHasServer reports whether a JSON settings file under the home folder
// already names Sentinel in mcpServers.
func jsonHasServer(env Env, rel string) (bool, error) {
	if env.HomeDir == "" {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(env.HomeDir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return false, nil
	}
	_, ok := doc.MCPServers[mcpServerName]
	return ok, nil
}

func firstLine(texts ...string) string {
	for _, t := range texts {
		if line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(t), "\n", 2)[0]); line != "" {
			return line
		}
	}
	return "it ended with an error"
}
