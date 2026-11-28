# Privacy and data flow

> **Planned.** This page describes what the planned software will send. If it and the
> code differ, the code wins.

## Today

`sentinel cancel`, `sentinel pull`, `sentinel resume`, `sentinel run`,
`sentinel stop`, `sentinel verify`, `sentinel runs` and `sentinel watch` are the
commands that contact a Sentinel server, the one in `SENTINEL_URL`. `sentinel run`
sends the task, the repository name, the commit GitHub already has, a patch of your
working tree and your API key, and reads back the state of the run; the patch holds
the files that `sentinel run --dry-run` lists. `sentinel verify` sends the repository
name, the pull request number and your API key, and reads back the state of the run.
`sentinel runs` and `sentinel watch` send your API key and a run id, and read back the
state and the recent log lines of your runs. `sentinel cancel` sends your API key and
a run id, and reads back the state of that run. `sentinel pull` sends your API key and
a run id, then runs `git fetch` against your repository's own remote.
`sentinel resume` sends your API key, a run id, the task the server already has for
that run, the limit you give and, with `--reply`, what you write there.
`sentinel stop` sends only your API key. Only
`sentinel run` reads or sends your working tree. `sentinel update` asks GitHub for the
latest release of the command and downloads it; it sends nothing about this machine or
its repositories. `sentinel up` downloads the images of Sentinel from the GitHub
Container Registry (`ghcr.io`) and runs them on this machine; the secrets it creates
stay in its folder on this machine. `sentinel down` connects to nothing.
`sentinel doctor <destination>` connects to the server you name, with your own `ssh`,
and to nothing else. `sentinel run --dry-run` reads your repository with git and
connects to nothing; it prints file names, not file contents.

## The open edition

Your code and your prompts go to three places, all chosen by you:

| Where | What |
| --- | --- |
| Your server | The task, a patch of your working tree, the run and its logs |
| GitHub | The branch and the pull request, with the credential you gave the server |
| Your model provider | The prompts and the code the agent reads, with your own key |

The Sentinel service is not on that path. It will receive two things:

- a sign-in with GitHub, reusing the session of `gh`;
- usage counts: how many runs, how long they took, which model and provider they
  used, how they ended, and the version and operating system of the client.

It will not receive prompts, code, diffs or logs.

## The paid editions

In the hosted edition the server is run by Sentinel, so the task, the code and the
logs are on a machine Sentinel operates. That edition is not in this repository, and
its terms will be published with it.

## Not yet decided

| Question | Answer |
| --- | --- |
| The exact list of usage counts | Not yet decided |
| Whether a repository is identified in them, and how | Not yet decided |
| How long they are kept | Not yet decided |
| How to delete your account and its data | Not yet decided |
| Who at Sentinel can access them | Not yet decided |
| Whether you can switch them off | Not yet decided |

Until these points are decided, do not assume any answer. The list will be final
before the first release, and what is not on it will not be sent.

Report a privacy concern through the private channel in [SECURITY.md](../SECURITY.md).
