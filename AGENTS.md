# Agent instructions

## Product boundary

Aether Desktop is a macOS-only local engineering agent. AEL Engine is a general
embedded-systems laboratory. Keep robotics-specific concepts in optional
adapters or examples, never in core contracts.

## Required checks

Before reporting a Go or desktop change as complete, run:

```bash
go fmt ./...
go vet ./...
go test ./...
npm --prefix frontend run lint
npm --prefix frontend run test
npm --prefix frontend run build
```

For contract changes also regenerate and diff schemas:

```bash
go run ./cmd/schema-export --output schemas/v2
```

## Evidence language

- Passing unit tests prove only the tested software behavior.
- Browser automation does not prove behavior on every website or browser build.
- Simulator success does not prove hardware behavior.
- Hardware equivalence is claim-scoped and valid only in a signed Validation
  Envelope.
- Missing tools, models, permissions, or hardware must block explicitly; never
  replace them silently with a mock.

## Safety

- Resolve paths against an explicit workspace root and reject escapes and
  symlink traversal.
- Model output may invoke only typed tools. Never expose arbitrary shell,
  simulator monitor, SCPI, database, or host-device access through MCP or UI.
- Commands run through the executor and permission engine with the narrowest
  possible profile.
- OpenAI credentials live only in macOS Keychain and never in config, logs,
  prompts, evidence, fixtures, or plugin manifests.
- Preserve unrelated user changes and never reset or discard a dirty worktree.
