package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/mcp"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// mcpInstructions tell the agent what the tools are for. The agent writes the
// change; Sentinel runs it in its own sandbox, checks it with a QA and
// reviewers that are not the model that wrote it, and signs the result.
const mcpInstructions = `Sentinel verifies changes in its own sandbox and signs the result.
Use sentinel_verify_pull_request after you open a pull request, and sentinel_run_task to hand Sentinel a task together with this working tree.
Both return a run id at once; follow it with sentinel_run_status, which also gives the pull request, the verdict and the tests.
Sentinel never merges: a person does.`

// runMCP serves the Sentinel tools to a coding agent (Claude Code, Codex,
// Cursor and the like) over MCP's stdio transport. Each tool runs the same
// command a person would type, with --json, so the agent gets exactly what a
// script gets and the server is reached with the same sign-in.
func runMCP(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	if code := parse(fs, args, env, "mcp"); code >= 0 {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(env.Stderr, "sentinel mcp takes no arguments: an agent starts it and talks to it over stdin and stdout")
		return ExitUsage
	}
	in := env.Stdin
	if in == nil {
		in = strings.NewReader("")
	}
	info := mcp.Info{Name: "sentinel", Version: env.Build.Version, Instructions: mcpInstructions}
	if err := mcp.Serve(ctx, in, env.Stdout, info, mcpTools(env)); err != nil {
		fmt.Fprintf(env.Stderr, "sentinel mcp: %v\n", err)
		return ExitFailed
	}
	return ExitOK
}

func mcpTools(env Env) []mcp.Tool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	object := func(props map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	base := str("The branch the pull request targets, when it is not the repository's default branch.")
	runID := str("The run id, as sentinel_run_task, sentinel_verify_pull_request or sentinel_list_runs gave it.")

	return []mcp.Tool{
		{
			Name:  "sentinel_verify_pull_request",
			Title: "Verify a pull request",
			Description: "Ask Sentinel to verify a pull request of the repository in this folder: it runs the tests in its own sandbox " +
				"and has a QA and reviewers that are not the model that wrote the change check it. Returns the run id at once.",
			InputSchema: object(map[string]any{
				"pull_request": str("The pull request: its number, or its URL."),
				"base":         base,
			}, "pull_request"),
			Call: func(ctx context.Context, raw json.RawMessage) mcp.Result {
				var a struct {
					PullRequest string `json:"pull_request"`
					Base        string `json:"base"`
				}
				if r, ok := mcpArgs(raw, &a); !ok {
					return r
				}
				argv := []string{"verify", "--json", "--no-wait"}
				if a.Base != "" {
					argv = append(argv, "--base", a.Base)
				}
				return mcpCommand(ctx, env, append(argv, "--", a.PullRequest))
			},
		},
		{
			Name:  "sentinel_run_task",
			Title: "Run a task with this working tree",
			Description: "Send Sentinel a task together with the work in this folder that is not on GitHub yet. Sentinel finishes it in " +
				"its sandbox, verifies it and opens a pull request. Returns the run id at once.",
			InputSchema: object(map[string]any{
				"task": str("What to do, in one or two sentences."),
				"base": base,
			}, "task"),
			Call: func(ctx context.Context, raw json.RawMessage) mcp.Result {
				var a struct {
					Task string `json:"task"`
					Base string `json:"base"`
				}
				if r, ok := mcpArgs(raw, &a); !ok {
					return r
				}
				argv := []string{"run", "--json", "--no-wait"}
				if a.Base != "" {
					argv = append(argv, "--base", a.Base)
				}
				return mcpCommand(ctx, env, append(argv, "--", a.Task))
			},
		},
		{
			Name:        "sentinel_run_status",
			Title:       "Status of a run",
			Description: "Where a run is, and once it ends its pull request, verdict, tests and cost. It does not wait.",
			InputSchema: object(map[string]any{"run_id": runID}, "run_id"),
			ReadOnly:    true,
			Call: func(ctx context.Context, raw json.RawMessage) mcp.Result {
				var a struct {
					RunID string `json:"run_id"`
				}
				if r, ok := mcpArgs(raw, &a); !ok {
					return r
				}
				return mcpRunStatus(ctx, env, a.RunID)
			},
		},
		{
			Name:        "sentinel_list_runs",
			Title:       "List runs",
			Description: "The most recent runs of the Sentinel server, newest first.",
			InputSchema: object(map[string]any{
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxRunsLimit, "description": "How many runs to list."},
			}),
			ReadOnly: true,
			Call: func(ctx context.Context, raw json.RawMessage) mcp.Result {
				var a struct {
					Limit int `json:"limit"`
				}
				if r, ok := mcpArgs(raw, &a); !ok {
					return r
				}
				argv := []string{"runs", "--json"}
				if a.Limit != 0 {
					argv = append(argv, "--limit", strconv.Itoa(a.Limit))
				}
				return mcpCommand(ctx, env, argv)
			},
		},
		{
			Name:        "sentinel_stop_run",
			Title:       "Stop a run",
			Description: "Stop one run that has not ended. A run that already ended is left as it is.",
			InputSchema: object(map[string]any{"run_id": runID}, "run_id"),
			Call: func(ctx context.Context, raw json.RawMessage) mcp.Result {
				var a struct {
					RunID string `json:"run_id"`
				}
				if r, ok := mcpArgs(raw, &a); !ok {
					return r
				}
				return mcpCommand(ctx, env, []string{"cancel", "--", a.RunID})
			},
		},
	}
}

