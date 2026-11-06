# Getting started

Status: planned design, not implemented.

> **Planned.** Nothing on this page works yet. There is no plugin, no local program
> and no installable release. This page describes the flow Sentinel for developers is
> designed to have, so you can judge it before it exists. Every command below is
> labeled, and every one of them may change before the first release.

Sentinel for developers checks the work of your coding agent. For the overall design,
see [how it works](how-it-works.md). For the full command list, see
[commands](commands.md). For supported hosts and systems, see
[compatibility](compatibility.md).

## Before you start

You will need:

- Docker working on your machine. The local program will run your tests, your app and
  a browser in Docker. See [compatibility](compatibility.md) for the Docker notes.
- Claude Code, at a minimum version to be confirmed at release. The sign-in step
  described below needs a recent version.
- A GitHub account. Sign-in to Sentinel goes through GitHub.
- An invitation to the beta. Access is by invitation only for now, and there is no way
  to request one yet.

You will not need an API key. Your own agent does the thinking, under your own
membership, inside your interactive session. Sentinel will not pay for that inference
and will not ask for a key.

## Add the marketplace and install the plugin

The plugin will be called `sentinel`. It will be published from the marketplace in
this repository. The marketplace name `sentinel-dev` is an assumption: it has to match
the `name` field of a `marketplace.json` file that does not exist yet. Check this
step against the real file before you rely on it.

You can run the commands from your shell or inside a Claude Code session.

From your shell:

Planned. This command does not work yet.

```bash
claude plugin marketplace add Chisu-Technologies/sentinel-dev
```

Planned. This command does not work yet.

```bash
claude plugin install sentinel@sentinel-dev
```

Inside a Claude Code session:

Planned. This command does not work yet.

```text
/plugin marketplace add Chisu-Technologies/sentinel-dev
```

Planned. This command does not work yet.

```text
/plugin install sentinel@sentinel-dev
```

In a session, `/plugin install` opens a panel on the plugin's details so you can
review what it adds and choose an install scope before anything is installed. A plugin
can run hooks and MCP servers, and this one will start a local program that launches
containers, so read that panel. See
[plugin security and trust](https://code.claude.com/docs/en/plugins/security)
(checked 2026-11-06).

For reference, these are the official forms of the same commands, with example names,
from [Anthropic's plugin installation guide](https://code.claude.com/docs/en/discover-plugins)
(checked 2026-11-06):

```text
/plugin marketplace add anthropics/claude-plugins-official
/plugin install commit-commands@claude-plugins-official
```

```bash
claude plugin install formatter@your-org --scope project
```

Those three lines belong to other projects and are shown only to explain the syntax.
In `name@marketplace`, the part after the `@` is the name the marketplace registered
under.

## The local program

The plugin will start a local program as an MCP server. That program is what logs you
in, runs your tests in Docker, and builds the Brain index. You will not need to open
it yourself.

How the program reaches your machine is not decided yet. It could arrive with the
plugin or as a separate download, and this page states no mechanism. See
[open decisions](#open-decisions).

## Sign in

Run `/sentinel:login` in Claude Code.

Planned. This command does not work yet.

```text
/sentinel:login
```

It will open your browser so you can sign in with GitHub. That creates your Sentinel
account, or signs you in to the one you already have. Sentinel will keep a personal
token in your operating system keychain, not in your repository or in a plain file.
That token is limited to the plugin and cannot approve plans.

If your Claude Code client does not offer to open the browser for you, the command will
print the sign-in URL and a code as text instead.

## Set up your repository

Run `/sentinel:setup` inside your repository.

Planned. This command does not work yet.

```text
/sentinel:setup
```

It will:

- check that Docker works;
- register the repository with your account. Only the repository name and remote are
  registered, without any user or token the URL may carry, and not the code;
- build the first Brain index. This part arrives with Phase 2. The Brain is Sentinel's
  map of your repository: which files and symbols exist and how they relate to each
  other.

What leaves your machine is listed in the README section
[What will leave your machine, and what will not](../README.md#what-will-leave-your-machine-and-what-will-not).

## A day of work

The sequence below is the design. Each step carries the phase in which it first
becomes available. Phase numbers show order, not dates; see the
[roadmap](roadmap.md).

1. Plan. `/sentinel:architect` with a goal in plain words, for example "login with
   magic link". Your agent acts as the architect using Sentinel's instructions, asks
   the questions it needs, and saves a plan made of stages, sprints and tasks. You get
   a link to the board. (Phase 1, without the Brain. The Brain is consulted from
   Phase 2.)
2. Approve. `/sentinel:approve` opens the plan page in your browser, and you approve
   it there with your own web session. The plugin token cannot approve, so neither
   your agent nor the repository can. (Phase 1)
3. Run. `/sentinel:run` takes the next task, and your agent implements it in a git
   worktree on the branch `sentinel/<task>`, without touching your working folder.
   In Phase 1 the run covers implementing, running the tests in Docker and reporting.
   From Phase 2 it also starts your app, tests it in a browser through a separate QA
   subagent, and has a code reviewer and a security reviewer read the diff.
4. Keep going. `/sentinel:run --all` continues task by task while your session is open
   and the plan does not need a decision from you. (Phase 1)
5. Verify without a plan. `/sentinel:verify` takes a snapshot of your current changes
   and verifies it. In Phase 1 it runs the tests only. The full check arrives with
   Phase 2. This is the intended way to try Sentinel without writing a plan.
6. Correct. `/sentinel:retry <task>` tries a failed task again. `/sentinel:backlog
   <task>` returns it to the backlog. (Phase 1.) `/sentinel:reopen <task> "what is
   wrong"` starts a correction round from your description. (Phase 2)
7. Look things up. `/sentinel:plans` lists plans and switches the active one
   (Phase 1). `/sentinel:brain`, `/sentinel:evidence` and `/sentinel:runs` search the
   Brain, show evidence and list past runs, without touching your code (Phase 2).

All of these commands are described in [commands](commands.md).

## Where results land

Results will stay in local branches named `sentinel/<task>`. Pushing a branch and
opening a pull request is your decision.

## What you see on the web

The same account will give you a web board with the plan and the status of each task.
A task will not close without a report from the runner on your machine. Evidence will
be shown as reported: your machine reports it to the hosted service, so it resists
casual edits but not a determined user. It is not attested.

## Open decisions

- Delivery of the local program. How it reaches your machine is not decided.
- The marketplace name. `sentinel-dev` is an assumption until the marketplace file
  exists.
- The minimum Claude Code version. It is a design figure, to be confirmed at release.

## Where to ask

Open a [question](https://github.com/Chisu-Technologies/sentinel-dev/issues/new/choose)
in this repository's issues. Do not put credentials or private code in an issue.
