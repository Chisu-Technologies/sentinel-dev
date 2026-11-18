# Changelog

Notable changes are listed here, following
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is
`0.y.z`, anything may change at any time.

## [Unreleased]

### Added

- `sentinel mcp` serves Sentinel's tools to a coding agent over MCP (stdio): verify a
  pull request, send a task with this working tree, follow a run, list runs and stop
  one. The agent writes with its own model and sign-in; Sentinel verifies and signs.
  In Claude Code: `claude mcp add --transport stdio sentinel -- sentinel mcp`.
- `sentinel agents` adds `sentinel mcp` to the coding agents it finds: Claude Code,
  Codex and Gemini CLI through their own command, Cursor by editing
  `~/.cursor/mcp.json` after keeping a copy. `--dry-run` changes nothing.
- `sentinel login --chatgpt` lets your Sentinel use your ChatGPT plan, through OpenAI's
  "Sign in with ChatGPT". The sign-in runs on this machine and goes to the Sentinel you
  are signed in to; no token is printed or kept here. It needs a Sentinel release with
  the ChatGPT plan.
- `sentinel run --from-claude-code` hands Sentinel the work a Claude Code session left
  in this folder, for example when Claude Code reached its limit. The task is what you
  asked in the newest session there, newest request first and at most 4000 characters;
  Claude's answers and the tools' output are never read. `--dry-run` shows it first.

## [0.3.4] - 2026-11-17

### Added

- `sentinel login --url <address>` signs in to a Sentinel server: an owner or admin
  approves the code it prints in the panel, and the command keeps an API key for you,
  readable by you only. `sentinel logout` forgets it. `SENTINEL_URL` and
  `SENTINEL_API_KEY` still win. It needs a server that supports it.
- `sentinel pull <run id>` fetches the pull request of a run into the local branch
  `sentinel/<run id>` and switches to it. It refuses uncommitted changes, and an
  existing branch only moves forward.
- `sentinel resume <run id>` continues a run that paused at its budget, from the work
  it kept if any, with `--budget` for the new limit. It needs a server that resumes runs.
- `sentinel stop --yes` stops every run of the server and refuses new ones until the
  owner allows them again in the panel. It needs a server that accepts it from an API
  key.
- `sentinel update` replaces the command with the latest release after the same checks
  as the installer; `--check` only says whether there is one.

### Changed

- The server that `sentinel up` runs accepts `sentinel login`, `sentinel resume` and
  `sentinel stop`, and its QA knows which part of a change is your local work. A run
  that paused at its budget before writing anything can be resumed too: the task
  starts again under the new budget.

### Fixed

- `sentinel run` no longer leaves out of the patch an edit of the same size made in
  the same instant as the last git command, which it could on a fast machine.

## [0.3.3] - 2026-11-17

### Added

- `sentinel run "<task>"` sends the task and a patch of your working tree to a Sentinel
  server, waits, and prints the pull request. It stops the run when the server did not
  take the patch, as a server without support for local work would do the task
  without it. `--dry-run` still sends nothing.
- The server that `sentinel up` runs accepts a task together with your uncommitted
  work, which `sentinel run` uses.
- A task that reaches its budget pauses instead of failing, and resumes with a larger
  limit or on another model.
- The owner can stop every run of the installation at once; only the owner allows runs
  again.
- Every change gets a security review and a scan for secrets and insecure code before it
  is approved.

### Changed

- The patch of a task is limited to 128 KiB, the limit of the server, and a changed
  file whose text is not UTF-8 is refused before anything is sent.
- The panel uses a side menu, also on a phone.
- A task that does not fit in the machine's memory waits for room instead of failing.

### Fixed

- `sentinel verify` accepts a server that queues the run, which answers `202 Accepted`
  instead of `201 Created`.
- Receipts are signed again. `sentinel up` wrote a signing key the server could not
  load, so every receipt showed as not signed; each `up` now replaces such a key and
  says so, keeping every other setting.
- The Brain works on one machine: it answered that the shared Brain was disabled.
- Architect plans move past their first step on one machine.
- An approved plan no longer waits forever for checks on a repository that has no
  workflow running on pull requests; merging the plan's pull request is still yours.
- A message to a failed, completed or cancelled plan is refused with a clear reason
  instead of leaving the Architect's turn open.


## [0.3.2] - 2026-11-15

### Added

- `sentinel setup <destination>` runs Sentinel on your server over ssh: it checks the
  server, installs the same release of `sentinel` there and runs `sentinel up` with the
  GitHub CLI session of this machine, passed on the standard input of `ssh`.
- `sentinel up --github-token-stdin` reads the GitHub token from the standard input,
  for a machine without the GitHub CLI.
- Sentinel follows its pull requests by asking GitHub once a minute, so merges and
  review comments reach the run without a webhook or a public address.
- Settings > Repositories can open Sentinel's pull requests as a bot. One click
  creates a private GitHub App on your account and you choose the repositories it
  may use; its pull requests come from `<app>[bot]`, so you can review and approve
  them. It is optional: without it, Sentinel keeps using your GitHub CLI session.

### Changed

- Settings shows the open edition and this installation where it used to show
  billing and a hosted machine, which do not exist in the open edition.
- Each model call asks only for the output the run's remaining budget can pay for,
  and a run stops before a call it cannot pay for instead of after it.

### Fixed

- Runs started on a local installation failed before they did any work: the worker
  could not reach the local model broker, and after that every run stopped at its
  first model call because the installation had no price list. Both work now.
- Runs use the model you choose in Settings > AI keys. Before, they always used
  the default model.

## [0.3.1] - 2026-11-14

### Fixed

- `sentinel up` no longer creates an empty `tenant-releases` folder next to the
  installation: the bundle stops mounting a folder the open edition never reads.
- When GitHub is not set up, the panel says to run `sentinel up` again with the GitHub
  CLI signed in, instead of pointing to a token setting that no longer exists.

## [0.3.0] - 2026-11-14

### Changed

- `sentinel up` needs the GitHub CLI signed in to github.com. Sentinel reaches GitHub
  with that session and with nothing else: `sentinel up` reads its token, keeps it in
  the installation's `.env`, reads it again on every run and prints the account.
  Without the GitHub CLI or a session, or with a session that cannot work on
  repositories, it stops and says what to run.
- The end-to-end workflow signs the GitHub CLI in before `sentinel up`, with a token
  kept as a repository secret. Without that secret it skips `sentinel up` and says so
  in a warning, instead of passing as if it had run.

## [0.2.3] - 2026-11-13

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
