# Commands

`[x]` is optional; `<x>` is a value you supply.

## Available today

| Command | What it does |
| --- | --- |
| `sentinel agents` | Add `sentinel mcp` to the coding agents on this machine (Claude Code, Codex, Gemini CLI, Cursor) and to the Claude desktop app. In Claude Code it also adds a hook that, when Claude Code stops at its limit, shows a desktop notification suggesting `sentinel run --from-claude-code`. `--dry-run` says what it would add and changes nothing. |
| `sentinel cancel <run id>` | Stop a run of a Sentinel server. |
| `sentinel down` | Stop Sentinel on this machine. Its data stays. |
| `sentinel backup` | Back up the data of Sentinel on this machine: its three databases, each dump checked by a `SHA256SUMS`, in a new folder under the installation's `backups` (or `--out`). It works with Sentinel running or stopped and deletes no older backup. The master key stays in the installation's `.env`, which the backup does not carry; keep that file with it. |
| `sentinel restore --yes <backup>` | Replace the data of Sentinel on this machine with a backup. It checks the backup's sums before touching anything, stops Sentinel, restores the three databases and leaves it stopped; `sentinel up` starts it again. It needs the same `.env` the backup was taken with. |
| `sentinel doctor` | Check that git, the GitHub CLI and ssh are installed on this machine, and that `gh` is signed in to github.com. |
| `sentinel doctor <destination>` | Check a server before installing on it: operating system, architecture, privileges, systemd, memory and Docker. It only reads: it installs and configures nothing on the server. |
| `sentinel login --chatgpt` | Let your Sentinel use your ChatGPT plan. You sign in with ChatGPT in your browser, on this machine, and the sign-in goes to the Sentinel you are signed in to. Only a Sentinel you run yourself (`sentinel up`) takes it. Its model calls use your plan's allowance, not an API key. If this command is not signed in yet, it first signs in to the Sentinel on this machine, approved in its panel. Lending a plan needs ChatGPT Plus or Pro. |
| `sentinel models add <name> <address>` | Let Sentinel on this machine use a model served here, such as Ollama (`http://localhost:11434/v1`) or LM Studio (`http://localhost:1234/v1`). `localhost` becomes `host.docker.internal`, the name Sentinel's containers give this machine; plain `http://` is only for this machine or a private address. It restarts Sentinel's API and broker, and the model shows in Settings > API keys under Operator OpenAI-Compatible Endpoint. `sentinel models` lists them and `sentinel models remove <name>` removes one. Only a Sentinel you run yourself takes a model on your machine. |
| `sentinel mcp` | Serve Sentinel's tools to your coding agent (Claude Code, Codex, Cursor and others) over MCP. The agent starts it; you do not run it by hand. |
| `sentinel pull <run id>` | Fetch the pull request of a run into the local branch `sentinel/<run id>` and switch to it. |
| `sentinel resume <run id>` | Continue a run that paused at its budget, from the work it kept if any, and wait for the pull request. `--budget` sets the new limit per task. |
| `sentinel run "<task>"` | Send a task and a patch of this working tree to a Sentinel server, wait, and print the pull request. With `--dry-run` it shows what would be sent: the GitHub repository, the commit GitHub already has, and the files in the patch. It then sends nothing. |
| `sentinel run --from-claude-code` | Like `sentinel run`, with the task taken from the newest Claude Code session started in this folder: what you asked there, newest request first, at most 4000 characters. Claude's answers and the tools' output are never read. Use it when Claude Code stops at its limit; `--dry-run` shows the task first. |
| `sentinel runs` | List the latest runs of a Sentinel server: status, repository, pull request, cost, start and title. |
| `sentinel session ["<what to do>"]` | Plan a change to the repository of this folder with the Architect of a Sentinel server, in the terminal, then run it as far as the mode allows. `--mode plan` (the default) talks the plan through and runs nothing; `--mode edit` asks before each task; `--mode auto` approves the plan and runs every task without asking. Type `/plan`, `/edit` or `/auto` to change the mode, `/quit` to leave: the plan stays on the server and `--plan <id>` continues it. It needs the sign-in of `sentinel login`: a key made in the panel cannot talk to the Architect, and approving or running a plan needs the person who approved that sign-in to be an admin. Sentinel never merges: each task ends in a pull request. |
| `sentinel setup <destination>` | Run Sentinel on your server, over ssh: `sentinel up` there, with the GitHub CLI session of this machine. |
| `sentinel stop --yes` | Stop every run of a Sentinel server and refuse new ones until the owner allows runs again in its panel. |
| `sentinel up --email <address>` | Run Sentinel on this machine, with Docker, and print a link to sign in to its panel. |
| `sentinel update` | Replace this command with the latest release, after the same checks as the installer. `--check` only says whether there is one. |
| `sentinel verify <pull request>` | Ask a Sentinel server to verify an existing pull request, wait, and print the verdict. |
| `sentinel version` | Print the version. |
| `sentinel watch <run id>` | Follow a run of a Sentinel server until it ends, printing its status changes and its log. |

