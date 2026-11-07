# Privacy and data flow

> **Planned.** This page describes what the planned program will send. If it and the
> code in this repository differ, the code wins. The hosted service is closed source,
> so this page covers only what the open program visibly sends.

The list of what leaves your machine is the README section
[What will leave your machine, and what will not](../README.md#what-will-leave-your-machine-and-what-will-not).
This page adds the details.

## Sign-in

You sign in with GitHub in your browser. The hosted service then issues a personal,
revocable token that works only with the plugin's interface. It lives in the operating
system keychain, not in the repository and not in a plain file.

## The plan

A plan has stages, sprints and tasks, each with a title, a description, acceptance
criteria and dependencies. All of it is sent, as is the status of each task.

## Fields of a runner report

| Field | Meaning |
| --- | --- |
| Plan and task identifiers | Which plan and task the run belongs to |
| Kind | Tests, app or browser (app and browser from Phase 2) |
| Commands | Each command, with its exit code and its duration |
| Image digest | The digest of the container image the commands ran in |
| Copied tree identifier | The identifier of the copy of your last commit that was used |
| Filtered output tail | The end of the output, after the secret filter |
| Hashes | Hashes of screenshots and of full logs, not the files themselves |
| Runner version, start and end | Version of the runner and the times of the run |
| `commit_sha`, lease identifier, idempotency key | Which commit, which reservation of the task, and a key that prevents double submission |

The tail is filtered for secrets before upload, on a best-effort basis. Your agent
receives only the end of the output; the full log stays on your machine. The hosted
service, not the runner, computes the verdict and assigns the report its identifier.
It requires at least one command and an image from Sentinel's list.

## Screenshots and full logs

They stay on your machine; the report carries only their hashes. A per-repository
setting to upload them is planned; its name and phase are not yet decided.

## Your agent's own provider

Sentinel does not change what your coding agent sends to its own model provider under
your account. Output from Sentinel's tools, such as a test tail, enters your agent's
context. The README statement about source files concerns what Sentinel uploads: your
agent reads your code to work on it, and what it sends to its provider is outside this
page.

## Not yet decided

| Question | Answer |
| --- | --- |
| What account data is stored | Not yet decided |
| How long data is kept | Not yet decided |
| How to delete your data or your account | Not yet decided |
| Who at Sentinel can access stored data | Not yet decided |
| Whether stored data is used to train models | Not yet decided |
| Telemetry beyond the categories in the README | Not yet decided |

The hosted service will record usage of the free plan. Until these points are
decided, do not assume any answer.

Report a privacy concern through the private channel in [SECURITY.md](../SECURITY.md).
