# Changelog

Notable changes are listed here, following
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is
`0.y.z`, anything may change at any time.

## [Unreleased]

### Added

- `sentinel resume --reply "<text>"` answers the question a run was paused with, or
  says what changes in the task. A run paused with a question prints it, with the
  command that answers it, where a blocked run prints its reason; `sentinel resume`
  without `--reply` on such a run prints the question and sends nothing. It needs a
  server that takes a reply: one that does not has the new run stopped at once.

### Changed

- `sentinel resume` continues toward the branch the stopped run targeted, on a server
  that keeps it. Before, without `--base` the new run's pull request targeted the
  default branch. `--base` with another branch than the one the run targeted is
  refused.
- The documentation describes a run with a supervisor, QA and adversarial QA, and says
  what bounds a run: its spending limit.
- `sentinel run` and `sentinel verify`, when they stop waiting for a run that is still
  going, print the command that follows it again: `sentinel watch <run id>`.
- `sentinel resume` no longer says which stops can be continued by listing them: a run
  can be continued whatever stopped it, except one a person stopped, one the
  organization's rules refused, and one that waits on its pull request.

## [0.12.7] - 2026-11-25

### Changed

- The server that `sentinel up` runs is Sentinel 4eb45f6f.
- `sentinel resume` also continues a run that ended because a reviewer's verdict could
  not be used: QA had approved the change, one reviewer answered with an incomplete
  verdict, and the task ended blocked with its change written. The new run reviews it
  again from the work the run kept.

### Fixed

- A task is no longer blocked as a failure of the model provider or of the sandbox
  because of what its tests print. When your tests ran and failed, the failure goes back
  to the agent whatever they say. Before, words such as `unauthorized`, `HTTP 401`,
  `rate limit`, `permission denied` or `403 Forbidden` in the name or the message of a
  failing test were read as that kind of failure; a test named `...Unauthorized...`
  ended its task with "Automatic recovery is not allowed for this failure".
- When your tests do not finish in the sandbox's time, the agent now gets what they had
  printed until then, and a message that says so. Before it got only a timeout notice,
  with nothing to tell which test was waiting, and ran them again.

## [0.12.6] - 2026-11-25

### Changed

- The server that `sentinel up` runs is Sentinel 7f19905b.
- `sentinel resume` also continues a run that used up its rounds: one that ended because
  QA or the reviewers still turned the change down when no round was left. The new run
  starts from the work the run kept and from what was left to correct, with rounds
  again. Before, only a run that paused at its budget or its time limit could be
  resumed, and this one had to be done again from the start.

### Fixed

- A task that runs for more than an hour keeps its access to the model. Its permission
  lasted one hour while a task may run for 110 minutes, so the server refused its next
  call at minute 61 and the task ended blocked, saying "Reconcile unknown inference cost
  before more paid work".

## [0.12.5] - 2026-11-24

### Changed

- The server that `sentinel up` runs is Sentinel e74c2102.
- Sentinel's verdict is also the commit status `sentinel/verification`: of the commit a
  verification checked (success, failure, or error when it could not conclude) and of
  each commit Sentinel publishes. A branch rule can require it, which it cannot do with
  a comment.
- The section a correction round adds to its pull request ends with the `git revert`
  that undoes the round.
- QA and the reviewers are told that the tool calls of one response run together, as
  the implementer was. They made a single call in 70 % and 76 % of their turns.

### Fixed

- A change its reviewers reject goes back to the implementer with what they said, as
  one QA rejects does. The task ended there, with the change written, approved by QA
  and not delivered. If the budget is spent by then, the task pauses with what was left
  to correct, and `sentinel resume` goes on from it.
- A task its reviewers block says what the reviewer that rejected it said. It quoted
  the first reviewer of the pair, whichever of the two had rejected.
- A pull request whose checks GitHub did not start, as when the account's Actions
  budget has run out, ends the task blocked at once with what GitHub said. Sentinel
  took those checks for a failure of the change and paid the implementer to correct
  code nothing was wrong with.
- The page of a task blocked after QA approved the change says why it is blocked. Under
  "Blocked" it showed QA's approval and no reason, as happened with those checks.

## [0.12.4] - 2026-11-23

### Changed

- The server that `sentinel up` runs is Sentinel ee0f439b.
- The reviewers of a change run at the same time instead of one after another. In the
  task measured, the four of them took 10 minutes where their sessions add up to 25.
  `SENTINEL_REVIEWERS_SIDE_BY_SIDE=0` goes back to one after another.
