# Roadmap

> **Planned.** There are no dates. A phase closes when its gate passes, and every gate
> is something that is run, not a document. The plan can change.

## Phase 0: measure before building

- How much memory the runtime takes on a clean 4 GB server, idle and per task.
- The runtime installed over ssh on a kind of server it has not run on before.
- The sandbox running under gVisor, and what it costs in speed.
- Repository instructions (`AGENTS.md`, `CLAUDE.md`) reach the agent and QA.

**Gate:** the three measurements are published with their numbers.

## Phase 1: one command, your server, a checked pull request

- `sentinel` for Linux, macOS and Windows, with an installer.
- `sentinel setup` on Ubuntu 22.04, Ubuntu 24.04 and Debian 12, with signed releases,
  and `sentinel doctor`.
- Sign-in with GitHub through `gh`; the laptop keeps one revocable token.
- `sentinel run`, `runs`, `watch`, `pull` and `cancel`.
- Pause and resume, stop everything, a spending limit checked before each model call,
  and admission by memory.
- Network access by tiers, and gVisor by default where the server allows it.
- The source of the runtime and its images, public.

**Gate,** on a clean Ubuntu 24.04 server with 4 GB:

1. One command installs everything.
2. `sentinel run` on a test repository opens a pull request with QA passed.
3. The server is restarted in the middle of a run, and the run finishes by itself.
4. A run that reaches its spending limit pauses, and one command resumes it.
5. Three tasks in three repositories open their pull requests with the laptop closed.

## Phase 2: the agent in your terminal

- An interactive session, with plan, edit and auto modes. Auto is not the default.
- Client hooks. A hook that fails blocks the action.
- A catalog of model providers, local models included.
- `sentinel --from-claude-code`.
- Evals with a published baseline, an audit log you can export, and backups.

**Gate:** one eval task passes with three providers, one of them local; a hook stops a
change to a forbidden path; `--from-claude-code` continues a real session; and a
public table shows at least 20 eval tasks.

## Phase 3: cells

- A microVM without a network card where the server has KVM, a warm pool of sandboxes,
  and a pause that keeps the memory of the sandbox.

**Gate:** runs in microVMs with no network card, a pause that keeps the processes
alive, and the measurements published with their failures.

## Phase 4: teams and companies

- Several servers, single sign-on, a central key vault, limits per team and policies
  per user.
- Installation on-premises, also without internet access, and private model endpoints.

**Gate:** a team of three to eight people uses one server for two weeks with single
sign-on, and one installation works without internet access.

Phase 4 belongs to the paid editions and is not built in this repository.