- `<destination>` is what you would give to `ssh`: `user@host`, or a `Host` from your
  ssh config. `sentinel` runs your own `ssh` and never asks for a password; if `ssh`
  would ask for one, the check fails and says so.
- `--json` prints the report or the version as JSON.
- Exit status: `0` when every check passed, `1` when a check failed, `2` when the
  command line was wrong. A warning does not change the exit status.
- Each check that does not pass prints a `fix:` line.
- `sentinel run --dry-run` builds the patch from a copy of the git index, so your
  index, your branch and your working tree stay as they were. The patch holds the
  commits you have not pushed, your staged and unstaged changes, and new files that
  are not ignored. It is limited to 128 KiB; past that, push your branch first. A
  changed file whose text is not UTF-8 cannot travel in the patch: commit and push it
  first.
- `sentinel run "<task>"` sends that patch with the task to the server in
  `SENTINEL_URL`, with the same `SENTINEL_URL` and `SENTINEL_API_KEY` rules as
  `sentinel verify`. The server starts from the commit GitHub already has, puts the
  patch in the tree and opens a pull request for you to review. The first line of the
  task is the title of the pull request; the whole task is its description.
  - The server must say that it took the patch. A server that does not take local work
    yet would do the task without it; `sentinel run` then stops the run and exits `1`.
  - `--base <branch>` names the branch the pull request targets, when it is not the
    default one. `--no-wait` prints the run id and stops. `--timeout <duration>` is how
    long to wait, `2h` by default; the run continues on the server after it.
  - `--json` prints one JSON object on standard output; progress goes to standard
    error.
  - Exit status: `0` when the run opened a pull request that passed QA, or when
    `--dry-run` or `--no-wait` succeeded; `1` for a blocked, failed or stopped run, a
    timeout, a missing variable or an error from the server; `2` when the command line
    was wrong.
- `sentinel verify` asks a server to check out the pull request, run the project's
  tests and an independent QA in a sandbox, and comment its verdict on the pull
  request; it does not push to the repository. It needs a Sentinel server that
  accepts API keys. The open runtime is not released yet, so today that is a server
  of the hosted edition.
  - `SENTINEL_URL` is the address of the server. It must be `https`; `http` is
    accepted only for `localhost`, `127.0.0.1` and `[::1]`. It must not carry a user
    name, a query or a fragment.
  - `SENTINEL_API_KEY` is the key. It is read only from the environment, never from
    a flag.
  - `<pull request>` is a number such as `12`, for the GitHub repository of the
    current checkout, or a URL such as `https://github.com/owner/name/pull/12`.
  - `--base <branch>` names the branch the pull request targets, when it is not the
    default one. `--no-wait` prints the run id and stops. `--timeout <duration>` is
    how long to wait, `45m` by default; the run continues on the server after it.
  - `--dry-run` shows the repository, the pull request and the host of the server,
    and sends nothing. It needs no key.
  - `--json` prints one JSON object on standard output; progress goes to standard
    error.
  - Exit status: `0` when the verdict is `APPROVED`, or when `--dry-run` or
    `--no-wait` succeeded; `1` for any other verdict, a run that failed or was
    stopped, a timeout, a missing variable or an error from the server; `2` when the
    command line was wrong.
