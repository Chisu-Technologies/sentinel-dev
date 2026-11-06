# Sentinel for developers

**Your agent writes; Sentinel proves it works.**

Sentinel for developers checks the work of your coding agent. It will run your tests,
start your app and drive a real browser against it, then report what actually
happened instead of what the agent claims happened.

Claude Code is a product of Anthropic and Codex of OpenAI; Sentinel is not affiliated
with either.

## Status: pre-alpha

There is nothing to install yet. This repository is public from day one so you can
see what is being built and how it is licensed. Access to the beta will be
controlled by Sentinel, not by the visibility of this repository.

## How it will work

- A plugin for Claude Code and, later, for Codex. It will work with Claude Code and
  use your own session; it will not replace it.
- A local program that will run your tests, your app and a browser in Docker on your
  own machine.
- A free Sentinel account that will store the plan, the Brain projection and the
  evidence. The Brain is Sentinel's map of your repository: which files and symbols
  exist and how they relate to each other.

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

## License

MIT. See [LICENSE](LICENSE). Third-party components are listed in
[THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).
Contributions are welcome under the DCO; see [CONTRIBUTING.md](CONTRIBUTING.md).
