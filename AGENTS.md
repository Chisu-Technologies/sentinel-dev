# Instructions for agents

These rules apply to any AI agent that edits this repository. The rules for people are
in [CONTRIBUTING.md](CONTRIBUTING.md); an agent follows those too.

## Status

Sentinel for developers is pre-alpha. Nothing is released and nothing is installable.
The hosted service is closed source and is not in this repository.

## Check your work

Run these from any directory. Both also run in CI as the `policy` workflow.

```bash
bash scripts/policy-check_test.sh
bash scripts/policy-check.sh
```

## Commits and pull requests

- Every commit needs a `Signed-off-by:` line, added with `git commit -s`. The sign-off
  is a certification by a person (see [CONTRIBUTING.md](CONTRIBUTING.md)), so only the
  human who answers for the change adds it.
- Commit subjects are short and imperative, with a prefix such as `fix:`, `feat:`,
  `docs:` or `ci:`. The body explains why, not what.
- Open an issue before a feature or behavior change, and wait for agreement.
- Say in the pull request that an agent helped. A pull request with nobody answering
  for it is closed.
- Do not copy code or text from other projects unless their license is compatible with
  MIT and the pull request states where it came from.

## Writing rules for documentation

- English, Markdown, prose wrapped at 88 columns, sentence-case headings, one H1 per
  file, relative links between files of this repository.
- Anything not released is written in the future tense and carries a marker such as
  `> **Planned (Phase N).** ...` or `> **Design intent.** ...`. Phase N means order, not
  a date. Do not describe a planned feature as if it worked.
- A command that does not work yet goes directly under the line "Planned. This command
  does not work yet." It is either a command of this project's own planned design or
  copied verbatim from official Claude Code or Codex documentation, with the
  documentation address and the check date next to it. Do not invent commands.
- The README section "What will leave your machine, and what will not" is negotiated
  text. Do not rewrite, weaken, broaden or paraphrase it. Quote it or link to it.
- Where retention, deletion, account data, staff access, training use or telemetry
  come up, write "not yet decided".
- No prices, quotas, release dates or exit criteria. No marketing adjectives. Avoid
  absolutes such as "never", "completely" or "secure" unless quoting the privacy text.
- Product names of other companies appear in text only, never as logos and never as
  part of this product's name. Do not suggest endorsement.
- Sentinel for developers has no relation to Microsoft Sentinel. Do not blur the two.

## What must never appear in this repository

- hostnames, internal addresses and internal URLs;
- infrastructure or hosting providers;
- names of private repositories;
- internal ticket or pull request numbers;
- internal file paths and line numbers;
- links to internal documents or artifacts;
- internal decision dates;
- commit hashes from private trees;
- real email addresses;
- secrets of any kind.

If you need a detail that is not in this repository, say it is not yet decided. Do not
guess it.
