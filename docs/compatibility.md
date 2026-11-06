# Compatibility

Status: planned design, not implemented. Nothing here has been tested, because there
is nothing to test yet.

> **Planned.** Every entry below is a plan. Where a fact depends on a vendor, this page
> links to the vendor's documentation and does not restate it. Vendor pages were
> checked on 2026-11-06.

## Hosts

| Host | State | Notes |
| --- | --- | --- |
| Claude Code in the terminal | Planned (Phase 1) | The first and only host of the closed beta. |
| Claude Code in the desktop app, Code tab | Planned | Whether the sign-in step can open your browser there is unverified. |
| Codex (CLI and the desktop app's Codex mode) | Planned (Phase 2) | After the Claude Code beta. Several points are unverified; see below. |

### Claude Code

Sentinel for developers will need a recent Claude Code, because the browser sign-in
step depends on it. The minimum version will be confirmed at release, and
`/sentinel:setup` will check it.

Plugin installation is described in Anthropic's
[plugin installation guide](https://code.claude.com/docs/en/discover-plugins). In the
same guide, the tab for the desktop app says the plugin browser works in local and SSH
sessions of the Code tab. Which Claude plan the desktop app requires is set by
Anthropic; see the
[desktop app documentation](https://code.claude.com/docs/en/desktop).

What is unverified for the Code tab: that the browser sign-in step appears in local
sessions there. If it does not, the sign-in command is designed to print the URL and a
code as text.

### Codex

> **Planned (Phase 2).** Codex is planned after the Claude Code beta. These points are
> still to verify against Codex itself, and the answers could change the plan.

- Whether Codex loads a local MCP server bundled inside a plugin. If it does not, the
  plan is to register the local program with `codex mcp add` during `/sentinel:setup`.
- Whether Codex shows the sign-in prompt for third-party servers. The design does not
  depend on it: approval happens on the web, and the sign-in command prints the URL as
  text.
- Where the Codex plugin manifest goes.
- How long a tool call may take before Codex times it out. The plan assumes a short
  limit, so long operations will be started and then polled.
- Whether a Codex subagent can be restricted to a list of tools. If it cannot, a Codex
  QA subagent could edit files; a task would still need the runner report to close.
- Skills are started with `$` or `/skills`, not with `/sentinel:`.

Which ChatGPT plans include Codex is set by OpenAI; read their documentation.

## Not supported

> **Not supported.** The surfaces below are outside the current plan.

The local program has to run on your machine, started by your agent's plugin. These
surfaces are out of scope:

- Claude chat on desktop, web and mobile. Anthropic describes what those surfaces load
  from a plugin in [Plugins on claude.ai and in Cowork](https://claude.com/docs/plugins/overview)
  (checked 2026-11-06).
- Cowork, until it has been tested.
- ChatGPT chat.
- Cursor.

## Operating systems

The local program is planned as one program per platform. All cells are planned and
untested.

| Operating system | x64 | arm64 |
| --- | --- | --- |
| Windows | Planned, untested | Planned, untested; the Brain engine is to be confirmed |
| macOS | Planned, untested | Planned, untested |
| Linux | Planned, untested | Planned, untested |

Notes:

- The Brain engine is a third-party program, `codebase-memory-mcp`, which is MIT
  licensed. The version the plan pinned has no Windows ARM64 build, so that cell is to
  be confirmed.
- How the program is delivered to each system is not decided.
- Windows notes will be added only for steps tested on Windows.

## Requirements

- Docker. The local program runs tests, the app and the browser in containers, so
  Docker has to work on your machine. Docker Desktop has its own license terms; read
  the [Docker Desktop license agreement](https://docs.docker.com/subscription/desktop-license/)
  (checked 2026-11-06). Whether free alternatives that are compatible with the Docker
  API work has not been tested, so this page does not claim that they do.
- On Windows with WSL2, containers have no CPU limit. The memory and process limits
  stay, and `/sentinel:setup` is planned to warn about it.
- The browser image is planned for both amd64 and arm64 in Phase 2. Until then, on a
  Mac with Apple Silicon the browser would run emulated.
- A GitHub account, for sign-in.
- An invitation to the beta. Access is by invitation only for now.

## Stacks

Phase 1 plans to support Node and Python projects, using images published for those
stacks. A stack Sentinel does not know will fail closed: the run will not start. Other
stacks are not planned.
