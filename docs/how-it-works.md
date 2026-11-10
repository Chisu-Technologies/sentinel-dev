# How it works

> **Planned.** This page describes the design; nothing on it exists yet. "Phase N"
> means order, not a date; see the [roadmap](roadmap.md).

Your coding agent does the thinking, your machine does the running, and the hosted
service keeps the record.

```text
  your machine                                         Sentinel
 +--------------------------------------------+     +----------------------+
 |  Your coding agent (Claude Code, later     |     |  Hosted service      |
 |  Codex), in your interactive session       |     |  (closed source)     |
 |      |                                     |     |                      |
 |      | loads                               |     |  plans, task state,  |
 |      v                                     |     |  Brain projection,   |
 |  Plugin `sentinel`  (MIT)                  |     |  runner reports,     |
 |      |                                     |     |  web board           |
 |      | starts, as an MCP server            |     |                      |
 |      v                                     |     |                      |
 |  Local program  (MIT)  ------------------------>  |                      |
 |      |   plan, task status, reports,       |     +----------------------+
 |      |   Brain projection                  |                ^
 |      v                                     |                |
 |  Docker: tests, your app, a browser        |     your web session approves
 +--------------------------------------------+     plans and reads the board
```

- **Plugin `sentinel`**: the `/sentinel:*` commands, three subagents and the
  configuration that starts the local program. The commands teach your agent the
  method; the rules that matter are enforced by the hosted service.
- **Local program**: a public binary the plugin starts as an MCP server. It holds the
  login, the MCP server, the runner, the Brain indexer and a receipt verifier.
- **Hosted service**: stores plans, task state, the Brain projection and runner
  reports, and draws the web board. It needs a free account. On the free plan it runs
  nothing for you: no container and no machine.

The Brain is a map of your repository: which files and symbols exist and how they
relate. It is built on your machine; only a projection is sent.

## Who does the thinking

Your own agent, with your own membership, inside your interactive session. The local
program does not launch headless sessions of your agent, modify its program or touch
its credentials, and usage counts against your own plan. These rules follow
Sentinel's reading of the vendors' terms, which are theirs to set and interpret; read
them yourself, for example
[Anthropic's legal and compliance page](https://code.claude.com/docs/en/legal-and-compliance)
(checked 2026-11-06). Nothing in this repository states that a particular use complies
with them.

Sentinel works only while your session is open.

## A task, from plan to done

1. Your agent, acting as the architect, asks what it needs and saves a plan of stages,
   sprints and tasks, checked against a published schema.
2. You approve the plan on the web with your own session. The plugin token cannot
   approve.
3. Your agent takes the next task with a reservation that expires; if the session
   dies, the task goes back to waiting.
4. It implements the task in a git worktree, on the local branch `sentinel/<task>`.
5. The runner tests a copy of the branch's last commit in Docker and uploads a report.
   The hosted service closes the task only with that report's identifier.
6. A red report starts another attempt, up to a limit of correction rounds; after
   that the task is marked as failed.

The board has four columns: in progress, needs a decision, needs attention and done;
tasks not yet started sit in the backlog. A task marked as human waits in "needs a
decision" until you mark it done on the web. Pushing the branch and opening a pull
request stay your decision.

## Evidence

The runner builds its report on your machine and uploads it with a token that lives
there; the hosted service computes the verdict. That resists casual edits, not a
determined user, so the evidence is **reported**, not attested. Signed receipts for
closed tasks (Phase 2) are labeled reported too.

## What is open

| Part | License |
| --- | --- |
| Plugin: commands, subagents, review rubrics | MIT, in this repository |
| Local program: login, MCP server, runner, Brain indexer, receipt verifier | MIT, in this repository |
| Formats: plan, runner report, Brain graph, evidence | MIT, as published schemas; a published v1 is not edited |
| Hosted service | Closed source |
| Engine of the paid plans, and the Heads | Closed source, not in this repository |

On the paid plans the work is meant to run without you present; their engine is not
part of this project. Because the hosted service is closed, what you can audit is
what the open parts send to it.
