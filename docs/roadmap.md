# Roadmap

> **Planned.** There are no dates. A phase starts when the previous one passes its
> gate, and each gate is checked by running something. The plan can change.

## Phase 0: the hosted service

- A free developer account, with sign-up outside checkout and protection against
  abuse.
- Device-code sign-in for the local program, and a personal token that cannot approve
  plans.
- An interface for the plugin to save plans, take tasks, and send runner reports and
  the Brain projection.
- Plans and tasks in the hosted service, a web page to approve a plan, and the board.
  The contracts between the plugin and the service are written before any code.

**Gate:** with a test token, a script saves a plan, a person approves it on the web,
and the script closes a task with a runner report id; the board shows it done.
Approving with the token fails, and so does closing a task without a report id.

## Phase 1: closed beta in Claude Code

- The local program: sign-in, the MCP server, and a test runner for Node and Python.
- The commands login, setup, architect, approve, run, verify, plans, retry and
  backlog, without the Brain; `verify` runs only the tests.
- Plugin prompts written for Claude Code, the marketplace in this repository, and the
  list of invited people.
- How the program reaches your machine, decided and tested on Windows, macOS and
  Linux.

**Gate:** external developers complete a plan in which every task is closed with a
runner report, and every approval in the audit record comes from a web session.

## Phase 2: QA, reviewers and the Brain

- The runner starts your app and a browser on a private network, with amd64 and arm64
  images.
- The QA subagent and the security and code reviewers, each with a restricted tool
  list.
- The full `run` and `verify`, the Brain indexer, the `brain` command, and planning
  that consults the Brain.
- The `evidence`, `runs` and `reopen` commands; signed receipts and their verifier.
- The plugin for Codex, once open points are verified, such as whether Codex loads a
  local MCP server bundled in a plugin.

**Gate:** in a test repository with a planted flaw, an endpoint without
authentication, the QA subagent or the security reviewer finds it and the task does
not close.

## Phase 3: public launch

- Third-party notices completed before any review rubric derived from another project
  is published.
- Submission to Anthropic's plugin directory, with Codex included in the launch.
- A way to move a plan, its Brain and its history to a paid plan.

**Gate:** public sign-up holds up under a surge of registrations without abuse, and
moving a plan to a paid plan works end to end.

## Not planned

Claude chat surfaces, Cowork until it has been tested, ChatGPT chat and Cursor.
