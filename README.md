<!-- Insert new languages in order by language code. -->
English · [Español](README.es.md)

[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel for developers

**Your agent writes; Sentinel proves it works.**

Sentinel for developers checks the work of your coding agent. It will run your tests,
start your app and drive a real browser against it, then report what actually happened
instead of what the agent claims happened.

> **Pre-alpha.** Nothing is installable yet; this repository holds the planned design.
> The beta will be by invitation only, and there is no way to request an invitation
> yet. Access is controlled by Sentinel, not by this repository being public.

## How it will work

- **A plugin** for Claude Code, and later Codex. Your agent does the thinking, in your
  own session and with your own membership. Sentinel will not ask for an API key.
- **A local program**, started by the plugin as an MCP server. It will run your tests,
  your app and a browser in Docker on your machine.
- **A free Sentinel account** that will store the plan, the Brain projection (a map of
  your repository's files and symbols) and the evidence.

A day of work: plan with `/sentinel:architect`, approve the plan on the web, implement
and test with `/sentinel:run`, then review the local branch yourself. See
[commands](docs/commands.md) and [how it works](docs/how-it-works.md).

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

## Evidence is reported, not attested

Your machine will run the checks and report the results, so the evidence can be forged
on your machine. Sentinel labels it "reported", not "attested". See the
[security model](docs/security-model.md).

## What is open

The plugin, the local program and the published formats (plan, runner report, Brain
graph, evidence) will be MIT-licensed in this repository. The hosted service, the
engine behind the paid plans and the Heads are closed source and are not here.

## Requirements (planned)

- Docker.
- Claude Code in the terminal, the only host of the closed beta; the minimum version
  will be confirmed at release. The desktop app's Code tab is planned and unverified.
- Windows, macOS or Linux, on x64 or arm64; Windows on arm64 is to be confirmed. None
  of them is tested yet, and how the program will be delivered is not yet decided.
- Usage limits on the free plan will apply; they are not published yet.
- A GitHub account, to sign in.

## Roadmap

Four phases, in order and without dates: the hosted service side, a closed beta in
Claude Code, QA with reviewers and the Brain, then the public launch. See the
[roadmap](docs/roadmap.md).

## License and contributing

MIT; see [LICENSE](LICENSE) and [THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).
Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
Contributions need a DCO sign-off; see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[code of conduct](CODE_OF_CONDUCT.md).

---

Claude Code is a product of Anthropic and Codex of OpenAI; Sentinel is not affiliated
with either. Sentinel for developers is not related to Microsoft Sentinel.
