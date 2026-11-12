# Changelog

Notable changes are listed here, following
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is
`0.y.z`, anything may change at any time.

## [Unreleased]

Nothing has been released.

### Added

- `sentinel doctor`: checks git, the GitHub CLI and ssh on this machine.
- `sentinel doctor <destination>`: checks a server over ssh before installing on it.
- `sentinel run --dry-run "<task>"`: shows what a task would send from the current
  repository, and sends nothing.
- `sentinel verify <pull request>`: asks a Sentinel server to verify an existing pull
  request, waits, and prints the verdict. It reads `SENTINEL_URL` and
  `SENTINEL_API_KEY` from the environment.
- `sentinel runs`: lists the latest runs of a Sentinel server, as a table or as JSON.
- `sentinel watch <run id>`: follows a run until it ends, printing its status changes
  and the new lines of its log, and exits `0` only when the run completed.
- `sentinel version`.

### Changed

- The project is now the open edition of Sentinel: a command-line client and a runtime
  you install on your own server. The earlier design, a plugin with a local program
  and a hosted account, is withdrawn; nothing of it was released.
- The license is Apache-2.0 instead of MIT.
