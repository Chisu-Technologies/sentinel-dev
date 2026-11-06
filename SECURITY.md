# Security policy

## Reporting a vulnerability

Please do not open a public issue for a security problem. Report it privately with
GitHub Private Vulnerability Reporting: open the **Security** tab of this repository
and choose **Report a vulnerability**.

Include what you found, the steps to reproduce it and the version or commit you
tested. Please do not include real credentials or other people's data in the report.

You can write your report in English or in Spanish.

## What to expect

We will acknowledge your report within 14 days. We ask you to keep the details private
until a fix is published or we have agreed a date with you.

## Scope

This repository is pre-alpha and ships no released software yet.

In scope today:

- the repository's own workflows, scripts and configuration.

In scope once the first release is published:

- the local program and the plugin;
- the isolation of the containers in which the program runs your tests and your app;
- anything that sends data to the hosted service beyond what the README lists under
  [what will leave your machine](README.md#what-will-leave-your-machine-and-what-will-not).

## The hosted service

The hosted service is closed source and is not part of this repository. If you find a
vulnerability in it, report it through the same private channel described above.

## Not a vulnerability

- Code in your own repository doing what it is written to do. The program is designed
  to run your tests and your app on your own machine; escaping the container isolation
  it puts around them is in scope.
- A known vulnerability in a dependency, reported without evidence that the vulnerable
  code can be reached from this project.

## Supported versions

Nothing has been released yet. Once releases exist, only the latest 0.x release will be
supported, and fixes will land there.
