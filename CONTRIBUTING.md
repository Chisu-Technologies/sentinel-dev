# Contributing

The project is pre-alpha; today this repository holds the documentation and the policy
check.

## Before you start

- Open an issue before a feature, a new document or a behavior change, and wait for
  agreement. A pull request without one may be closed unreviewed. Typos and broken
  links need no issue.
- The hosted service is closed source and out of scope here.
- Write in English, or in Spanish if that is easier for you. Ask questions with the
  Question issue form.
- Security problems go through [SECURITY.md](SECURITY.md), not an issue.

## Sign your commits (DCO)

Every commit needs a `Signed-off-by:` line, added with `git commit -s`. It certifies
the [Developer Certificate of Origin 1.1](https://developercertificate.org/). The
`policy` check rejects a pull request with an unsigned commit that is not a merge.

## Commits and pull requests

- Short imperative subject with a prefix such as `fix:`, `feat:`, `docs:` or `ci:`;
  the body explains why.
- Code or text from other projects only under a license compatible with MIT, with the
  source stated in the pull request.
- AI tools are welcome. Say in the pull request which part they wrote. The person who
  signs off must understand every change and be able to explain it. A pull request
  opened by an autonomous agent, with nobody answering for it, is closed.

## Documentation

- Describe planned features as planned, never as working.
- No prices, quotas, dates or promises about the hosted service.
- Changes to what leaves a user's machine, retention, deletion, account data, training
  or telemetry need an issue first. Where something is not decided, write "not yet
  decided".
- No internal details: hostnames, providers, private repositories, ticket numbers,
  internal paths, secrets or personal email addresses.
- A translation names the English file and commit it comes from, and the English text
  prevails.

## Checks

```bash
bash scripts/policy-check_test.sh
bash scripts/policy-check.sh
```

Both run in CI as the `policy` workflow. The maintainer merges only after it passes.

Taking part in this project means following the [code of conduct](CODE_OF_CONDUCT.md).
