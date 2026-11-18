package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func serve(t *testing.T, tools []Tool, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, Info{Name: "t", Version: "1"}, tools); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var answers []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("not one JSON message per line: %q", line)
		}
		answers = append(answers, msg)
	}
	return answers
}

func TestAnUnknownProtocolVersionIsAnsweredWithTheLatest(t *testing.T) {
	got := serve(t, nil, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := got[0]["result"].(map[string]any)["protocolVersion"]; v != LatestProtocolVersion {
		t.Errorf("protocolVersion = %v, want %s", v, LatestProtocolVersion)
	}
}

func TestBrokenInputGetsAParseErrorAndTheServerGoesOn(t *testing.T) {
	got := serve(t, nil, `{not json`, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if len(got) != 2 {
		t.Fatalf("answers = %v", got)
	}
	if code := got[0]["error"].(map[string]any)["code"].(float64); code != codeParse {
		t.Errorf("code = %v, want %d", code, codeParse)
	}
	if got[1]["id"].(float64) != 7 || got[1]["error"] != nil {
		t.Errorf("ping after the bad line = %v", got[1])
	}
}

func TestToolCallsReachTheToolAndAnUnknownToolIsAnError(t *testing.T) {
	var gotArgs string
	tools := []Tool{{
		Name: "echo", InputSchema: map[string]any{"type": "object"},
		Call: func(_ context.Context, args json.RawMessage) Result {
			gotArgs = string(args)
			return Result{Text: "done"}
		},
	}}
	got := serve(t, tools,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
	)
	if gotArgs != "{}" {
		t.Errorf("missing arguments should arrive as {}, got %q", gotArgs)
	}
	content := got[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	if content["text"] != "done" {
		t.Errorf("result = %v", got[0])
	}
	if code := got[1]["error"].(map[string]any)["code"].(float64); code != codeInvalidParams {
		t.Errorf("unknown tool code = %v, want %d", code, codeInvalidParams)
	}
}

func TestNotificationsAreNeverAnswered(t *testing.T) {
	got := serve(t, nil, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`)
	if len(got) != 0 {
		t.Errorf("a notification got an answer: %v", got)
	}
}
