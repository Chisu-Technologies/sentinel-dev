// Package mcp serves tools over the Model Context Protocol's stdio transport:
// one JSON-RPC 2.0 message per line on stdin and stdout. It covers what a tool
// server needs (initialize, ping, tools/list, tools/call) and nothing else, so
// the command keeps the standard library as its only dependency.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// LatestProtocolVersion is offered when the client asks for a version this
// server does not know; the client then decides whether it can go on.
const LatestProtocolVersion = "2025-06-18"

var knownVersions = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

// maxMessage bounds one line of input, so a client cannot make the server hold
// an unbounded buffer.
const maxMessage = 4 << 20

// Tool is one tool the server offers.
type Tool struct {
	Name        string
	Title       string
	Description string
	// InputSchema is the JSON Schema of the arguments, an object schema.
	InputSchema map[string]any
	// ReadOnly says the tool changes nothing; a client may call it without
	// asking the person.
	ReadOnly bool
	// Call runs the tool. A non-nil error is a protocol error; a failure the
	// model should read is a Result with IsError.
	Call func(ctx context.Context, args json.RawMessage) Result
}

// Result is what a tool call returns: text for the model, and whether it is
// the description of a failure.
type Result struct {
	Text    string
	IsError bool
}

// Info names the server in the initialize answer.
type Info struct {
	Name         string
	Version      string
	Instructions string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve answers requests from in on out until in ends or ctx is cancelled.
// Tool calls run one at a time, in the order they arrive.
func Serve(ctx context.Context, in io.Reader, out io.Writer, info Info, tools []Tool) error {
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	var mu sync.Mutex
	write := func(r response) error {
		r.JSONRPC = "2.0"
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		_, err = out.Write(append(data, '\n'))
		return err
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64<<10), maxMessage)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := write(response{ID: json.RawMessage("null"), Error: &rpcError{codeParse, "the message is not JSON"}}); err != nil {
				return err
			}
			continue
		}
		// A message without an id is a notification: it gets no answer.
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			if err := write(response{ID: req.ID, Error: &rpcError{codeInvalidRequest, "not a JSON-RPC 2.0 request"}}); err != nil {
				return err
			}
			continue
		}
		result, rerr := handle(ctx, req, info, tools, byName)
		if err := write(response{ID: req.ID, Result: result, Error: rerr}); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading the client: %w", err)
	}
	return nil
}

func handle(ctx context.Context, req request, info Info, tools []Tool, byName map[string]Tool) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := LatestProtocolVersion
		if knownVersions[p.ProtocolVersion] {
			version = p.ProtocolVersion
		}
		result := map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": info.Name, "version": info.Version},
		}
		if info.Instructions != "" {
			result["instructions"] = info.Instructions
		}
		return result, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			entry := map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
				"annotations": map[string]any{"readOnlyHint": t.ReadOnly},
			}
			if t.Title != "" {
				entry["title"] = t.Title
			}
			list = append(list, entry)
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "tools/call needs a tool name"}
		}
		tool, ok := byName[p.Name]
		if !ok {
			return nil, &rpcError{codeInvalidParams, "there is no tool named " + p.Name}
		}
		args := p.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage("{}")
		}
		r := tool.Call(ctx, args)
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": r.Text}},
			"isError": r.IsError,
		}, nil
	default:
		return nil, &rpcError{codeMethodNotFound, "this server does not implement " + req.Method}
	}
}
