# Instructions for agents

These rules apply to any AI agent that edits this repository, on top of
[CONTRIBUTING.md](CONTRIBUTING.md).

## Check your work

```bash
gofmt -l .
go vet ./...
go test ./...
bash scripts/policy-check_test.sh
bash scripts/policy-check.sh
```

`gofmt -l .` must print nothing. All of it runs in CI, as the `ci` and `policy`
workflows.

## Code

- Go, standard library first. A new dependency needs an issue, a license compatible
  with Apache-2.0 and an entry in [THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).
- External programs (`ssh`, `git`, `gh`) run through `internal/execx`, so every
  command is tested without them. User input never reaches a shell, and never reaches
  a program as something it could read as an option.
- A change of behavior comes with a test that fails without it.
- Comments explain why, not what.
- Secrets, tokens and the output of commands that print them never go into a report,
  a log or an error message.

## Commits and pull requests

- Only the human who answers for the change adds the `Signed-off-by:` line
  (`git commit -s`): it is a personal certification.
- Short imperative subject with a prefix such as `fix:`, `feat:`, `docs:` or `ci:`;
  the body explains why, not what.
- Open an issue before a feature or behavior change, and wait for agreement.
- Say in the pull request that an agent helped.
- Do not copy code or text from other projects unless their license is compatible
  with Apache-2.0 and the pull request states the source.

## Writing documentation

- Be brief. Say a thing once, in the page that owns it, and link to it elsewhere.
- English Markdown, prose wrapped at 88 columns, sentence-case headings, one H1 per
  file, relative links between files of this repository.
- A page about unreleased work starts with a one-line `> **Planned.**` or
  `> **Design intent.**` banner; under it the design may be described in the present
  tense. Elsewhere, unreleased work is written in the future tense. Do not describe a
  planned feature as if it worked. "Phase N" means order, not a date.
- Do not invent commands. The commands that exist are the ones `sentinel help` prints;
  [docs/commands.md](docs/commands.md) keeps the planned ones apart.
- The README section "What leaves your machine" and
  [docs/privacy.md](docs/privacy.md) are commitments. Do not weaken or broaden them
  without an issue; what is undecided stays "not yet decided".
- No prices, quotas, release dates or marketing adjectives. Avoid absolutes such as
  "never", "completely" or "secure" unless the code enforces them and a test shows it.
- Other companies and their products appear as plain text only, never as logos or as
  part of the name of this product, and never suggest endorsement. Do not compare
  Sentinel with other products here. This product has no relation to Microsoft
  Sentinel.

## Never in this repository

Hostnames, internal addresses and internal URLs; infrastructure or hosting providers;
names of private repositories; internal ticket or pull request numbers; internal file
paths and line numbers; links to internal documents; internal decision dates; commit
hashes from private trees; personal email addresses (the public contact is
info@chisu.io); secrets of any kind. If a detail is not in this repository, write that
it is not yet decided instead of guessing.
