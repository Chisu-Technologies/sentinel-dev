# Contributing

Thanks for helping. This project is pre-alpha, so this guide will grow.

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

## Testing your changes

For now the only check is `scripts/policy-check.sh`, which also runs in CI. It works
from any directory. `scripts/policy-check_test.sh` tests the check itself; run it with
`bash scripts/policy-check_test.sh`. This section will be completed as code lands.
