# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:
[Report a vulnerability](https://github.com/AmitwalaH/doriki/security/advisories/new) (the
"Security" tab, then "Report a vulnerability"). Do not open a public issue.

## Supported versions

doriki has no release yet. Once v0.1.0 ships, fixes go to the latest release only.

## What is in scope

doriki is read-only against the cluster and makes no network calls other than to the Kubernetes
API. Reports we especially want:

- A secret, token or credential that reaches a `doriki capture` bundle, a report or a log.
- Any code path that sends a mutating request to the Kubernetes API.
- Any network call other than the Kubernetes API.