- A task has 110 minutes on this machine, where it had 45.
- The implementer has 48 turns, where it had 28. It is told how many it has and when
  10 and 5 are left.
- Two tasks of a plan run at the same time only when the plan puts them in the same
  parallel group. A plan with no groups runs one task at a time; the automatic mode
  ran two, whatever files they changed.
- This machine runs as many tasks at once as its CPUs hold: one per CPU, less 15 %.
  It ran as many as fit in memory. The rest wait in line.

### Fixed

- A task that runs out of turns or of time keeps its work. It stops by itself five
  minutes before its time is up, saves what it wrote and a note for the session that
  continues it, and `sentinel resume` goes on from there. It ended blocked with nothing
  saved, once two seconds after it had opened its pull request.
- A session that continues an unfinished task gets the notes of the one before it, up
  to three times. It got a diff and read the same files again.
- The Go files a task writes are formatted with `gofmt` before QA sees them. A pull
  request failed its checks on formatting the implementer had no tool to fix.
- The board does not show as failed a task that is only waiting for another to end.

## [0.12.3] - 2026-11-23

### Changed

- The server that `sentinel up` runs is Sentinel 8d75fc11.
- The implementer and QA ask the model for medium reasoning effort, with the Claude
  models that accept the setting. Every other role keeps the model's default.

### Fixed

- With the Claude models that accept it, QA and the reviewers can only send a verdict
  that has all its parts and is about the criteria of the task. A verdict that says
  neither approved nor rejected is asked for again. It read as a rejection, and a
  change every check had passed could end blocked.
- A plan the Architect is writing does not end because two commands of one answer were
  wrong: both get the one correction a plan has.
- A long plan is no longer cut while the model is still answering. It failed with
  `plan_compiler_activity_failed`.
- When the pull request of an approved plan fails on something only a person can
  change in the repository, the task says what it is.

## [0.12.2] - 2026-11-23

### Changed

- The server that `sentinel up` runs is Sentinel 9fbe9940.

### Fixed

- In a Go repository, the tests QA writes are run. The command that ran them skipped
  their folder, so it answered with the project's own tests and QA spent its turns
  finding out why.
- When QA approves a change but leaves part of its verdict out, QA is asked again. The
  task went back to the implementer, which had nothing to change.
- When a reviewer approves a change but its verdict has a part too many or too few, the
  reviewer is asked to correct it. The task ended blocked.
- A task of a stopped run reads as stopped. It read "The processing activity failed:
  Cancelled".
- `sentinel run` and `sentinel verify` show what the run cost when its story was cut
  short by a stop or by the server's time limit. They printed $0.00: the story's own
  figure is written by the work that was cut.

## [0.12.1] - 2026-11-22

### Changed

- The server that `sentinel up` runs is Sentinel 0e7e6d62.

### Fixed

- A task on a Go repository whose `go.mod` asks for Go 1.26 no longer starts from a red
  base: the sandbox's Go is 1.26.11, it was 1.22.12. When a `go.mod` asks for a newer Go
  than the sandbox has, the task stops as a broken test environment; before, Sentinel
  started a repair of the repository.
- A stopped run no longer keeps its spinners or reads $0.00: its unfinished stories end
  as stopped, and the run shows what its model calls cost.
- A stop reaches the agent sooner: the worker's heartbeat, which brings the stop, is held
  back 10 seconds at most; it was 60, and the agent kept calling the model meanwhile.
- While a run is under way, its page shows the phase it is in, what it has spent and the
  attempt it is on. It read "Preparing the repository", "$0.00" and "0 of 5" until the
  story ended.
- "Sessions so far" lists each session's role, attempt and outcome.
- The live timeline shows the first run of the tests, and says when a run waits for
  another task, such as a Brain digest, to finish.
- A Brain digest that ended well no longer shows as rejected in the worker's log.

## [0.12.0] - 2026-11-22

### Added

- Where Docker has Kata Containers, each command of the agent can run in its own
  microVM: `sentinel up` says how to turn it on, and before starting Sentinel it checks
  with a test container that the runtime set in `.env` works. If it does not, that run
  uses runc and says why. See "Each command in its own microVM" in
  `docs/security-model.md`.

### Changed

