# Security policy

## Reporting a vulnerability

Use GitHub's private vulnerability reporting:
**[Report a vulnerability](https://github.com/rowhitswami/flakestat/security/advisories/new)**.
It is enabled on this repository, so the report stays private until there is a
fix. Please do not open a public issue for anything exploitable.

Include the version (`flakestat version`), the platform, and the smallest input
or command that reproduces it. If you have a patch, a private fork attached to
the advisory is welcome.

Expect an acknowledgement within a few days. This is a single-maintainer
project, so nothing faster is promised, and saying so is more useful than
publishing an SLA nobody meets. You will be credited in the advisory and the
changelog unless you prefer otherwise.

## Supported versions

The latest release only. Pre-1.0 means no backports; fixes go out as a new
patch release.

## What is in scope

The threat model is worth stating, because it decides what counts.

**flakestat parses JUnit XML that it does not control.** Reports arrive from
CI artifacts, other people's shards, and sometimes other people's pull
requests. Anything in a report that leads to code execution, a file being read
or written outside the state directory, a network request, or unbounded memory
or CPU is in scope. Deeply nested and entity-expanded inputs are rejected by a
depth guard; a way past it is a finding.

**The installers download and execute a binary.** `scripts/install.sh`, the
npm `postinstall`, and the PyPI wrapper each fetch a release archive and verify
it against `checksums.txt`. They fail closed. A way to make any of them install
an unverified or substituted binary without `FLAKESTAT_SKIP_CHECKSUM=1` being
set is in scope, as is any archive-extraction path escape.

**flakestat runs the command you give it.** `hunt` executes it directly with
`exec.Command` and never through a shell. A path from report contents, config
file, or dimension values to command execution is in scope.

**It records a whitelist of CI variables.** Six providers, eighteen named
environment variables, and a fixed list of recognised JUnit `<property>` names.
The process environment is never enumerated. Anything that causes an
unwhitelisted variable or an arbitrary property to be written into the history
is in scope, because that is how a token ends up in a committed file.

**The GitHub Action.** Anything that lets a pull request from a fork influence
what runs in the workflow, or leak the token.

## What is not in scope

- A test command you deliberately pass to `hunt`. That is the feature.
- Reading or writing inside `.flakestat/`, which is the tool's own state.
- Anything requiring `FLAKESTAT_SKIP_CHECKSUM=1`, which exists to be an
  explicit, documented choice.
- Denial of service from a report you supplied to your own local run.
- Vulnerabilities in the Go standard library with no reachable path here.
  `govulncheck` is run against the tree; reachability is the bar.

## Verifying a release

Release binaries carry build provenance from v0.2.1 onward:

```sh
gh attestation verify flakestat_0.2.1_linux_amd64.tar.gz -R rowhitswami/flakestat
```

Every release also ships `checksums.txt`, which the installers verify
automatically. npm packages are published with provenance, and PyPI uploads use
trusted publishing over OIDC, so neither has a long-lived token to leak.
