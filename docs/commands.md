# Commands

> **Planned.** None of these commands exists yet, and the list may change. The phase
> column is order, not a date; see the [roadmap](roadmap.md).

The plugin `sentinel` has 13 commands, started as `/sentinel:<command>` in Claude
Code. `[x]` is optional; `<x>` is a value you supply.

| Command | What it does | Phase |
| --- | --- | --- |
| `/sentinel:login` | Sign in with GitHub in the browser. If the client cannot open it, the URL and a code are printed. | 1 |
| `/sentinel:setup` | Check Docker, register the repository (name and remote only) and build the first Brain index. | 1; Brain index in 2 |
| `/sentinel:architect <goal>` | Ask the questions needed and save a plan of stages, sprints and tasks. | 1; consults the Brain from 2 |
| `/sentinel:approve [plan]` | Open the plan page, where you approve with your web session. | 1 |
| `/sentinel:run [task] [--all]` | Implement a task, test it and close it with a runner report. `--all` continues while the session is open and no decision is needed. | 1; app, browser and reviewers in 2 |
| `/sentinel:verify` | Test and review a snapshot of your current changes, without a plan. | 1 tests only; full check in 2 |
| `/sentinel:retry <task>` | Try a failed task again. | 1 |
| `/sentinel:reopen <task> "<what fails>"` | Start a correction round from what you say is wrong. | 2 |
| `/sentinel:backlog <task>` | Return the task to the backlog. | 1 |
| `/sentinel:plans` | List the plans and switch the active one. | 1 |
| `/sentinel:brain [search]` | Summarize, search and trace the Brain; the full graph is on the web. | 2 |
| `/sentinel:evidence [task or receipt]` | Show and verify the evidence. | 2 |
| `/sentinel:runs` | Show the history of runs. | 2 |

- `/sentinel:run` works in a git worktree on the local branch `sentinel/<task>`. The
  runner tests a copy of that branch's last commit, without ignored files.
- `/sentinel:verify` snapshots the versioned files and new files that are not ignored.
  It is the way to try Sentinel without writing a plan.

## Subagents (Phase 2)

Each has a fixed tool list and sets its own model. In Claude Code a subagent can only
use Claude models, so the reviewers use a different Claude model from your session's
when your plan offers one.

| Subagent | What it does | Tools |
| --- | --- | --- |
| `sentinel-qa` | Tests the task on a clean copy with the runner: tests, app and browser, in a clean context. | `Read`, `Grep`, `Glob`, the runner and browser tools |
| `sentinel-security` | Reviews the diff against the OWASP Top 10 and STRIDE. | `Read`, `Grep`, `Glob` |
| `sentinel-code-review` | Reviews the quality of the diff and its test coverage. | `Read`, `Grep`, `Glob` |

The QA subagent cannot edit or write files, run shell commands, close tasks or change
plans. It rejects what it could not verify and does not accept a screenshot as proof.

## What a command cannot guarantee

Commands that change something are marked so the model cannot start them on its own,
but the Sentinel tools stay within your agent's reach. The hosted service therefore
enforces the rules that matter: a plan is approved only from a web session, and a task
does not close without a runner report.

## Codex

Planned for Phase 2. No Codex commands are listed until loading the plugin in Codex is
verified.
