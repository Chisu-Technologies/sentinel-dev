package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// AgentSession is an implementer, QA or reviewer session of a run that waits for the user's
// own coding agent to answer it (Sentinel 7.16, open edition).
type AgentSession struct {
	ID      string `json:"id"`
	RunID   string `json:"run_id"`
	StoryID string `json:"story_id"`
	// Role is implementer, qa, reviewer-a or reviewer-b.
	Role string `json:"role"`
}

// OpenedSession is what a subagent gets when it opens a session: its token,
// the instructions a model would get, and the only tools it may call.
type OpenedSession struct {
	Token        string            `json:"token"`
	Role         string            `json:"role"`
	RunID        string            `json:"run_id"`
	Instructions string            `json:"instructions"`
	Tools        []json.RawMessage `json:"tools"`
}

// CallResult answers a call: "answered" with the result, "running" when it is
// not done yet (ask again with the same call), or "done"/"expired" when the
// session ended and there is nothing more to call.
type CallResult struct {
	Status string          `json:"status"`
	Call   int             `json:"call"`
	Result json.RawMessage `json:"result,omitempty"`
}

// AgentSessions lists the sessions of one run that wait for an agent. A server
// without them (hosted, or older) answers 404, which is an empty list here.
func (c *Client) AgentSessions(ctx context.Context, runID string) ([]AgentSession, error) {
	if !ValidRunID(runID) {
		return nil, errors.New("not a valid run id")
	}
	var out struct {
		Sessions []AgentSession `json:"sessions"`
	}
	err := c.do(ctx, http.MethodGet, "/api/org/agent-sessions?"+url.Values{"run_id": {runID}}.Encode(), nil, http.StatusOK, &out)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil, nil
	}
	return out.Sessions, err
}

// OpenAgentSession takes a session for one subagent, which says what model it is.
func (c *Client) OpenAgentSession(ctx context.Context, id, model string) (OpenedSession, error) {
	if !ValidRunID(id) {
		return OpenedSession{}, errors.New("not a valid session id")
	}
	var out OpenedSession
	err := c.do(ctx, http.MethodPost, "/api/org/agent-sessions/"+id+"/open", map[string]any{"model": model}, http.StatusOK, &out)
	return out, err
}

// CallAgentSession asks for one tool of the session. Sentinel runs it in its
// sandbox; the answer may still be running.
func (c *Client) CallAgentSession(ctx context.Context, id, token, tool string, input map[string]any) (CallResult, error) {
	if !ValidRunID(id) {
		return CallResult{}, errors.New("not a valid session id")
	}
	if input == nil {
		input = map[string]any{}
	}
	var out CallResult
	body := map[string]any{"token": token, "name": tool, "input": input}
	err := c.do(ctx, http.MethodPost, "/api/org/agent-sessions/"+id+"/call", body, http.StatusOK, &out)
	return out, err
}

// AgentSessionResult asks again for the answer to call number `call`.
func (c *Client) AgentSessionResult(ctx context.Context, id, token string, call int) (CallResult, error) {
	if !ValidRunID(id) {
		return CallResult{}, errors.New("not a valid session id")
	}
	var out CallResult
	body := map[string]any{"token": token, "call": call}
	err := c.do(ctx, http.MethodPost, "/api/org/agent-sessions/"+id+"/result", body, http.StatusOK, &out)
	return out, err
}
