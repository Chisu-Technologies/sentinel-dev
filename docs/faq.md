# Frequently asked questions

Status: planned design, not implemented.

> **Planned.** Sentinel for developers is pre-alpha and there is nothing to install.
> The answers below describe the intended design in the future tense. Where something
> has not been decided, the answer says so instead of guessing.

## What is Sentinel for developers?

It is a plugin for your coding agent, a local program and a free account on the hosted
service. It will run your tests, start your app and drive a real browser against it,
then report what happened instead of what the agent claims happened. The results are
reported by your own machine. The answer on
[reported evidence](#what-is-reported-evidence) explains what that means and what it
does not.

See [How it works](how-it-works.md) for the three pieces.

## Is this open source?

Partly. The repository is licensed under the MIT license. The following will be
published under MIT:

- the plugin: commands, subagents and review rubrics in skill format;
- the local program: sign-in, the MCP server, the runner, the Brain indexer and the
  receipt verifier;
- the formats: the plan, the runner report, the Brain graph and the evidence.

The hosted service, the engine of the paid plans and the Heads are not published.
Nothing is released yet; today this repository holds documentation and the checks that
guard it. Third-party components are listed in
[THIRD_PARTY-NOTICES.md](../THIRD_PARTY-NOTICES.md).

## Do I need an account?

Yes. Plans, tasks, the Brain projection and the evidence are stored by the hosted
service, so without an account there is nothing to store them in. You will sign in
with GitHub in your browser, and the program will keep its token in the operating
system keychain.

## Do I need an API key?

No. Your own agent, Claude Code or Codex, does the thinking, with your own membership.
Sentinel will not pay for inference and will not ask for an API key. Usage by your
agent will count against your own plan with its provider, not against one from
Sentinel.

## Does my code leave my machine?

Some of what your commands print can contain code, so read the exact list instead of a
summary. This is the privacy section of the [README](../README.md), copied word for
word:

> Sent to Sentinel:
>
> - the repository name and remote, without any user or token the URL may carry;
> - the plan and the status of its tasks;
> - runner reports: command, exit code, duration, a truncated tail of the output and
>   digests. The tail is whatever your own commands print, so a test failure or a
>   stack trace can contain lines of code. Secret filtering is best-effort, not a
>   guarantee;
> - a projection of the Brain: the names of files and symbols and how they relate,
>   not their contents, up to 300 nodes and 64 KB.
>
> Sentinel never uploads your source files or your diffs, and it does not read your
> environment files or credentials; anything your commands print is subject to the
> best-effort filter above. Screenshots and full app logs stay on your machine unless
> you turn on uploading for that repository.

Separately, Sentinel will not change what your agent sends to its model provider under
your own account, and the output that Sentinel's tools return will enter your agent's
context. The [privacy page](privacy.md) is the single place for the full data flow.

## Do you read my Claude or ChatGPT session?

The README states it this way, word for word: "The program will never read, store or
forward your Claude or ChatGPT session, and containers will receive no secrets other
than test variables you declare for that repository."

## What does the hosted service keep about me?

The hosted service will record usage. Beyond what the previous answers list, the
following is not yet decided:

- what account data is stored;
- how long anything is retained, and how data or an account can be deleted;
- whether Sentinel staff can access stored data, and under what conditions;
- whether any data is used for training;
- whether telemetry is collected.

Nothing on this page should be read as a promise about these points. The hosted
service is closed source, so it cannot be audited from this repository.

## Does it cost anything?

The plan for developers is a free account. Beta access is by invitation only for now,
and usage limits will apply. The limits will be published at release; none are
published today.

## What do paid plans add?

The free plan works with you, and only while your agent session is open. The hosted
service also has paid plans, in which the work is meant to run without you being
present. The engine behind them is not part of this repository. This repository does
not describe their prices or terms.

## Why Docker?

The runner will run your tests, your app and a browser in containers on your own
machine, and project code is not meant to run outside a hardened container. Docker is
therefore a requirement. Docker Desktop has its own license terms; read the
[Docker Desktop license agreement](https://docs.docker.com/subscription/desktop-license/)
(checked 2026-11-06) for your situation. Whether free alternatives that speak the
Docker API work has not been tested, so none is claimed to work.

See [Security model](security-model.md) for the planned isolation, and
[Compatibility](compatibility.md) for systems and requirements.

## Is it related to Microsoft Sentinel?

No. Sentinel for developers is not related to Microsoft Sentinel.

## Can I use it offline?

Not in the design as it stands. The program will sign in through your browser, send
plans and reports to the hosted service, and your agent needs its own connection to
its model provider. The one piece meant to work without a connection is the receipt
verifier, which is planned for Phase 2 of the [roadmap](roadmap.md).

## What happens if my agent's usage allowance runs out mid-task?

The design is that a run across several tasks stops with a warning, the reservation on
the current task expires, and the task goes back to pending so it can be picked up
again later. The hosted service will not close a task without a runner report, so a
task interrupted this way will not be recorded as done.

## What is "reported" evidence?

Your machine runs the checks and reports the result to the hosted service. That
resists casual tampering, not a determined user, because the token lives on your
machine. The evidence is therefore labeled "reported", not "attested". Signed receipts
for reported evidence are planned for Phase 2.

## Will Sentinel push my code or open pull requests?

That is your decision. Work is planned to land in local branches named
`sentinel/<task>`, and pushing them and opening a pull request is up to you.

## Which agents and systems will it support?

The closed beta is planned for Claude Code only. Codex is planned after that, with
open points still to verify. Claude chat surfaces, Cowork, the ChatGPT chat and Cursor
are not supported. Operating systems and requirements are on the
[compatibility page](compatibility.md).

## How do I join the beta?

Beta access is by invitation only, and there is no way to request an invitation yet.
Access is controlled by the hosted service, not by who can see this repository.

## Where do I ask a question or report a problem?

Open an issue in this repository with the
[Question form](https://github.com/Chisu-Technologies/sentinel-dev/issues/new/choose);
GitHub Discussions are off for now. Report security problems privately, as described
in [SECURITY.md](../SECURITY.md), not in an issue.