- `sentinel cancel`, `sentinel runs` and `sentinel watch` use the same `SENTINEL_URL`
  and `SENTINEL_API_KEY` as `sentinel verify`, and the same rules for them.
  - `sentinel cancel` takes the id of one run, as `sentinel runs` shows it. It reads
    the run and, when the run is still going, asks the server to stop it. A run that
    already ended is left as it is. Some servers stop the run at once; others hand the
    stop to the machine of the run, and then the command says that the stop was
    accepted and `sentinel watch` shows when the run ends. It needs a server that
    accepts an API key for stopping a run. A server that stops runs only from its
    panel refuses, the command says so, and the run keeps going.
  - `sentinel runs` prints a header and one line per run, newest first; it prints
    `No runs yet.` when there are none. `--limit <n>` is how many to list, from 1 to
    100, `20` by default. `--json` prints the runs as a JSON array with the same
    fields.
  - `sentinel watch` takes the id of one run, as `sentinel runs` shows it. It asks for
    the state of the run every 5 seconds and prints each change of status to
    standard error. It prints to standard output the recent lines of the run log,
    only the lines that are new since the last time, one short line per event: the
    time, who acted (`implementer`, `qa`) and what it did. A line the client does not
    recognise as an event is printed as it came. `--raw` prints the log as the server
    gives it, one JSON event per line.
    `--timeout <duration>` is how long to follow, `2h` by default; the run continues
    on the server after it, and so after Ctrl-C.
