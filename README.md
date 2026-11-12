<!-- Insert new languages in order by language code. -->
English · [Español](README.es.md)

[![ci](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/ci.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/ci.yml)
[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel

**Hand a task to an agent on your own server. Get back a pull request that a separate
QA tested before it was opened.**

Sentinel runs coding agents on a machine you control. Each task runs in a sandbox that
never holds your keys. A second agent, which did not write the change, tests it. The
result is a pull request for you to review; the agent cannot merge.

> **Pre-alpha.** Only `sentinel cancel`, `sentinel doctor`, `sentinel run --dry-run`,
> `sentinel runs`, `sentinel verify`, `sentinel version` and `sentinel watch` exist
> today. The rest of this page is the planned design; see the
> [roadmap](docs/roadmap.md).

## How it will work

```console
$ sentinel setup deploy@my-server     # install the runtime on your server, over ssh
$ sentinel run "fix the flaky login test"
$ sentinel watch                      # follow the run from any terminal
$ sentinel pull                       # fetch the branch of the pull request
```

- **Your laptop** runs the `sentinel` command, on Linux, macOS or Windows. It keeps
  one revocable token and nothing else.
- **Your server** does the work: Ubuntu 22.04, Ubuntu 24.04 or Debian 12, installed with
  one command over ssh. Model keys and the GitHub credential stay there, outside the
  sandbox. A run goes on when you close the laptop.
- **GitHub** receives a pull request, opened only after QA passed.

Read [how it works](docs/how-it-works.md) and the [commands](docs/commands.md).

## What it is built around

- **A pull request that was checked.** A QA agent separate from the one that wrote the
  change runs the tests and reviews the diff before the pull request opens.
- **Runs that are not lost.** Each step of a run is saved when it completes, so a run
  continues after the server restarts. Restarting the server in the middle of a run is
  one of the tests that closes Phase 1; it has not been run yet.
- **Rules that are on without configuring anything.** A spending limit per run, a
  hardened sandbox and the ban on merging. A run that reaches its limit pauses, and you
  resume it with a larger limit or another model.

## Try what exists

With Go 1.26 or newer:

```bash
go install github.com/Chisu-Technologies/sentinel-dev/cmd/sentinel@latest
```

No release has been published yet. An installer of one command and signed releases
are ready for the first one; see [installing the command](docs/install.md).

```bash
sentinel doctor
```

```bash
sentinel doctor deploy@my-server
```

```bash
sentinel run --dry-run "fix the flaky login test"
```

```bash
sentinel verify 12
```

The first form checks git, the GitHub CLI and ssh on your machine. The second checks a
server before you install on it; it only reads, and installs nothing there. Each
failed check prints how to fix it. The third shows what a task would send from the
repository you are in, and sends nothing. The fourth asks a Sentinel server, the one in
`SENTINEL_URL`, to verify pull request 12 of the repository you are in, and prints the
verdict; see the [commands](docs/commands.md).

## Editions

| Edition | For | Paid with |
| --- | --- | --- |
| Open | One person or a small team, on their own server, with their own model keys | Nothing; Apache-2.0 |
| Hosted | People who do not want to run a server | A subscription |
| Teams | Several servers, single sign-on, audit, a central key vault, limits per team | A yearly license |
| Enterprise | On-premises or private cloud, also without internet access | A contract |

QA, durable runs and the ban on merging are part of the open edition. The paid editions
are planned and are not in this repository.

## What leaves your machine

In the open edition your code and your prompts go to three places: your server, GitHub
and the model provider you choose. The Sentinel service will receive a GitHub sign-in
and usage counts, such as how many runs there were, how long they took, which model
they used and how they ended. It will not receive prompts, code or diffs. The exact
list is not final; it will be on the [privacy page](docs/privacy.md) before the first
release, and what is not on it will not be sent.

Today `sentinel cancel`, `sentinel verify`, `sentinel runs` and `sentinel watch` are the
commands that contact a Sentinel server, the one in `SENTINEL_URL`. They send your API
key, and the repository name and pull request number (`verify`) or a run id (`cancel`,
`watch`); they read back the state of your runs, and `watch` their recent log lines.
None of them reads or sends your working tree.

## Requirements (planned)

- On your laptop: git, the GitHub CLI (`gh`) and an ssh client.
- On your server: Ubuntu 22.04, Ubuntu 24.04 or Debian 12, on x86_64 or arm64, with
  4 GB of memory and either root or sudo without a password.
- A GitHub account, and a key for the model provider you choose.

The memory figure is the target Sentinel is being sized for, not yet a measured
minimum.

## License and contributing

Apache-2.0; see [LICENSE](LICENSE), [NOTICE](NOTICE) and
[THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md). Report vulnerabilities privately as
described in [SECURITY.md](SECURITY.md). Contributions need a DCO sign-off; see
[CONTRIBUTING.md](CONTRIBUTING.md) and the [code of conduct](CODE_OF_CONDUCT.md).

---

Sentinel is not related to Microsoft Sentinel.
