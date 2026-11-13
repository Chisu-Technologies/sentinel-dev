# Changelog

Notable changes are listed here, following
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is
`0.y.z`, anything may change at any time.

## [Unreleased]

### Changed

- On Linux and macOS, the installer puts its folder on the `PATH` of new terminals,
  with one line in the startup file of your shell, and prints the line to use
  `sentinel` in the same terminal. `SENTINEL_NO_MODIFY_PATH=1` leaves the files alone.
- On Linux, when your user may not use Docker, `sentinel up` says so and gives the
  command that fixes it, instead of reporting that Docker is not running.

### Added

- An end-to-end workflow that installs `sentinel` with the one-line installer on a clean
  Ubuntu machine, runs `sentinel up`, signs in as the owner, and runs `sentinel down`
  and `up` again. On macOS it checks the install and a new terminal only: GitHub's macOS
  machines have no Docker that holds up through the image download.

## [0.2.2] - 2026-11-13

### Fixed

- The installers ask GitHub which release is the latest and download that tag. Right
  after a release, the address they used went on serving the previous one for minutes.

### Changed

- On Windows, the installer adds its folder to your user `PATH`, so `sentinel` works
  without any step of your own. `SENTINEL_NO_MODIFY_PATH=1` leaves the `PATH` alone.

## [0.2.1] - 2026-11-13

### Fixed

- `sentinel up` keeps its files in `.sentinel/local` in the home folder. On Windows it
  used a folder under `AppData`, which Docker Desktop did not see: the first `up`
  failed while starting the package proxy.

## [0.2.0] - 2026-11-13

### Added

- `sentinel up`: runs the open edition of Sentinel on this machine with Docker, and
  prints a link to sign in to its panel. `sentinel down` stops it and keeps its data.

### Changed

- `sentinel watch` prints each event of the run log as one short line: the time, who
  acted and what it did. `--raw` prints the log as the server gives it, one JSON
  event per line, as 0.1.0 did.

## [0.1.0] - 2026-11-13

The first release.

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
- `sentinel cancel <run id>`: asks a Sentinel server to stop a run that is still going.
  It needs a server that accepts an API key for stopping a run.
- `sentinel version`.
- Installers for Linux, macOS (`scripts/install.sh`) and Windows
  (`scripts/install.ps1`), and a release workflow that builds one archive per system
  and signs their checksums.

### Changed

- The project is now the open edition of Sentinel: a command-line client and a runtime
  you install on your own server. The earlier design, a plugin with a local program
  and a hosted account, is withdrawn; nothing of it was released.
- The license is Apache-2.0 instead of MIT.
