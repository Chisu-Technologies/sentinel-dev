# Commands

`[x]` is optional; `<x>` is a value you supply.

## Available today

| Command | What it does |
| --- | --- |
| `sentinel cancel <run id>` | Stop a run of a Sentinel server. |
| `sentinel doctor` | Check that git, the GitHub CLI and ssh are installed on this machine, and that `gh` is signed in to github.com. |
| `sentinel doctor <destination>` | Check a server before installing on it: operating system, architecture, privileges, systemd, memory and Docker. It only reads: it installs and configures nothing on the server. |
| `sentinel run --dry-run "<task>"` | Show what a task would send from this repository: the GitHub repository, the commit GitHub already has, and the files in the patch. It sends nothing. |
| `sentinel runs` | List the latest runs of a Sentinel server: status, repository, pull request, cost, start and title. |
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
  are not ignored. It is limited to 3 MiB; past that, push your branch first. Without
  `--dry-run` the command stops and says that sending is not available yet.
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
  - Exit status of `sentinel cancel`: `0` when the run was stopped, when the server
    accepted the stop, or when the run had already ended; `1` for a run the server
    does not know, a refusal, a missing variable or an error from the server; `2`
    when the command line was wrong.
  - Exit status of `sentinel runs`: `0` on success, `1` for a missing variable or an
    error from the server, `2` when the command line was wrong.
  - Exit status of `sentinel watch`: `0` when the run ended as `completed`; `1` for
    any other end, a timeout, a run the server does not know, a missing variable or
    an error from the server; `2` when the command line was wrong.

## Planned

> **Planned.** None of the commands below exists yet, and the list may change. The
> phase is order, not a date; see the [roadmap](roadmap.md).

| Command | What it will do | Phase |
| --- | --- | --- |
| `sentinel setup <destination>` | Install the runtime on your server over ssh, from signed releases. It asks only for what the server lacks, and running it again is safe. | 1 |
| `sentinel login` | Sign in with GitHub, reusing the session of `gh`. | 1 |
| `sentinel run "<task>"` | Send the task and the patch that `--dry-run` shows, and start a run. | 1 |
| `sentinel pull [run]` | Fetch the branch of the pull request. | 1 |
| `sentinel resume <run>` | Continue a paused run with a larger limit or another model. | 1 |
| `sentinel stop` | Halt every run on the server until you allow them again. | 1 |
| `sentinel update` | Update the client and the runtime. | 1 |
| `sentinel` | Start an interactive session, with plan, edit and auto modes. | 2 |
| `sentinel --from-claude-code` | Continue in Sentinel a session you started in Claude Code. | 2 |
