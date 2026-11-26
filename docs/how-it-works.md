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
- **The supervisor.** One per task. It reads the task before anyone works on it, lists
  what the task requires and forbids, and checks that what the task needs is there. It
  briefs the agent that writes the change, answers that agent and QA when they are
  stuck, and pauses the task with a question for you when only you have what is
  missing. It writes no code and approves nothing.
- **QA.** A second agent that did not write the change. It works on a clean copy, runs
  the tests and checks the change against the task. Without its approval no pull
  request opens.
- **Adversarial QA.** A third agent. It tries to break the change, within what the
  change touched: edge cases, security holes, data leaving the system. Without its
  approval no pull request opens either.
- **PolicyGate.** The rules every action passes through. It denies by default, and
  merging is always denied.

## A run, from task to pull request

1. `sentinel run "<task>"` sends the task with your working tree as a patch on top of
   a commit GitHub already has.
2. The server queues the run. If there is no memory for another sandbox, the run waits
   instead of failing.
3. The supervisor prepares the task. If something only you can give is missing, the run
   pauses here with its question, before any work is paid for.
4. The agent works in its sandbox: the tests first, then the code. It can run commands
   of its own there, such as a build, one test or a formatter; QA can run them too,
   without changing the code. The instructions your repository carries in `AGENTS.md`
   and `CLAUDE.md` are given to it as data from the repository, with a size limit.
5. QA tests the result on a clean copy. A rejection sends the work back to the agent
   with its reason.
6. Adversarial QA tries to break what the change touched. What it finds sends the work
   back the same way.
7. With both passed, the server pushes a branch and opens the pull request. You review
   and merge it yourself.

Each step is saved when it completes. What bounds a run is its spending limit: a run
that reaches it pauses and waits for you, and `sentinel resume` continues it with a
larger limit or another model. A limit of rounds, and of steps per agent, applies only
to a run that has no spending limit or whose supervisor cannot be reached. A run its
supervisor paused shows the question, and `sentinel resume --reply "<answer>"` continues
it from the step where it stopped.
A task that outlasts one stretch of work goes on by itself from what it saved. Its
supervisor looks at each stretch: when one brought nothing new, it pauses the task and
asks you.
`sentinel stop` halts every run on the server, and stays in force after a restart.

## What is open

| Part | Where | License |
| --- | --- | --- |
| `sentinel`, the command | This repository | Apache-2.0 |
| The runtime: API, orchestrator, gateway, sandbox, QA, PolicyGate | Published with the first release | Apache-2.0 |
| Hosted, Teams and Enterprise editions | Not in this repository | Commercial |

Whatever runs on your laptop or on your server is open. What is paid is running it for
you, and what a team or a company needs on top.
