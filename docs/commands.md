# Commands

Status: planned design, not implemented.

> **Planned.** None of these commands exists yet. The list may change before the first
> release. Phase numbers show order, not dates; see the [roadmap](roadmap.md). A phase
> starts when the previous one passes its gate.

The Sentinel for developers plugin, called `sentinel`, will have 13 commands and 3
subagents. Each command will be started as `/sentinel:<command>` in Claude Code. For
the flow in order, see [getting started](getting-started.md).

## The 13 commands

| Command | What it will do | Phase |
| --- | --- | --- |
| `/sentinel:login` | Sign in with GitHub in the browser. If your client cannot open the browser for you, the URL and a code appear as text. | 1 |
| `/sentinel:setup` | Check that Docker works, register the repository (name and remote only) and, from Phase 2, build the first Brain index. | 1 (Brain index: 2) |
| `/sentinel:architect <goal>` | Ask the questions needed and save a plan of stages, sprints and tasks. | 1 (without the Brain; the Brain is consulted from 2) |
| `/sentinel:approve [plan]` | Open the plan page so you can approve with your web session. | 1 |
| `/sentinel:run [task] [--all]` | Implement a task, test it and close it with a runner report. With `--all`, continue task by task while the session is open and the plan needs no decision. | 1 (implement, run tests, report); 2 (app, browser and reviewers) |
| `/sentinel:verify` | Test and review a snapshot of your current changes, without a plan. | 1 (tests only); 2 (full check) |
| `/sentinel:retry <task>` | Try a failed task again. | 1 |
| `/sentinel:reopen <task> "<what fails>"` | Start a correction round from what you say is wrong. | 2 |
| `/sentinel:backlog <task>` | Return the task to the backlog. | 1 |
| `/sentinel:plans` | List the plans and switch the active one. | 1 |
| `/sentinel:brain [search]` | Show a summary, search and trace. The full graph is on the web. | 2 |
| `/sentinel:evidence [task or receipt]` | Show and verify the evidence. | 2 |
| `/sentinel:runs` | Show the history of runs. | 2 |

Square brackets mean optional, and angle brackets mean you supply the value.

Some details:

- `/sentinel:approve` will not approve on your behalf. It will open the plan page, and
  you press the approve button there with your web session. The plugin token will have
  no permission to approve.
- `/sentinel:run` will work in a git worktree, on a branch named `sentinel/<task>`,
  without touching your working folder. Results will stay in local branches. You decide
  whether to push and open a pull request.
- When tests run, the runner will run them in Docker on a copy of the last commit of
  that branch, without ignored files.
- A task will not close without a report from the runner.
- `/sentinel:verify` will take a snapshot of the versioned files and of new files that
  are not ignored. It is the intended way to try Sentinel without writing a plan.
- `/sentinel:brain` and the Architect's use of the Brain arrive with Phase 2.

## The 3 subagents

All three arrive with Phase 2. Each one will have a fixed list of allowed tools and
will set its own model. In Claude Code a subagent can only use Claude models, so the
reviewers will use a different Claude model from your session's when your Claude plan
offers one.

| Subagent | What it will do | Tools it will have |
| --- | --- | --- |
| `sentinel-qa` | Test the task on a clean copy with the runner: tests, app and browser. It works with a clean context. | `Read`, `Grep`, `Glob` and the runner and browser tools |
| `sentinel-security` | Review the diff against the OWASP Top 10 and STRIDE. | `Read`, `Grep`, `Glob` |
| `sentinel-code-review` | Review the quality of the diff and its test coverage. | `Read`, `Grep`, `Glob` |

What the QA subagent will not be able to do: edit files, write files, run shell
commands, close tasks or change plans. It will also reject what it could not verify,
and it will not accept a screenshot as proof. The reviewers will have read-only tools.

## What a command cannot guarantee

The commands that change something will be marked so that the model cannot start them
on its own. That only stops your agent from launching the command by itself. The
Sentinel tools remain reachable to the agent. For that reason the hosted service
enforces the rules that matter: a plan is approved only from your web session, and a
task does not close without a runner report. The commands teach the method; they are
not the safeguard.

## Codex

> **Planned (Phase 2).** The same plugin is planned for Codex after the Claude Code
> beta, with caveats listed on the [compatibility page](compatibility.md).

In Codex, skills are started with `$` or `/skills`, not with `/sentinel:`. This page
gives no Codex commands, because loading the plugin in Codex has not been verified.
