# Security policy

## Reporting a vulnerability

Do not open a public issue. Use GitHub Private Vulnerability Reporting: open the
**Security** tab of this repository and choose **Report a vulnerability**. Include
what you found, how to reproduce it and the version or commit you tested, without real
credentials or other people's data. English or Spanish is fine.

We acknowledge reports within 14 days. Please keep the details private until a fix is
published or we agree on a date with you.

## Scope

Nothing is released yet. Today the scope is this repository's workflows, scripts and
configuration. From the first release it also covers:

- the local program and the plugin;
- the isolation of the containers that run your tests and your app;
- anything sent to the hosted service beyond the README list of
  [what will leave your machine](README.md#what-will-leave-your-machine-and-what-will-not).

The hosted service is closed source and not in this repository; report its
vulnerabilities through the same channel.

Not a vulnerability: code in your own repository doing what it is written to do, or
a known issue in a dependency without evidence that this project reaches the
vulnerable code. Escaping the container isolation around your code is in scope.

## Supported versions

Once releases exist, only the latest 0.x release receives fixes.
