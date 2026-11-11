# How it works

> **Planned.** This page describes the design. The commands that exist today are in
> [commands](commands.md). "Phase N" means order, not a date; see the
> [roadmap](roadmap.md).

Your laptop asks, your server works, and GitHub receives a pull request.

```text
 your laptop                 your server
+----------------+   ssh    +----------------------------------------------------+
| sentinel       | -------> | API           runs, events, pause, resume, stop    |
|                |  tunnel  | Orchestrator  saves each step; continues after a   |
| one revocable  |          |               restart                              |
| token          |          | Gateway       holds the model keys and the GitHub  |
+----------------+          |               credential; checks the limit before  |
                            |               each model call                      |
                            | Sandbox       one per task; no keys; network by    |
                            |               allowlist                            |
                            | QA            a separate agent; runs before the    |
                            |               pull request                         |
                            | PolicyGate    denies by default; no merge          |
                            +----------------------------------------------------+
                                                   |
                                                   v
                                        GitHub: a pull request
```

## The parts

- **`sentinel`**, the command on your laptop. It packages the task, sends it through an
  ssh tunnel and shows you the run. It holds one revocable token. Your model keys and
  your GitHub credential are not kept on the laptop.
- **The runtime**, on your server. It is the API, the orchestrator, the gateway, the
  sandboxes and QA. `sentinel setup` installs it from signed releases.
- **The gateway.** The only part that holds keys. A sandbox gets a token that expires,
  valid for its own run. The gateway swaps it for the real key, and refuses a model
  call that would take the run over its spending limit.
- **The sandbox.** One per task. It has the code and no credentials. Its network goes
  through an allowlist. See the [security model](security-model.md).
- **QA.** A second agent that did not write the change. It works on a clean copy, runs
  the tests and reviews the diff. Without its approval no pull request opens.
- **PolicyGate.** The rules every action passes through. It denies by default, and
  merging is always denied.

## A run, from task to pull request

1. `sentinel run "<task>"` sends the task with your working tree as a patch on top of
   a commit GitHub already has.
2. The server queues the run. If there is no memory for another sandbox, the run waits
   instead of failing.
3. The agent works in its sandbox. The instructions your repository carries in
   `AGENTS.md` and `CLAUDE.md` are given to it as data from the repository, with a
   size limit.
4. QA tests the result on a clean copy. A rejection sends the work back to the agent,
   up to a limit of rounds.
5. With QA passed, the server pushes a branch and opens the pull request. You review
   and merge it yourself.

Each step is saved when it completes. A run that reaches its spending limit pauses and
waits for you; `sentinel resume` continues it with a larger limit or another model.
`sentinel stop` halts every run on the server, and stays in force after a restart.

## What is open

| Part | Where | License |
| --- | --- | --- |
| `sentinel`, the command | This repository | Apache-2.0 |
| The runtime: API, orchestrator, gateway, sandbox, QA, PolicyGate | Published with the first release | Apache-2.0 |
| Hosted, Teams and Enterprise editions | Not in this repository | Commercial |

Whatever runs on your laptop or on your server is open. What is paid is running it for
you, and what a team or a company needs on top.
