# Contributing

The project is pre-alpha. This repository holds `sentinel`, the command-line client,
and the documentation of the planned design.

## Before you start

- Open an issue before a feature, a new document or a behavior change, and wait for
  agreement. A pull request without one may be closed unreviewed. Typos and broken
  links need no issue.
- The paid editions are not in this repository and are out of scope here.
- Write in English, or in Spanish if that is easier for you. Ask questions with the
  Question issue form.
- Security problems go through [SECURITY.md](SECURITY.md), not an issue.

## Sign your commits (DCO)

Every commit needs a `Signed-off-by:` line, added with `git commit -s`. It certifies
the [Developer Certificate of Origin 1.1](https://developercertificate.org/). The
`policy` check rejects a pull request with an unsigned commit that is not a merge.
Contributions are licensed under Apache-2.0.

## Commits and pull requests

- Short imperative subject with a prefix such as `fix:`, `feat:`, `docs:` or `ci:`;
  the body explains why.
- Code or text from other projects only under a license compatible with Apache-2.0,
  with the source stated in the pull request.
- AI tools are welcome. Say in the pull request which part they wrote. The person who
  signs off must understand every change and be able to explain it. A pull request
  opened by an autonomous agent, with nobody answering for it, is closed.

## Code

- Go 1.26 or newer. The client uses the standard library only; a new dependency needs
  an issue first.
- A change of behavior comes with a test that fails without it.
- External programs run through `internal/execx`, so tests need no network and no
  installed tools.

## Documentation

- Do not describe a planned feature as if it worked.
- No prices, quotas or dates.
- Changes to what leaves a user machine, retention, deletion, account data, training
  or telemetry need an issue first. Where something is not decided, write "not yet
  decided".
- No internal details: hostnames, providers, private repositories, ticket numbers,
  internal paths, secrets or personal email addresses.
- Open an issue before starting a translation. A translation names the English file
  and commit it comes from, and the English text prevails.

## Checks

```bash
gofmt -l .
go vet ./...
go test ./...
bash scripts/policy-check_test.sh
bash scripts/policy-check.sh
```

CI runs them as two workflows. `ci` formats, vets, tests and builds on Linux, macOS
and Windows. `policy` checks for a `LICENSE` file, a 5 MiB cap on tracked files and on
every blob a pull request adds, no `export-ignore`, `export-subst` or `filter`
attributes, and a sign-off on every non-merge commit. The maintainer merges only after
both pass.

Taking part in this project means following the [code of conduct](CODE_OF_CONDUCT.md).
