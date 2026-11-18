package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// limitWarningCommand is the subcommand Claude Code's hook runs. It is not in
// the help: only the hook calls it.
const limitWarningCommand = "claude-code-limit"

// limitWarningText is the desktop notification. Claude Code shows nothing a
// StopFailure hook prints except a terminal sequence, and allows OSC 9, which
// Windows Terminal, iTerm2, ConEmu and WezTerm turn into a notification
// (https://code.claude.com/docs/en/hooks.md, "Emit terminal notifications").
const limitWarningText = "Claude Code reached its limit. sentinel run --from-claude-code hands the work in this folder to Sentinel."

// runLimitWarning answers Claude Code's StopFailure hook with the
// notification. What the hook sends on stdin is read and not kept.
func runLimitWarning(args []string, env Env) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "sentinel "+limitWarningCommand+" takes no arguments: Claude Code's hook runs it")
		return ExitUsage
	}
	if env.Stdin != nil {
		_, _ = io.Copy(io.Discard, env.Stdin)
	}
	return writeJSON(env, map[string]string{"terminalSequence": "\x1b]9;" + limitWarningText + "\x07"})
}

// claudeCodeLimitAgent adds to Claude Code's user settings a StopFailure hook
// for "rate_limit" that runs the subcommand above. Claude Code has no command
// to add a hook, so the file is edited like Cursor's.
func claudeCodeLimitAgent(env Env, dry bool, exe string) agentResult {
	const label = "Claude Code limit warning"
	if _, err := env.Runner.LookPath("claude"); err != nil {
		return agentResult{Agent: label, Status: "not_installed"}
	}
	config := env.getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		if env.HomeDir == "" {
			return agentResult{Agent: label, Status: "not_installed"}
		}
		config = filepath.Join(env.HomeDir, ".claude")
	}
	command := limitHookCommand(exe)
	return editJSONSettings(label, filepath.Join(config, "settings.json"), dry, func(doc map[string]any) bool {
		hooks, _ := doc["hooks"].(map[string]any)
		if hooks == nil {
			hooks = map[string]any{}
		}
		groups, _ := hooks["StopFailure"].([]any)
		if hasLimitHook(groups) {
			return false
		}
		hooks["StopFailure"] = append(groups, map[string]any{
			"matcher": "rate_limit",
			"hooks":   []any{map[string]any{"type": "command", "command": command}},
		})
		doc["hooks"] = hooks
		return true
	})
}

// limitHookCommand is the hook's command line. Claude Code runs it in a
// shell, so a path with a space is quoted.
func limitHookCommand(exe string) string {
	if strings.ContainsAny(exe, " \t") {
		exe = `"` + exe + `"`
	}
	return exe + " " + limitWarningCommand
}

// hasLimitHook reports whether a StopFailure hook already runs the warning,
// whichever binary path it was written with.
func hasLimitHook(groups []any) bool {
	for _, g := range groups {
		group, _ := g.(map[string]any)
		inner, _ := group["hooks"].([]any)
		for _, h := range inner {
			hook, _ := h.(map[string]any)
			if command, _ := hook["command"].(string); strings.HasSuffix(strings.TrimSpace(command), " "+limitWarningCommand) {
				return true
			}
		}
	}
	return false
}
