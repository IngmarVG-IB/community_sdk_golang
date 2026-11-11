# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub private vulnerability reporting on the upstream
repository: <https://github.com/kentik/community_sdk_golang/security/advisories/new>.

Do not open a public issue for a suspected vulnerability. Include the affected version, a description of the
issue and, if possible, a minimal reproduction. Never include Kentik API credentials in a report.

## Supported versions

| Version | Supported |
|---|---|
| Unreleased (`master`, modernisation line) | Yes |
| v0.2.x | No. Upgrade, see [docs/UPGRADING.md](docs/UPGRADING.md) |
| < v0.2 | No |

## Dependency scanning

`make vuln` runs `govulncheck ./...`; CI runs it on every push. The module has no known vulnerabilities at the
time of writing. Any finding that cannot be fixed by an upgrade must be justified here or in an ADR.
