# Security model

> **Design intent.** Nothing here runs yet; each item becomes a fact only with code and
> a test. A full threat model will come with the first release.

## Trust

A plugin runs code with your user privileges; see Anthropic's
[plugin security documentation](https://code.claude.com/docs/en/plugins/security)
(checked 2026-11-06). The local program also runs as your user and starts containers
through Docker, which is why it and the plugin are published under MIT here. The
hosted service is closed source, and this page makes no claim about its internals.

## A hostile repository

A repository can try to steer your agent, for example with instructions in a file or
a hook that answers prompts on its own. The design limits the damage:

- the runner executes only in containers and mounts nothing but the copy of your code;
- only your web session can approve a plan; the plugin token cannot, so a hook cannot
  either;
- no task closes without a runner report.

These rules live in the hosted service, not in the commands, because your agent can
reach the local program's tools directly. Task reservations expire, so a dead session
does not hold a task.

## Runner isolation

Tests from Phase 1; your app and a browser from Phase 2.

| Measure | What it means |
| --- | --- |
| Copy from a git archive | The code is copied from an archive of the branch's last commit, with LF line endings. Ignored files and environment files are not in the copy. |
| `--cap-drop=ALL` | The container drops every Linux capability. |
| `--security-opt=no-new-privileges` | Processes in the container cannot gain privileges. |
| Non-root user | The commands run as a user without root. |
| `--read-only` with tmpfs | The root filesystem is read-only; scratch space is in memory. |
| `--memory` and `--pids-limit` | Memory and process count are limited. |
| `--cpus` | Set only when the kernel supports it; see the limits below. |
| No `--privileged`, no Docker socket | Containers are not privileged, and the socket is not mounted. |
| Images pinned by digest | Every image is pinned by digest. |

- Dependencies come through a local proxy that allows only listed registries (npm and
  PyPI in Phase 1), on a Docker network created with `--internal`.
- Preparing the app (Phase 2) runs with `--network=none`.
- The app and the browser (Phase 2) share a per-run `--internal` network with no route
  to the internet.
- Phase 1 supports Node and Python. An unknown stack fails closed: the run does not
  start.

## Credentials

- The Sentinel token is personal, revocable, limited to the plugin's interface and
  unable to approve plans. It lives in the operating system keychain.
- Login uses a device code: you sign in with GitHub in your browser and confirm the
  code. If your client cannot open the URL, the URL and the code are printed.
- The program does not read, store or forward your Claude or ChatGPT session, and
  containers receive only the test variables you declare for that repository.

## Evidence

The runner builds its report on your machine and uploads it with a token that lives
there, so the evidence is **reported**: it resists casual edits, not a determined user.
In Phase 2 the hosted service will sign a receipt for each closed task, with a
predicate type of its own for "reported", and the local program will include a
verifier that works offline. The signature does not turn reported evidence into
attested evidence.

## Known limits

- Docker is required. Docker Desktop has its own
  [license terms](https://docs.docker.com/subscription/desktop-license/) (checked
  2026-11-06); alternatives compatible with the Docker API are untested.
- On Windows with WSL2, containers get no CPU limit; the memory and process limits
  remain.
- Until the arm64 browser image exists (Phase 2), the browser runs emulated on Apple
  Silicon.
- The Brain engine, `codebase-memory-mcp` (MIT), has no Windows arm64 build in the
  pinned version.
- If your agent's usage runs out in the middle of a plan, the run stops with a
  warning, the reservation expires and the task goes back to waiting.
- If Codex cannot restrict a subagent to a tool list, a Codex QA subagent could edit
  files. Closing a task would still need a runner report.

Report vulnerabilities as described in [SECURITY.md](../SECURITY.md), not in a public
issue.
