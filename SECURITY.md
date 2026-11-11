# Security policy

## Reporting a vulnerability

Do not open a public issue. Use GitHub Private Vulnerability Reporting: open the
**Security** tab of this repository and choose **Report a vulnerability**. Include
what you found, how to reproduce it and the version or commit you tested, without real
credentials or other people's data. English or Spanish is fine.

We acknowledge reports within 14 days. Please keep the details private until a fix is
published or we agree on a date with you.

## Scope

Today the scope is `sentinel`, the command-line client in this repository, and the
workflows, scripts and configuration of the repository. From the first release it also
covers:

- the runtime that `sentinel setup` installs on a server;
- the isolation of the sandbox each task runs in;
- the gateway that holds the model keys and the GitHub credential;
- anything sent to the Sentinel service beyond what the
  [privacy page](docs/privacy.md) lists.

The paid editions are not in this repository; report their vulnerabilities through the
same channel.

Not a vulnerability: code in your own repository doing what it is written to do, or
a known issue in a dependency without evidence that this project reaches the
vulnerable code. Escaping the sandbox is in scope, and so is making the agent merge.

## Supported versions

Once releases exist, only the latest 0.x release receives fixes.
