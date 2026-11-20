package cli

import (
	"context"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/mcp"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// sessionCallWait bounds how long one sentinel_session_call waits for a tool
// that is still running (a test suite can take minutes) before it answers
// "running" and lets the subagent ask again. Each request to the server waits
// on its own; this only adds them up. A variable so tests can shorten it.
var sessionCallWait = 4 * time.Minute

func mcpSessionOpen(ctx context.Context, env Env, id, model string) mcp.Result {
	client, failed := mcpClient(env)
	if failed != nil {
		return *failed
	}
	opened, err := client.OpenAgentSession(ctx, id, model)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	return mcpJSON(opened)
}

func mcpSessionCall(ctx context.Context, env Env, id, token, tool string, input map[string]any, call int) mcp.Result {
	if (tool == "") == (call == 0) {
		return mcp.Result{Text: "give either the tool to call, or the number of a call that said running", IsError: true}
	}
	client, failed := mcpClient(env)
	if failed != nil {
		return *failed
	}
	var answer server.CallResult
	var err error
	if tool != "" {
		answer, err = client.CallAgentSession(ctx, id, token, tool, input)
	} else {
		answer, err = client.AgentSessionResult(ctx, id, token, call)
	}
	deadline := time.Now().Add(sessionCallWait)
	for err == nil && answer.Status == "running" && time.Now().Before(deadline) {
		answer, err = client.AgentSessionResult(ctx, id, token, answer.Call)
	}
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	return mcpJSON(answer)
}
