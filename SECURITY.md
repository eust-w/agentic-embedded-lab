# Security policy

## Supported versions

Agentic Embedded Lab is currently a Development Preview. Security fixes target
the latest commit on `main` and the latest published preview. No preview is a
hardware-equivalence or safety-certified release.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting for this repository. If the
private report form is unavailable, open a minimal public issue asking the
maintainers for a private contact channel **without including vulnerability
details**.

Do not put any of the following in a public issue, pull request, log, Evidence
Bundle, or model package:

- access tokens, API keys, passwords, or private certificates;
- internal hostnames, lab network topology, device identifiers, or credentials;
- exploitable payloads against a reachable deployment;
- sensitive firmware, datasheets, traces, or instrument captures that you do
  not have permission to disclose.

Include the affected revision, impact, reproduction preconditions, and a minimal
proof of concept in the private report. Maintainers will acknowledge the report,
triage severity and affected versions, coordinate a fix, and publish an advisory
when disclosure is safe.

## Security boundaries

- MCP and HTTP do not expose arbitrary shell, Renode Monitor, raw SCPI, host
  devices, or paths outside the configured workspace.
- Backend commands and OCI images are administrator-configured, not supplied by
  agents or experiments.
- Generated code runs through a network-off, rootless, read-only, resource-bound
  sandbox abstraction.
- Provider credentials come only from environment or secret injection and must
  never enter prompts, receipts, evidence, fixtures, logs, or the model registry.
- Lab Workers connect outbound using independent mTLS identities and expose only
  allow-listed board and instrument operations.
- An agent may promote a model only through `conformance_validated`. Hardware and
  production states require independent evidence and a human actor.

See [docs/security.md](docs/security.md) for the implementation threat model and
[docs/production-readiness.md](docs/production-readiness.md) for release gates.
