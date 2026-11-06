# Privacy and data flow

Status: planned design, not implemented. Last reviewed: 2026-11-06.

> **Planned (Phase 1 and later).** Sentinel for developers is pre-alpha and nothing is
> installable yet. This page describes what the planned program will send. When this
> page and the code in this repository differ about what the program sends, the code
> wins. The hosted service is closed source and cannot be audited from here, so this
> page limits itself to what the open program visibly sends.

This page is the single place that describes the data flow. The
[README](../README.md) carries the short version; the two must agree, and the README
wording is the reference.

## What will leave your machine, and what will not

The text below is copied word for word from the README section of the same name.

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

## The five categories in more detail

1. **Repository name and remote.** Registered when you run `/sentinel:setup`. The
   program will remove any user or token the remote URL carries before sending it.
2. **The plan and the status of its tasks.** The plan has stages, sprints and tasks,
   each with a title, description, acceptance criteria and dependencies.
3. **Runner reports.** One report per run of the runner, uploaded by the runner when it
   finishes. See the field table below.
4. **The Brain projection.** Names of files and symbols and how they relate, not their
   contents. The index itself stays on your machine; only the projection is sent.
5. **Account sign-in.** You will sign in with GitHub in your browser. The hosted
   service will then issue a personal token that you can revoke and that works only
   with the plugin's interface. What the service stores about your account is
   described in "Account data" below.

### Fields of a runner report

| Field | Meaning |
|---|---|
| Plan and task identifiers | Which plan and task the run belongs to |
| Kind | Tests, app or browser (app and browser from Phase 2) |
| Commands | Each command, with its exit code and its duration |
| Image digest | The digest of the container image the commands ran in |
| Copied tree identifier | The identifier of the copy of your last commit that was used |
| Filtered output tail | The end of the output, after the secret filter |
| Hashes | Hashes of screenshots and of full logs, not the files themselves |
| Runner version, start and end | Version of the runner and the times of the run |
| `commit_sha`, lease identifier, idempotency key | Which commit, which reservation of the task, and a key that prevents double submission |

The hosted service, not the runner, will compute the verdict from the report, and it
will assign the report its identifier. It will require at least one command and an
image from Sentinel's list.

## Output tails and secret filtering

The tail of the output is whatever your commands print. A failing test or a stack trace
can include lines of your code. The program will filter secrets in the tail before it
is uploaded, and that filter will be best-effort, not a guarantee. The tool results that
come back to your agent will contain only the end of the output, and the full log will
stay on your machine.

## Screenshots and full application logs

They will stay on your machine. The report will carry only their hashes. A setting to
upload them for a given repository is planned, and its name and phase are not yet
decided.

## Credentials

- Your Sentinel token will live in the operating system keychain, not in the repository
  and not in a plain file.
- The program will not read, store or forward your Claude or ChatGPT session.
- Containers will receive no secrets other than test variables you declare for that
  repository.

## Your agent's own provider

Sentinel does not change what your coding agent sends to its own model provider under
your own account. Output that Sentinel's tools return, such as the tail of a test run,
enters your agent's context. This page does not describe your agent provider's
practices. The statement about source files above concerns what Sentinel uploads. It
does not mean that your code reaches no other service: your agent reads your code to
work on it, and what it then sends to its provider is outside this page.

## Account data

What the hosted service stores about your account, beyond what is listed above: not yet
decided.

## Retention, deletion, access and training

| Question | Answer |
|---|---|
| How long data is kept | Not yet decided |
| How you can delete your data or your account | Not yet decided |
| Who at Sentinel can access stored data | Not yet decided |
| Whether stored data is used to train models | Not yet decided |

Until these are decided, do not assume any particular answer.

## Usage records

The hosted service will record usage of the free plan. The details, including whether
and what telemetry the program sends beyond the categories above, are not yet decided.

## Reporting a privacy concern

Use the private channel described in [SECURITY.md](../SECURITY.md). A privacy problem
that is a vulnerability goes through the same route.

## Related pages

- [How it works](how-it-works.md)
- [The security model](security-model.md)
