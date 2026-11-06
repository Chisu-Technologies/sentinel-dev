# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Versioning

No version has been released. While the major version is zero (`0.y.z`), the project is
in initial development: anything may change at any time, and nothing should be
considered a stable interface. Once releases exist, only the latest `0.x` receives
security fixes; see [SECURITY.md](SECURITY.md).

## [Unreleased]

### Added

- The MIT license, in [LICENSE](LICENSE).
- A README with the project status and the statement of what will leave your machine
  and what will not.
- [CONTRIBUTING.md](CONTRIBUTING.md), which asks for a `Signed-off-by:` line on every
  commit (Developer Certificate of Origin 1.1).
- [SECURITY.md](SECURITY.md), which asks for private reports through GitHub Private
  Vulnerability Reporting.
- [THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md), which lists the components expected
  to be used, with the license of each. Nothing is redistributed yet.
- The `policy` workflow, with `scripts/policy-check.sh` and its self-test
  `scripts/policy-check_test.sh`. It requires a `LICENSE` file, caps tracked files at
  5 MiB, rejects `export-ignore`, `export-subst` and `filter` attributes, and, on pull
  requests, requires a `Signed-off-by:` line on every non-merge commit and checks the
  size of every blob the pull request adds.
- Code owner rules in `.github/CODEOWNERS`, and line-ending and binary-file rules in
  `.gitattributes`.
- Documentation of the planned design in `docs/`: an [index](docs/README.md),
  [getting started](docs/getting-started.md), [how it works](docs/how-it-works.md),
  [privacy](docs/privacy.md), the [security model](docs/security-model.md),
  [commands](docs/commands.md), [compatibility](docs/compatibility.md), a
  [FAQ](docs/faq.md), a [glossary](docs/glossary.md) that also serves as the
  translation term list, and a [roadmap](docs/roadmap.md).
- A Spanish translation of the README, [README.es.md](README.es.md).
- [AGENTS.md](AGENTS.md) with the rules for AI agents that edit this repository, and
  [CLAUDE.md](CLAUDE.md), which imports it.
- A pull request template and issue forms for documentation problems, proposals and
  questions.

No software has been released.
