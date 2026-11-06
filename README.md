<!-- Insert new languages in order by language code. -->
English · [Español](README.es.md)

[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel for developers

**Your agent writes; Sentinel proves it works.**

Sentinel for developers checks the work of your coding agent. It will run your tests,
start your app and drive a real browser against it, then report what actually
happened instead of what the agent claims happened. The results are reported by your
own machine; see [what the evidence is and is not](#what-the-evidence-is-and-is-not).

Claude Code is a product of Anthropic and Codex of OpenAI; Sentinel is not affiliated
with either.

## Status: pre-alpha

> **Nothing here is installable yet.** Everything described below is planned.

This repository is public from day one so you can see what is being built and how it
is licensed. Access to the beta will be controlled by Sentinel, not by the visibility
of this repository. The beta is by invitation only. Questions go through
[issues](https://github.com/Chisu-Technologies/sentinel-dev/issues/new/choose) for now.

## How it will work

- A plugin for Claude Code and, later, for Codex. It will work with Claude Code and
  use your own session; it will not replace it.
- A local program that will run your tests, your app and a browser in Docker on your
  own machine.
- A free Sentinel account that will store the plan, the Brain projection and the
  evidence. The Brain is Sentinel's map of your repository: which files and symbols
  exist and how they relate to each other.

Your agent does the thinking, with your own membership. Sentinel will not pay for
inference and will not ask for an API key. Everything will happen inside your
interactive session, and the results will land in local branches. Pushing and opening
a pull request stays your decision.

> **Planned (Phase 1).** A day of work will look like this, with command names only:
> plan with `/sentinel:architect`, approve the plan in the web with
> `/sentinel:approve`, implement and test with `/sentinel:run`, check the current
> changes with `/sentinel:verify`, then review the branch yourself. See the
> [commands page](docs/commands.md) for the phase of each command.

## What is open and what is not

> **Planned.** None of the open parts exists yet; today this repository holds the
> documentation and the checks that guard it.

Will be open, under the MIT license, in this repository: the plugin (commands,
subagents and review rubrics), the local program (login, MCP server, runner, Brain
indexer and receipt verifier) and the published formats for plans, runner reports, the
Brain graph and evidence.

Not open: the hosted service, the engine behind the paid plans and the Heads. They are
not in this repository. See [how it works](docs/how-it-works.md).

## What will leave your machine, and what will not

Sent to Sentinel:

- the repository name and remote, without any user or token the URL may carry;
- the plan and the status of its tasks;
- runner reports: command, exit code, duration, a truncated tail of the output and
  digests. The tail is whatever your own commands print, so a test failure or a stack
  trace can contain lines of code. Secret filtering is best-effort, not a guarantee;
- a projection of the Brain: the names of files and symbols and how they relate, not
  their contents, up to 300 nodes and 64 KB.

Sentinel never uploads your source files or your diffs, and it does not read your
environment files or credentials; anything your commands print is subject to the
best-effort filter above. Screenshots and full app logs stay on your
machine unless you turn on uploading for that repository.

Your Sentinel token will live in the operating system keychain. The program will never
read, store or forward your Claude or ChatGPT session, and containers will receive no
secrets other than test variables you declare for that repository.

For the full picture, including what is not yet decided, read the
[privacy page](docs/privacy.md).

## What the evidence is and is not

Results are reported by your machine to the hosted service. Evidence can be forged on
your machine, so Sentinel labels it "reported" and never "attested". Read the
[security model](docs/security-model.md) for the design intent and its limits.

## Requirements (planned)

- Docker.
- Claude Code, at a minimum version to be confirmed at release.
- A supported operating system; see the [compatibility page](docs/compatibility.md).

## Install

There is nothing to install yet. The planned flow is described, with every command
labeled as not working, in [getting started](docs/getting-started.md).

## Documentation

- [Documentation index](docs/README.md)
- [Getting started](docs/getting-started.md) and [how it works](docs/how-it-works.md)
- [Privacy](docs/privacy.md) and [security model](docs/security-model.md)
- [Commands](docs/commands.md) and [compatibility](docs/compatibility.md)
- [FAQ](docs/faq.md), [glossary](docs/glossary.md) and [roadmap](docs/roadmap.md)

## What is next

The work is planned in four phases, in order and without dates: Phase 0 builds the
hosted service side, Phase 1 is a closed beta in Claude Code, Phase 2 adds QA,
reviewers and the Brain, and Phase 3 is the public launch. See the
[roadmap](docs/roadmap.md).

## License

MIT. See [LICENSE](LICENSE). Third-party components are listed in
[THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).

## Security and contributing

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).
Contributions are welcome under the DCO; see [CONTRIBUTING.md](CONTRIBUTING.md).

---

Product names belong to their owners. Sentinel for developers is not related to
Microsoft Sentinel.