- The server that `sentinel up` runs is Sentinel 6bc7ae7f, whose sandbox reaches the
  package proxy from inside a Kata microVM.

## [0.11.0] - 2026-11-22

### Added

- While you hold the agent from the run's "Live" tab, you can run commands in its
  sandbox, with the same rules as the agent's own, and click on the picture of its
  browser. When you give it back, the agent reads what you did before going on, and
  the pull request counts your commands and clicks.

### Changed

- The server that `sentinel up` runs is Sentinel 838f8ea9.

### Fixed

- `sentinel update` on Windows no longer fails with "Access is denied" when an editor still
  runs `sentinel mcp` from the binary an earlier update replaced: that binary is moved to a
  name of its own and removed by a later update.

## [0.10.0] - 2026-11-21

### Added

- From the run's "Live" tab you can pause the agent before its next step, for up to 15
  minutes, and give it back; the pull request says how long it was held.
- With `qa_by_you`, the fixes a run needs after its QA or checks send it back are made
  by your own agent too, through the new subagent `sentinel-implementer`: a run you
  send calls no model of Sentinel's. The signed attestation says so.
- `sentinel_verify_pull_request` now has Sentinel's reviewers check the pull request
  after its QA approves, as for a run, and its verdict and comment say what they found.

### Changed

- The server that `sentinel up` runs is Sentinel 22ab9ff4.

## [0.9.0] - 2026-11-21

### Added

- The run's "Live" tab in the panel shows the agent's terminal as it works.

### Fixed

- Verifying a pull request (`sentinel:verify-pr`) reaches GitHub through the `gh` session
  that `sentinel up` uses. Before, every verification stopped before reading the pull
  request.
- A pull request's verification checks its tests against the pull request's base, so a
  test that proves the change counts as such. Before, the differential always said
  `NO_NEW_TESTS`.
- The comment on the pull request counts the tests the QA wrote, and links the signed
  receipt only when the Sentinel has a public address. Before, it said "0 tests written"
  and printed an empty link.

### Changed

- The server that `sentinel up` runs is Sentinel c5af1277.

## [0.8.0] - 2026-11-20

### Added

- Live View on this machine: the Sentinel that `sentinel up` runs starts a repository's
  app in an isolated container and opens it in a tab of its own, at
  `http://lv-<name>.localhost:<port>`. `sentinel up` chooses a free port from 8421, keeps it
  as `SENTINEL_LIVE_VIEW_PORT` and says it. The app reaches the internet, never this
  machine, Sentinel or your network.

### Changed

- The server that `sentinel up` runs is Sentinel e5e46501: it serves Live View from this
  machine, with its own gateway on `127.0.0.1:<port>`.

## [0.7.2] - 2026-11-20

### Changed

- `sentinel update` checks who built the release as the installers do: with `gh` signed in,
  GitHub's attestation of the archive, before cosign. When the check fails, nothing is
  changed. Before, it only used cosign and said the signature was not checked without it.

## [0.7.1] - 2026-11-20

### Fixed

- `sentinel login` with no address signs in to the Sentinel that `sentinel up` installed on
  this machine, as the panel's setup guide says to do. Before, it asked for `--url`.

## [0.7.0] - 2026-11-20

### Added

- Your own coding agent can answer the QA and the reviewers of the runs it sends, on its
  own plan instead of a key: `qa_by_you` on `sentinel_verify_pull_request` and
  `sentinel_run_task` (`--qa-by-agent` on `sentinel verify` and `sentinel run`), the
  sessions waiting for it in `sentinel_run_status`, and the tools `sentinel_session_open`
  and `sentinel_session_call`. Sentinel still runs every tool in its sandbox and decides
  the verdict. The Claude Code plugin does it by itself with three subagents:
  `sentinel-qa`, `sentinel-reviewer-a` (Opus) and `sentinel-reviewer-b` (Sonnet). The
  signed attestation of such a run says your agent did the QA and the reviews.

### Changed

- The installers check who built the release without cosign: with `gh` signed in, they
  ask GitHub for the attestation of the archive that each release now carries. When the
  check fails, nothing is installed. A `gh` without a session is not asked; cosign still
  works as before, and `SENTINEL_REQUIRE_SIGNATURE=1` accepts either.
- The server that `sentinel up` runs is Sentinel 32fafe19: it takes the QA and the reviews
  from your own agent, and its first steps explain how to use Sentinel from a coding agent.

## [0.6.0] - 2026-11-20

