# Security Policy

## Reporting a vulnerability

Do **not** open a public GitHub issue for security vulnerabilities.

Send a description to **security@wlow.dev** (or open a GitHub private vulnerability report if that is enabled). Include:

- Affected component and version/commit
- Steps to reproduce or a minimal proof of concept
- Potential impact

We aim to acknowledge within 48 hours and provide a fix timeline within 14 days for critical issues.

## Supported versions

Only the latest commit on `main` receives security patches. Older tags are not actively maintained.

## Scope

In scope: the orchestrator, wlow-runner, artifact store, Go SDK, Rust runner binary.
Out of scope: third-party infrastructure (NATS, Firecracker, OCI registries) — report those to their respective projects.
