# Security model

Status: planned design, not implemented.

> **Design intent.** Each item below becomes a statement of fact only when code and a
> test exist. The repository is pre-alpha and nothing here is running. This page is the
> precursor of a proper threat model, which will be written when there is a release.

This page explains how Sentinel for developers is meant to limit the damage a hostile
repository, a careless agent or a mistake can do. For what data is sent, see
[the privacy page](privacy.md). For the overall design, see
[how it works](how-it-works.md).

## Trust model

A plugin for a coding agent can run code with your user privileges. Anthropic describes
this in its [plugin security documentation](https://code.claude.com/docs/en/plugins/security)
(checked 2026-11-06).
Sentinel's local program is in the same position: it will run on your machine as your
user, and it will start containers through Docker. Installing it will mean trusting
its code, which is why the plugin and the local program will be published under the MIT
license in this repository.

The hosted service is closed source. It cannot be audited from here, and nothing on
this page makes a claim about its internals.

## A hostile repository

A repository can try to manipulate your agent, for example with instructions in a file
or a hook that answers dialogs on its own. Sentinel's tools are designed to limit the
damage in three ways:

- the runner will execute only inside containers, and will mount nothing from your
  system except the copy of your code;
- only your own web session can approve a plan. The plugin's token has no permission to
  approve, so a hook in the repository that answers prompts on its own cannot approve
  one either;
- no task will be closed without a runner report. The hosted service will reject a
  closing call that does not carry the identifier of a report.

### Why the rules live in the hosted service

The commands that change something will be marked so that your agent does not launch
them on its own. That only prevents your agent from starting the command by itself: the
tools of the local program stay within reach of the model. For this reason the rules
that matter will be enforced by the hosted service, not by the commands. Examples:

- a plan is approved only from a web session;
- a task is closed only with a report;
- a task handed out has a reservation that expires, so a dead session does not hold it.

## Runner isolation

> **Design intent (Phase 1 for tests, Phase 2 for the app and the browser).**

The runner will be part of the local program. It will run your tests, and from Phase 2
your app and a browser, in Docker on your machine.

| Measure | What it means |
|---|---|
| Copy from a git archive | The code will be copied from an archive of the last commit of the branch, with LF line endings. Only versioned files will be in the copy; ignored files and environment files will not. |
| `--cap-drop=ALL` | The container drops every Linux capability. |
| `--security-opt=no-new-privileges` | Processes in the container cannot gain privileges. |
| Non-root user | The commands run as a user without root. |
| `--read-only` with tmpfs | The root filesystem is read-only. Scratch space is in memory. |
| `--memory` and `--pids-limit` | Memory and process count are limited. |
| `--cpus` | Set only when the kernel supports it. See the limits below. |
| No `--privileged` | Containers do not run privileged. |
| No Docker socket | The Docker socket is not mounted inside the container. |
| Images pinned by digest | Images are pinned by digest. |

### Network

- Dependencies will be fetched through a local proxy that allows only a list of
  package registries (npm and PyPI in Phase 1), inside a Docker network created with
  `--internal`.
- Preparing the app (Phase 2) will run with `--network=none`.
- The app and the browser (Phase 2) will share a per-run `--internal` network that has
  no route to the internet.

### Stacks and failure

Phase 1 will support Node and Python. A stack the runner does not recognize will fail
closed: the run will not start.

### Reading the results

The tools will return only the end of the output. The full log will stay on your
machine, and the report will carry the filtered tail and a hash. Secret filtering will
be a scanner in the local program and will be best-effort, not a guarantee; see
[the privacy page](privacy.md).

## Credentials

- The Sentinel token will be personal, revocable, and limited to the plugin's
  interface. It will not be able to approve plans.
- It will be stored in the operating system keychain, not in the repository and not in
  a plain file.
- Login will use a device code: the local program asks the hosted service for a code,
  you sign in with GitHub in your browser and confirm the code. If your client cannot
  open the URL for you, the URL and the code will appear as text.
- The program will not read, store or forward your Claude or ChatGPT session, and
  containers will receive no secrets other than test variables you declare for that
  repository.

## Boundaries with the agent vendors

Sentinel is designed around four rules, based on its reading of the vendors' published
terms:

1. It will not launch headless sessions of your agent.
2. It will not modify the agent's program.
3. It will not touch the agent's credentials.
4. Usage will count against your own plan, not against Sentinel's.

The terms belong to the vendors, who set and interpret them, and they may change. Read
them yourself:
[Anthropic's legal and compliance page](https://code.claude.com/docs/en/legal-and-compliance)
(checked 2026-11-06).
Nothing in this repository is a statement that a particular use complies with those
terms.

## Evidence is reported, not attested

The runner will build its report on your machine and upload it with a token that lives
on your machine. The hosted service will compute the verdict and require at least one
command and an image from Sentinel's list. This resists casual edits. It does not
resist a user who is determined to forge a report. This is why Sentinel calls the
evidence **reported**.

> **Planned (Phase 2).** The hosted service will sign a receipt for each closed task,
> with a predicate type of its own for "reported", and the local program will include a
> verifier that works offline. The signature does not turn reported evidence into
> attested evidence.

## Known limits

- Docker is required. Docker Desktop has its own license terms for some
  organizations; read the
  [Docker Desktop license agreement](https://docs.docker.com/subscription/desktop-license/)
  (checked 2026-11-06) before you use it for work. Whether free alternatives
  compatible with the Docker API work is not yet verified.
- On Windows with WSL2, containers have no CPU limit. The memory and process limits
  remain.
- The browser image is planned for amd64 and arm64 in Phase 2. Until the arm64 image
  exists, a browser on an Apple Silicon Mac would run emulated.
- If your agent's plan runs out of usage in the middle of a long plan, the run will
  stop with a warning, the reservation will expire and the task will go back to
  waiting.
- If Codex still cannot restrict a subagent to a list of tools when Codex support is
  built, a Codex quality-assurance subagent could edit files. The hosted service would
  still require the runner report to close a task. Codex support is planned and
  unverified.

## Reporting a vulnerability

Please follow [SECURITY.md](../SECURITY.md). Do not open a public issue.