### Added

- `sentinel agents` also adds Sentinel to the Claude desktop app, by editing its
  `claude_desktop_config.json` after a backup, as it does for Cursor. It finds the folder of the
  app from the Microsoft Store, of the other Windows installer and of macOS.
- `sentinel agents` also adds Sentinel to Codex when only the Codex or ChatGPT desktop app is
  installed, with no `codex` command: those apps read the same `~/.codex/config.toml`.
- A Claude Code plugin, `sentinel@chisu-sentinel`, in this repository's marketplace: the
  `sentinel mcp` server and three skills (`verify`, `verify-pr`, `runs`) that tell Claude Code
  when to send a change to Sentinel and how to follow it until its pull request.
- [INSTALL.md](INSTALL.md): the steps an AI agent follows when someone gives it this repository
  and asks it to install Sentinel; the README and AGENTS.md point to it.
- `sentinel mcp` has two more tools, `sentinel_list_repos` and `sentinel_brain`, so the agent can
  ask which repositories Sentinel has and what its Brain knows of one. They need a Sentinel
  that lets the key of `sentinel login` read them.
- Your own MCP servers: in Settings > External tools, the Sentinel that `sentinel up` runs
  takes any MCP server (on this machine, on your network or on the internet) and gives its
  tools to the agents a task's contract allows. Tools that may change data start off.
  Composio Connect works as one of them. See [docs/mcp-servers.md](docs/mcp-servers.md).
- `sentinel up` writes the key that signs each run's access to those tools, and adds it
  to an installation made before.

### Changed

- `sentinel agents` ends by saying what to do now: which agents have Sentinel, a few things to ask
  them in your own words, and Sentinel's tools. In Claude Code, `/mcp` lists them.
- `sentinel login --chatgpt` is one command: if this CLI has no sign-in yet and `sentinel up`
  installed Sentinel here, it signs in to that one first, approved in its panel, then
  continues with ChatGPT.
- `sentinel login --url` opens the approval page in the browser too.
- The server that `sentinel up` runs gives agents the MCP servers you add in Settings >
  External tools, lets the key of `sentinel login` read your repositories and the Brain, and
  carries containerd with the fix for CVE-2026-95839.

### Fixed

- `sentinel login --chatgpt` keeps the client OpenAI issued even when OpenAI refuses the code
  exchange, as OpenAI asks, and says what to check: lending a ChatGPT plan needs ChatGPT Plus
  or Pro on the chosen workspace. Signing in again then shows OpenAI's own reason instead of
  registering a new client each time.

## [0.5.0] - 2026-11-19

### Added

- `sentinel models add <name> <address>` lets the Sentinel on this machine use a model served
  here (Ollama, LM Studio, vLLM). `localhost` becomes `host.docker.internal`; plain HTTP is
  only for this machine or a private address. `sentinel models` lists them and
  `sentinel models remove <name>` removes one. It needs a Sentinel release with local models.
- `sentinel session` plans a change with the Architect of a Sentinel server in the
  terminal, then runs it as the mode says: `plan` (the default) runs nothing, `edit`
  asks before each task, `auto` runs every task. It needs a server that lets the
  sign-in of `sentinel login` talk to the Architect.

### Changed

- The server that `sentinel up` runs accepts models served on this machine, adds Groq,
  Mistral AI, Cerebras, Together AI and DeepInfra as model providers, lets you pick the
  model of the second reviewer, and lets `sentinel session` talk to its Architect.

## [0.4.0] - 2026-11-18

### Changed

- The server that `sentinel up` runs accepts `sentinel login --chatgpt`, learns only
  from lessons you approve, runs the checks a repository declares in `.sentinel.yaml`
  before it commits, and shows which runs are reviews of a plan you sent directly.

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
- `sentinel agents` also adds to Claude Code (`~/.claude/settings.json`, after keeping a
  copy) a `StopFailure` hook for `rate_limit`: when Claude Code stops at its limit, a
  desktop notification suggests `sentinel run --from-claude-code`. The terminal must
  show OSC 9 notifications (Windows Terminal, iTerm2, ConEmu, WezTerm).
- `sentinel backup` and `sentinel restore --yes <backup>` back up and restore the Sentinel
  on this machine: its three databases as `pg_dump` custom dumps with a `SHA256SUMS`, the
  way central's own backups are made. A restore checks the sums before it stops anything.

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
