# How Sentinel for developers works

Status: planned design, not implemented. Nothing on this page works yet.

> **Planned (Phase 1 and later).** This page describes the design. The repository is
> pre-alpha and there is nothing to install. "Phase N" means order, not a date; see
> [the roadmap](roadmap.md).

Sentinel for developers is made of three pieces: a plugin, a local program and a
hosted service. Your coding agent does the thinking, your machine does the running,
and the hosted service keeps the record.

## The three pieces

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

- **The plugin** will carry the commands (`/sentinel:*` in Claude Code), three
  subagents and the configuration that starts the local program. The commands teach
  your agent the method. The rules that matter are enforced by the hosted service, not
  by the commands; see [the security model](security-model.md).
- **The local program** will be a public binary that the plugin starts as an MCP
  server. It will have five parts: login, the MCP server, the runner, the Brain
  indexer and a receipt verifier. You will not need to open it yourself.
- **The hosted service** will store your plan, the state of its tasks, the Brain
  projection and the runner reports, and will draw the web board from them. It will
  need a free account, because that is where the plan is stored.

The Brain is Sentinel's map of your repository: which files and symbols exist and how
they relate to each other. The local program will build it on your machine and send
only a projection of it. See [the privacy page](privacy.md) for exactly what is sent.

## Who does the thinking

Your own coding agent, with your own membership. Sentinel will not pay for inference
on the free plan and will not ask for an API key.

Everything will happen inside your interactive session. The local program will not
launch headless sessions of your agent, will not modify the agent's own program, and
will not touch its credentials. Whatever your agent uses will count against your own
plan, not Sentinel's. Sentinel is designed around these rules, based on its reading of
the vendors' published terms. The terms are the vendors' to set and interpret, so read
them yourself: for Claude Code, see
[Anthropic's legal and compliance page](https://code.claude.com/docs/en/legal-and-compliance)
(checked 2026-11-06).

A consequence: Sentinel will work while your session is open. It will not run on its
own when you close it.

## What runs where

| Where | What | Phase |
|---|---|---|
| Your machine, in Docker | Your tests, with the Sentinel runner | 1 |
| Your machine, in Docker | Your app and a real browser, on a private network | 2 |
| Your machine | The Brain index, built by the local program | 2 |
| Your machine | The coding agent and the plugin's commands | 1 |
| The hosted service | Plans, task state, Brain projection, runner reports | 1 |
| The hosted service | The web board and the plan approval page | 1 |

By design, the hosted service will not run anything per user on the free plan: no
container and no machine will be started for you there.

The runner will copy the last commit of your branch, so only versioned files will be in
the copy, and will run your commands inside a hardened container. The details are in
[the security model](security-model.md).

## A task, from plan to done

> **Planned (Phase 1).** Brain-assisted planning arrives in Phase 2.

1. Your agent will act as the architect, following the plugin's instructions, ask you
   the questions it needs and save a plan to the hosted service. A plan has stages,
   sprints and tasks, and it will be checked against a published schema.
2. You will open the plan page in your browser and approve it with your web session.
   The plugin's token will have no permission to approve, so neither your agent nor a
   hostile repository will be able to approve a plan.
3. Your agent will ask for the next task. The hosted service will hand it over with a
   reservation that expires. If your session dies, the reservation lapses and the task
   goes back to waiting.
4. Your agent will implement the task in a git worktree, on a local branch named
   `sentinel/<task>`, without touching your working folder.
5. The runner will run the tests in Docker and upload a runner report. The task will be
   closed only with the identifier of that report: the hosted service will reject the
   closing call without it.
6. If the report is red, your agent will try again, up to a limit of correction rounds.
   After that the task will be marked as failed.

The board will have four columns: in progress, needs a decision, needs attention and
done. Tasks that are not started will sit in the backlog. A task that is marked as human
in the plan will wait in "needs a decision" until you mark it done on the web.

The results will stay in local branches. Pushing them and opening a pull request is your
decision.

## What the evidence is

The runner will upload its own report when it finishes, and the hosted service will
compute the verdict from it. The report will be written on your machine, using a token
that lives on your machine. That resists casual edits, but it does not resist a
determined user.
For that reason the evidence is called **reported**, and Sentinel does not describe it
as attested or tamper-proof. Signed receipts for closed tasks are planned for Phase 2,
and they will be marked as reported too.

## What is open and what is not

| Part | License | Note |
|---|---|---|
| The plugin: commands and subagents | MIT, planned for this repository | Its prompts travel to your session, so there is no attempt to hide them. |
| The local program: login, MCP server, runner, Brain indexer, receipt verifier | MIT, planned for this repository | A public binary per platform that the plugin starts on your machine. |
| The formats: plan, runner report, Brain graph, evidence | MIT, planned as published schemas | Published with a schema, and a published v1 schema is not edited. |
| The hosted service | Closed source | It is a service operated by Sentinel. |
| The engine of the paid plans and the Heads | Closed source, not in this repository | They belong to the paid plans, not to the plugin. |

Because the hosted service is closed, you cannot audit it from here. What you can read
is what the open parts send to it.

## Related pages

- [What leaves your machine](privacy.md)
- [The security model](security-model.md)
- [The planned commands](commands.md)
- [The roadmap](roadmap.md)
