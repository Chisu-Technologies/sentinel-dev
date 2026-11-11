# Security model

> **Design intent.** The runtime is not released. Each item here becomes a fact only
> with code and a test, and a full threat model will come with the first release. The
> last section says what exists today.

## What Sentinel assumes

- The code in a repository, and the instructions it carries, may be hostile. An agent
  that reads them can be steered.
- The agent itself may do the wrong thing, by error or because it was steered.
- The server is yours, and whoever can reach its API can spend your model keys. The
  API listens on the loopback interface only; your laptop reaches it through ssh.

## Keys stay out of the sandbox

- Model keys and the GitHub credential live in the gateway, on the server. They are
  not in the sandbox and not on your laptop.
- A sandbox receives a token that expires and is valid for its own run. The gateway
  swaps it for the real key.
- Before each model call the gateway reserves the worst-case cost. A call that would
  take the run over its spending limit is refused, and the run pauses.
- `git push` from a sandbox goes to the gateway, which adds the credential. The
  sandbox never holds it.
- Your laptop keeps one revocable token for the server.

## The sandbox

| Measure | What it means |
| --- | --- |
| No capabilities | The container drops every Linux capability. |
| No new privileges | Processes inside cannot gain privileges. |
| Not root | Commands run as a user without root. |
| Read-only root | The root filesystem is read-only; scratch space is temporary. |
| Limits | Memory, CPU and process count are limited. |
| No Docker socket | The sandbox cannot start containers on the server. |
| Network by allowlist | Traffic leaves through a proxy that allows listed hosts only. Package registries are one tier; source control is another, switched on only for the stacks that need it. |
| gVisor | A second kernel boundary, by default where the server allows it (Phase 1). |
| MicroVM | No network card at all, where the server has KVM (Phase 3). |

A container shares the kernel of the server. gVisor and microVMs exist to narrow that.

## Rules the agent cannot change

- **No merge.** The agent opens a pull request. Merging is denied in every mode.
- **PolicyGate denies by default.** An action that no rule allows does not happen.
- **QA is separate.** The agent that tests a change is not the one that wrote it, and
  it works on a clean copy. Without its approval no pull request opens.
- **Repository instructions are data.** `AGENTS.md` and `CLAUDE.md` are read from the
  commit the run started from, with a size limit, and given to the agent as content of
  the repository. They cannot widen what PolicyGate allows.
- **Client hooks fail closed** (Phase 2). A hook that errors blocks the action. Hooks
  from a repository do not load until you approve their exact content.

## What exists today

`sentinel doctor <destination>` is the only code that touches a server:

- it runs your own `ssh` with `BatchMode=yes`, so it never asks for a password, and
  with `StrictHostKeyChecking=accept-new`, so a changed host key still fails;
- a destination that `ssh` could read as an option is refused before `ssh` starts;
- the probe travels on standard input and only reads; the one command it runs with
  `sudo` is `sudo -n true`, to learn whether sudo works without a password;
- `sentinel doctor` reads the exit status of `gh auth status` and discards its output.

Report vulnerabilities as described in [SECURITY.md](../SECURITY.md), not in a public
issue.
