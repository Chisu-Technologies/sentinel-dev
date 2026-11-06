# Roadmap

Status: planned design, not implemented.

> **Planned.** Sentinel for developers is pre-alpha and nothing described here is
> available yet. There are no dates on this page. A phase starts when the previous one
> passes its gate, and the phase numbers give order, not time. The plan can
> change.

There are four phases. Each one ends with a gate that is checked by running something,
not by opinion. Terms are explained in the [glossary](glossary.md), and the commands in
[Commands](commands.md).

## Phase 0: the hosted service first

The plugin is of no use without somewhere to keep plans and evidence, so the hosted
service comes first. This phase adds what the plugin will need from it:

- a free account type for developers, with sign-up that does not go through checkout
  and has protection against abuse;
- sign-in for the local program with a device code, and a personal token that cannot
  approve plans;
- an interface for the plugin to save plans, take tasks, send runner reports and send
  the Brain projection;
- plans and tasks stored in the hosted service, a web page where you approve a plan,
  and the board that shows them.

The technical contracts between the plugin and the hosted service are written down
before any code, so that each side can be built and tested separately.

**Gate.** With a test token, a script saves a plan, a person approves it on the web,
and the script closes a task with a runner report id. The task shows as done on the
board. Approving with the token fails, and so does closing a task without a report id.

## Phase 1: closed beta in Claude Code

The first usable version, for invited people using Claude Code only. It brings:

- the local program with sign-in, the MCP server and a test runner for Node and Python
  projects;
- the plugin with the commands to log in, set up a repository, plan, approve, run,
  verify, list plans, retry and send a task back to the backlog. In this phase `setup`
  and the architect step work without the Brain, and `verify` runs only the tests;
- prompts for the plugin, written new for Claude Code;
- the public repository with the marketplace, and the list of invited people.

How the local program reaches your machine will be decided and tested on Windows,
macOS and Linux during this phase. It is not decided yet.

**Gate.** External developers complete a plan in which every task is closed with a
runner report. In the audit record, every approval comes from a web session and none
from a token.

## Phase 2: QA, reviewers and the Brain

The full version of the checks. It brings:

- the runner starting your app and a browser on a private network, with images for
  amd64 and arm64;
- the QA subagent and the security and code reviewers, each with a restricted tool list;
- the complete `run` and `verify`: tests, app, browser and reviewers;
- the local Brain indexer, the command to query it, and the architect step consulting
  the Brain;
- the commands for evidence, run history and reopening a task;
- receipts for reported evidence, signed by the hosted service, and the verifier;
- the same plugin for Codex. Several points about Codex are still to verify, among them
  whether it loads a local MCP server bundled in a plugin.

**Gate.** In a test repository with a flaw planted on purpose, an endpoint without
authentication, the QA subagent or the security reviewer finds it and the task is not
closed.

## Phase 3: public launch

Opening the project beyond the invited group. It brings:

- the third-party notices completed before any review rubric derived from another
  project is published;
- submission of the plugin to Anthropic's plugin directory;
- Codex included in the launch;
- a way to move a plan, its Brain and its history to a paid plan of the hosted service.

**Gate.** Public sign-up holds up under a surge of registrations without abuse, and
moving a plan to a paid plan works from start to finish.

## What is not on the roadmap

These surfaces are not supported and have no phase: the Claude chat surfaces, Cowork
until it has been tested, the ChatGPT chat and Cursor. The
[compatibility page](compatibility.md) has the details.
