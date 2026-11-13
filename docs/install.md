# Installing the command

Releases are on the [releases page](https://github.com/Chisu-Technologies/sentinel-dev/releases).
With Go 1.26 or newer you can also build from source:
`go install github.com/Chisu-Technologies/sentinel-dev/cmd/sentinel@latest`.

## With the installer

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.ps1 | iex
```

What the installer does, and nothing else:

- It asks GitHub which release is the latest, then downloads the archive for your
  system and the `SHA256SUMS` of that release, over https.
- It refuses to install when the checksum of the archive is not the one `SHA256SUMS`
  lists.
- It writes one file, the `sentinel` binary: in `~/.local/bin` on Linux and macOS, in
  `%LOCALAPPDATA%\Programs\sentinel` on Windows. It asks for no password and never runs
  `sudo`.
- On Windows it adds that folder to your user `PATH`, so `sentinel` works in the same
  window and in new ones; it changes nothing else. On Linux and macOS it does not change
  your `PATH`; it tells you when the directory is not in it.
- Running it again replaces the binary with the latest release.

| Variable | What it does |
| --- | --- |
| `SENTINEL_VERSION` | Install this tag, such as `v0.1.0`, instead of the latest release. |
| `SENTINEL_INSTALL_DIR` | Put the binary in this directory. |
| `SENTINEL_DOWNLOAD_BASE` | Read releases from here: an https address that mirrors the release pages, or a directory that holds the files of one release. For a machine without access to GitHub. |
| `SENTINEL_REQUIRE_SIGNATURE` | Set to `1` to stop unless the signature was checked. |
| `SENTINEL_NO_MODIFY_PATH` | Set to `1` on Windows to leave your `PATH` as it is. |

## The checksum and the signature

They answer different questions. The checksum says the archive is the file the release
lists; it comes from the same place as the archive, so it does not say who built it.
The signature does: the release workflow signs `SHA256SUMS` with the identity of its own
run, through [Sigstore](https://www.sigstore.dev/), and no signing key is stored
anywhere.

The installer checks the signature when [cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
is installed, and prints whether it did. Without cosign it prints
`Signature not checked: cosign is not installed.` and goes on with the checksum alone.

To check a download by hand, from the directory that holds the three files:

```bash
cosign verify-blob --bundle SHA256SUMS.sigstore.json --certificate-identity-regexp '^https://github\.com/Chisu-Technologies/sentinel-dev/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' --certificate-oidc-issuer https://token.actions.githubusercontent.com SHA256SUMS
```

```bash
sha256sum --check --ignore-missing SHA256SUMS
```

## What a release holds

One archive per system, named without the version: `sentinel_linux_amd64.tar.gz`,
`sentinel_linux_arm64.tar.gz`, `sentinel_darwin_amd64.tar.gz`,
`sentinel_darwin_arm64.tar.gz`, `sentinel_windows_amd64.zip` and
`sentinel_windows_arm64.zip`. Each holds the binary beside `LICENSE`, `NOTICE` and
`THIRD_PARTY-NOTICES.md`. `SHA256SUMS` lists the six, and `SHA256SUMS.sigstore.json` is
its signature.

The archives are built by `scripts/build-release.sh`, on Linux, from a checkout with
nothing uncommitted; it refuses any other. The same commit gives the same bytes, so
anyone can build a tag and compare with `SHA256SUMS`.