// mcpArgs decodes a tool's arguments, refusing a field the tool does not take
// so a misspelt one is said instead of silently ignored.
func mcpArgs(raw json.RawMessage, into any) (mcp.Result, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return mcp.Result{Text: "the arguments do not fit this tool: " + err.Error(), IsError: true}, false
	}
	return mcp.Result{}, true
}

// mcpCommand runs one sentinel command with its output captured: what it
// prints is the tool's answer, and a failure is returned with what the command
// said about it. Nothing reaches the protocol stream.
func mcpCommand(ctx context.Context, env Env, argv []string) mcp.Result {
	var out, notes bytes.Buffer
	sub := env
	sub.Stdin, sub.Stdout, sub.Stderr = nil, &out, &notes
	code := Run(ctx, argv, sub)
	text, said := strings.TrimSpace(out.String()), strings.TrimSpace(notes.String())
	if code != ExitOK {
		return mcp.Result{Text: strings.TrimSpace(said + "\n" + text), IsError: true}
	}
	if text == "" {
		text = said
	}
	return mcp.Result{Text: text}
}

// storyStatus is one task of a run in sentinel_run_status.
type storyStatus struct {
	Title         string  `json:"title"`
	State         string  `json:"state"`
	Verdict       string  `json:"verdict"`
	PullRequest   *string `json:"pull_request_url"`
	CostUSD       float64 `json:"cost_usd"`
	TestsPassed   *int    `json:"tests_passed,omitempty"`
	TestsTotal    *int    `json:"tests_total,omitempty"`
	BlockedReason *string `json:"blocked_reason,omitempty"`
	BlockedDetail *string `json:"blocked_detail,omitempty"`
}

func mcpRunStatus(ctx context.Context, env Env, id string) mcp.Result {
	if !server.ValidRunID(id) {
		return mcp.Result{Text: "that is not a run id", IsError: true}
	}
	var notes bytes.Buffer
	quiet := env
	quiet.Stdout, quiet.Stderr = &notes, &notes
	client, code := newServerClient(quiet, "mcp")
	if code >= 0 {
		return mcp.Result{Text: strings.TrimSpace(notes.String()), IsError: true}
	}
	run, err := client.Run(ctx, id)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	stories, err := client.Stories(ctx, id)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	clean := func(s *string) *string {
		if s == nil {
			return nil
		}
		c := client.Clean(*s, server.MaxDetail)
		return &c
	}
	count := func(f *float64) *int {
		if f == nil {
			return nil
		}
		n := int(*f)
		return &n
	}
	status := struct {
		Run     runSummary    `json:"run"`
		Stories []storyStatus `json:"stories"`
	}{Run: summarize(client, run), Stories: make([]storyStatus, 0, len(stories))}
	for _, s := range stories {
		item := storyStatus{
			Title:         client.Clean(s.Title, server.MaxDetail),
			State:         client.Clean(s.State, 64),
			Verdict:       client.Clean(s.Verdict(), 64),
			PullRequest:   clean(s.PRURL),
			CostUSD:       s.CostUSD,
			BlockedReason: clean(s.BlockedReason),
			BlockedDetail: clean(s.BlockedDetail),
		}
		if s.Evidence != nil {
			item.TestsPassed, item.TestsTotal = count(s.Evidence.TestsPassed), count(s.Evidence.TestsTotal)
		}
		status.Stories = append(status.Stories, item)
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	return mcp.Result{Text: string(data)}
}
