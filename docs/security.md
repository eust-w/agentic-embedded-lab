# Security model

- MCP and HTTP accept only workspace-relative document paths.
- No API accepts arbitrary shell, Renode Monitor, SCPI, or device commands.
- Generated code execution must use `SandboxSpec`: rootless OCI, no network by
  default, read-only root and workspace, no capabilities, no host devices,
  bounded CPU, memory, PIDs, temporary storage, and time.
- If Podman or Docker is unavailable, generated code is not run on the host.
- Lab Workers connect outbound with mTLS and expose allow-listed high-level
  instrument operations. Envoy overwrites the verified SHA-256 peer fingerprint
  header; the control plane checks registration and an operator allowlist.
- Only independently validated and signed model packages may enter a shared
  production registry.

The sandbox and worker implementations are defense-in-depth, not proof against container
escape. Production images, seccomp profiles, SBOMs, signatures, dependency
review, mTLS deployment evidence and host hardening require a separate security
acceptance record. Unit tests cannot pass this production gate.