- `sentinel up` runs the open edition of Sentinel on this machine: the panel, the
  server and an agent sandbox, with Docker. It needs Docker Desktop (Windows, macOS) or
  Docker Engine with the Compose plugin (Linux), and the [GitHub CLI](https://cli.github.com)
  signed in to github.com (`gh auth login`).
  - Sentinel reaches GitHub with the session of that GitHub CLI, and with nothing else.
    `sentinel up` reads its token (`gh auth token`) before it writes or downloads
    anything, keeps it in the installation's `.env` (never printed) and reads it again
    on every run, so signing in again with `gh` reaches Sentinel at the next
    `sentinel up`. Without the GitHub CLI, or without a session, it stops and says what
    to run. The session needs the `repo` scope, which `gh auth login` grants; without
    it, `sentinel up` says to run `gh auth refresh -s repo`.
  - Sentinel then acts on GitHub as you: it sees the repositories you can see, and the
    branches and pull requests it opens are yours. GitHub does not let the author of a
    pull request approve it.
  - The first time it needs `--email`, the owner of this installation. It creates the
    secrets of the installation (they are never printed), downloads the images, prepares
    the database and starts everything. At the end it prints the address of the panel,
    `http://localhost:8080`, and a link to sign in. The link works once and expires in
    15 minutes.
  - Running it again updates and starts what is stopped. It keeps the secrets and the
    data. `--sign-in` prints a new sign-in link.
  - It keeps its files in one folder: `.sentinel/local` in your home folder, or the one
    in `SENTINEL_HOME` or `--dir`. On Windows, a folder under `AppData` does not work:
    Docker Desktop does not see the files there.
  - An installation of the open edition has no limits on runs, members or repositories.
  - If the machine restarts or Docker stops while a run is working, the task it was on
    ends blocked and you run it again. To have Sentinel start that task again by
    itself, add `SENTINEL_STORY_RESTART_RETRIES=1` (at most `3`) to the installation's
    `.env` and run `sentinel up`. Each new attempt starts the task from the beginning
    and pays for it again.
  - The images are built for `linux/amd64`. On Apple Silicon, Docker runs them under
    emulation; that has not been measured yet.
- `sentinel setup <destination>` runs the open edition on a server instead of this
  machine. It needs a release of `sentinel` here, the GitHub CLI signed in here, and
  on the server Docker, `curl` and a user that may use Docker; it never runs `sudo`.
  - It first runs the checks of `sentinel doctor <destination>` and stops on a failed
    one or when Docker is missing. The privileges check does not stop it.
  - It installs on the server the same release as this `sentinel`, with
    `scripts/install.sh` read from that release, in `~/.local/bin`. A server that
    already has that release keeps it.
  - It then runs `sentinel up --github-token-stdin` there. The token of the GitHub CLI
    session of this machine travels on the standard input of `ssh`, never in a
    command line, and is kept in the server's installation like `sentinel up` keeps
    it. Run `sentinel setup` again after `gh auth login` to give the server the new
    session.
  - `--email` and `--sign-in` mean what they mean for `sentinel up`.
  - The panel listens only on the server's loopback. To open it from this machine,
    keep `ssh -N -L 8080:127.0.0.1:8080 <destination>` running and open
    `http://localhost:8080`.
  - Running it again is safe: it updates and starts what is stopped, as `sentinel up`.
  - Exit status: `0` on success, `1` when a check, the install or `sentinel up` on
    the server fails, `2` when the command line was wrong.
- `sentinel down` stops what `sentinel up` started. It removes nothing: the next
  `sentinel up` starts it again with the same data.
  - Exit status of `sentinel up` and `sentinel down`: `0` on success, `1` when Docker is
    missing or stopped, a step fails or the first run has no `--email`, `2` when the
    command line was wrong.
  - Exit status of `sentinel cancel`: `0` when the run was stopped, when the server
    accepted the stop, or when the run had already ended; `1` for a run the server
    does not know, a refusal, a missing variable or an error from the server; `2`
    when the command line was wrong.
- `sentinel pull` takes the id of one run. It reads the run from `SENTINEL_URL` and
  fetches its pull request with `git fetch` from the repository's own remote. It
  refuses when the working tree has uncommitted changes to tracked files or when the
  pull request is in another repository. A branch that already exists only moves
  forward: a local commit the pull request does not have stops it before anything
  changes. It never merges, rebases, stashes or discards.
  - Exit status: `0` when the branch was created or moved, `1` otherwise, `2` when the
    command line was wrong.
- `sentinel resume` takes the id of a run that ended because it reached its budget per
  task. It reads the run's task from `SENTINEL_URL` and starts a new run of it that
  begins with the work the paused run kept, so nothing done so far is repeated; a run
  that paused before writing anything starts the task again. Without
  `--budget` the new run gets the organization's limit; a limit above it is lowered to
  it, and only the owner can resume above it, in the panel. A run is resumed once. The
  server names the run it continues in its answer; a server that does not, and so
  would do the task again from the start, has that new run stopped at once. It waits
  and prints like `sentinel run`, with the same `--json`, `--no-wait`, `--timeout` and
  `--base`.
  - Exit status: `0` when the resumed run passed QA and opened a pull request, `1`
    otherwise, `2` when the command line was wrong.
- `sentinel stop --yes` stops every run of the organization of your API key. Without
  `--yes` it explains what it would do and sends nothing. A key can stop runs but not
  allow them again: only the owner can, in the panel.
  - Exit status: `0` when the server stopped the runs or accepted the stop, `1` for a
    refusal, a missing variable or an error from the server, `2` when the command line
    was wrong.
- `sentinel update` reads the latest release of this repository on GitHub. The archive
  must match `SHA256SUMS`, and when `cosign` is installed, `SHA256SUMS` must carry the
  signature of the release workflow; `SENTINEL_REQUIRE_SIGNATURE=1` stops the update
  when `cosign` is missing. The new binary is written beside the old one and renamed
  into place, so an interrupted update leaves the old one whole. A development build is
  not updated: install a release with the installer.
  - Exit status: `0` when it updated, when this is the latest release, or with
    `--check`; `1` for a failed check or download, `2` when the command line was wrong.
  - Exit status of `sentinel runs`: `0` on success, `1` for a missing variable or an
    error from the server, `2` when the command line was wrong.
  - Exit status of `sentinel watch`: `0` when the run ended as `completed`; `1` for
    any other end, a timeout, a run the server does not know, a missing variable or
    an error from the server; `2` when the command line was wrong.

## Sentinel in your coding agent

`sentinel mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server
over stdio. Your agent writes the change, with its own model and its own sign-in;
Sentinel runs the tests in its sandbox, has a QA and reviewers that are not the model
that wrote the change check it, and signs the result. Sentinel never merges.

In Claude Code:

```bash
claude mcp add --transport stdio sentinel -- sentinel mcp
```

Or install the Sentinel plugin, which brings the same server and teaches Claude Code
when to use it: `/sentinel:verify` sends the change you made, `/sentinel:verify-pr`
checks a pull request, and `/sentinel:runs` shows the latest runs. Claude Code also
uses them on its own when a change is ready.

```bash
claude plugin marketplace add Chisu-Technologies/sentinel-dev
```

```bash
claude plugin install sentinel@chisu-sentinel
```

Use the plugin or the `claude mcp add` line, not both: each adds a server named
`sentinel`.

Or let `sentinel agents` add it to every agent it finds. Claude Code, Codex and
Gemini CLI get it through their own `mcp add` command, so their settings keep the
format they expect. Cursor has no such command: `~/.cursor/mcp.json` is edited, and
the file it had is kept next to it as `mcp.json.before-sentinel`. The Claude desktop app has no such command either: its
`claude_desktop_config.json` is edited the same way, in the folder the app uses
(`%LOCALAPPDATA%\Claude-Data\<package>` for the app from the Microsoft Store,
`%APPDATA%\Claude` for the other Windows installer, `~/Library/Application Support/Claude`
on macOS). The app loads Sentinel when it starts again. An agent that
already has Sentinel is left as it is, and a file that is not plain JSON is never
rewritten. Restart an agent that is open so it loads Sentinel.

Any other agent that takes a stdio MCP server needs the command `sentinel` with the
argument `mcp`. The server it reaches is the one of `sentinel login`, or
`SENTINEL_URL` and `SENTINEL_API_KEY` in the agent's environment.

| Tool | What it does |
| --- | --- |
| `sentinel_verify_pull_request` | Verify a pull request of the repository in the agent's folder. Returns the run id at once. |
| `sentinel_run_task` | Send a task with the work in the agent's folder that is not on GitHub yet. Returns the run id at once. |
| `sentinel_run_status` | Where a run is and, once it ends, its pull request, verdict, tests and cost. Read-only. |
| `sentinel_list_runs` | The latest runs. Read-only. |
| `sentinel_list_repos` | The repositories connected to Sentinel, and how far its Brain has read each one. Read-only. |
| `sentinel_brain` | What the Brain knows of one repository: the commit it read, its files with their headings, and the digest of each part. Read-only. |
| `sentinel_stop_run` | Stop one run that has not ended. |

Each tool runs the command you would type, with `--json`, so the agent gets what a
script gets. Nothing but protocol messages is written to stdout.

## Planned

> **Planned.** None of the commands below exists yet, and the list may change. The
> phase is order, not a date; see the [roadmap](roadmap.md).

| Command | What it will do | Phase |
| --- | --- | --- |
| `sentinel login` | Sign in with GitHub, reusing the session of `gh`. | 1 |
| `sentinel resume --model` | Continue a paused run on another model. | 1 |
| `sentinel update` for the runtime | Update the Sentinel that `sentinel up` runs, not only this command. | 1 |
| `sentinel` | Start an interactive session, with plan, edit and auto modes. | 2 |
| `sentinel --from-claude-code` | Continue in Sentinel a session you started in Claude Code. | 2 |
