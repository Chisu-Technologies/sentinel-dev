# Instructions for agents

These rules apply to any AI agent that edits this repository, on top of
[CONTRIBUTING.md](CONTRIBUTING.md).

## Check your work

```bash
bash scripts/policy-check_test.sh
bash scripts/policy-check.sh
```

Both run in CI as the `policy` workflow, and work from any directory.

## Commits and pull requests

- Only the human who answers for the change adds the `Signed-off-by:` line
  (`git commit -s`): it is a personal certification.
- Short imperative subject with a prefix such as `fix:`, `feat:`, `docs:` or `ci:`;
  the body explains why, not what.
- Open an issue before a feature or behavior change, and wait for agreement.
- Say in the pull request that an agent helped.
- Do not copy code or text from other projects unless their license is compatible
  with MIT and the pull request states the source.

## Writing documentation

- Be brief. Say a thing once, in the page that owns it, and link to it elsewhere.
- English Markdown, prose wrapped at 88 columns, sentence-case headings, one H1 per
  file, relative links between files of this repository.
- A page about unreleased work starts with a one-line `> **Planned.**` or
  `> **Design intent.**` banner; under it the design may be described in the present
  tense. Elsewhere, unreleased work is written in the future tense. Do not describe a
  planned feature as if it worked. "Phase N" means order, not a date.
- Do not invent commands. A Claude Code or Codex command is copied verbatim from the
  official documentation, with its address and the check date.
- The README section "What will leave your machine, and what will not" is negotiated
  text. Do not rewrite, weaken, broaden or paraphrase it; link to it.
- Retention, deletion, account data, staff access, training use and telemetry are
  "not yet decided".
- No prices, quotas, release dates, exit criteria or marketing adjectives. Avoid
  absolutes such as "never", "completely" or "secure" unless quoting the privacy text.
- Other companies' product names appear as text only, never as logos or as part of
  this product's name, and never suggest endorsement. This product has no relation to
  Microsoft Sentinel.

## Never in this repository

Hostnames, internal addresses and internal URLs; infrastructure or hosting providers;
names of private repositories; internal ticket or pull request numbers; internal file
paths and line numbers; links to internal documents; internal decision dates; commit
hashes from private trees; personal email addresses (the public contact is
info@chisu.io); secrets of any kind. If a detail is not in this repository, write that
it is not yet decided instead of guessing.
