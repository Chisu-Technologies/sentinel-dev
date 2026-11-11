# Commands

`[x]` is optional; `<x>` is a value you supply.

## Available today

| Command | What it does |
| --- | --- |
| `sentinel doctor` | Check that git, the GitHub CLI and ssh are installed on this machine, and that `gh` is signed in to github.com. |
| `sentinel doctor <destination>` | Check a server before installing on it: operating system, architecture, privileges, systemd, memory and Docker. It only reads: it installs and configures nothing on the server. |
| `sentinel version` | Print the version. |

- `<destination>` is what you would give to `ssh`: `user@host`, or a `Host` from your
  ssh config. `sentinel` runs your own `ssh` and never asks for a password; if `ssh`
  would ask for one, the check fails and says so.
- `--json` prints the report or the version as JSON.
- Exit status: `0` when every check passed, `1` when a check failed, `2` when the
  command line was wrong. A warning does not change the exit status.
- Each check that does not pass prints a `fix:` line.

## Planned

> **Planned.** None of the commands below exists yet, and the list may change. The
> phase is order, not a date; see the [roadmap](roadmap.md).

| Command | What it will do | Phase |
| --- | --- | --- |
| `sentinel setup <destination>` | Install the runtime on your server over ssh, from signed releases. It asks only for what the server lacks, and running it again is safe. | 1 |
| `sentinel login` | Sign in with GitHub, reusing the session of `gh`. | 1 |
| `sentinel run "<task>"` | Send a task with your working tree and start a run. | 1 |
| `sentinel runs` | List the runs. | 1 |
| `sentinel watch [run]` | Follow a run. | 1 |
| `sentinel pull [run]` | Fetch the branch of the pull request. | 1 |
| `sentinel cancel <run>` | Cancel a run. | 1 |
| `sentinel resume <run>` | Continue a paused run with a larger limit or another model. | 1 |
| `sentinel stop` | Halt every run on the server until you allow them again. | 1 |
| `sentinel update` | Update the client and the runtime. | 1 |
| `sentinel` | Start an interactive session, with plan, edit and auto modes. | 2 |
| `sentinel --from-claude-code` | Continue in Sentinel a session you started in Claude Code. | 2 |
