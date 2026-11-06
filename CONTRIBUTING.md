# Contributing

Thanks for helping. This project is pre-alpha, so this guide will grow.

## Before you start

- **Open an issue first.** Please do not send a pull request for a new feature, a new
  document or a behavior change that nobody has agreed to. Open an issue, describe the
  problem, and wait for a reply. A pull request without a prior agreement may be closed
  without review. Small fixes, such as a typo or a broken link, do not need an issue.
- **Know what lives here.** This repository will hold the plugin, the local program and
  the published formats. Today it holds the documentation and the policy check; nothing
  is released yet. The hosted service is closed source and is not in scope for
  contributions here; the README says what is open and what is not.
- **Language.** Write issues and pull requests in English. We also read Spanish, so
  write in Spanish if that is easier for you; an English summary helps other readers.
- **Questions.** GitHub Discussions are off for now. Ask a question with the Question
  issue form.
- **Security problems.** Do not use an issue. See [SECURITY.md](SECURITY.md).

## Sign your commits (DCO)

Every commit must carry a `Signed-off-by:` line. Add it with `git commit -s`. By signing
off you certify the Developer Certificate of Origin 1.1:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the license(s) involved.
```

The `policy` workflow rejects a pull request if any commit in it that is not a merge
commit lacks a `Signed-off-by:` line. It checks that the line is present, not who
signed. Whoever signs still certifies the DCO above.

## Commit style

Short imperative subject (about 70 characters), a prefix such as `fix:`, `feat:`,
`docs:` or `ci:`, and a body that explains why the change is needed rather than what
the diff already shows.

## What we cannot accept

We do not accept contributions that copy code or text from other projects unless their
license is compatible with this repository's MIT license and the origin is stated in the
pull request. If you are not sure, ask first.

## AI-assisted contributions

You may use AI tools to write code, text or tests. The rules are the same as for any
contribution:

- Tell us in the pull request that you used an AI tool, and for which part.
- A human must understand every change in the pull request and be able to explain it
  in their own words when asked. The person who signs off the commits is that human.
- A pull request opened by an autonomous agent, with nobody answering for it, is closed
  without review.

## Documentation changes

Documentation is held to the same bar as code.

- Anything that is not released yet is written in the future tense and is marked as
  planned. Do not describe a planned feature as if it worked.
- Do not add prices, quotas, dates or promises about the hosted service.
- Sentences about what leaves a user's machine, retention, deletion, account data,
  training use or telemetry need review by the owner. Open an issue before changing
  them. Where a fact is not decided, write "not yet decided".
- Do not add internal details: hostnames and internal addresses, names of hosting or
  infrastructure providers, private repository names, internal ticket or pull request
  numbers, internal file paths, secrets, or real email addresses of other people.
- Link to other files in this repository with relative links, and link to official
  vendor documentation instead of restating vendor terms.

## Translations

English is the source of truth. A translated file says which English file and commit it
was made from, and if the two differ, the English one wins. Please open an issue before
you start a translation, because a translation that drifts from the English text is
worse than none, especially where it describes what data leaves a machine.

## Licensing of contributions

This repository is licensed under MIT; see [LICENSE](LICENSE). By signing off a commit
you certify the DCO above for your contribution. Nothing in this repository promises
anything about how the hosted service may use contributions.

## How decisions are made

This project has one maintainer, who decides what is merged. Pull requests are merged
only after the `policy` check passes.

## Testing your changes

For now the only check is `scripts/policy-check.sh`, which also runs in CI. It works
from any directory. `scripts/policy-check_test.sh` tests the check itself; run it with
`bash scripts/policy-check_test.sh`. This section will be completed as code lands.
