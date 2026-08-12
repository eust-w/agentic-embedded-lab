# Agent instructions

## Project boundary

AEL is a general embedded-systems lab. Do not introduce robotics-specific
concepts into core contracts. Robotics belongs in optional adapters or examples.

## Required checks

Before reporting a change as complete, run:

```bash
.venv/bin/ruff check .
.venv/bin/pytest
.venv/bin/ael doctor
```

For contract changes also regenerate and diff JSON schemas:

```bash
.venv/bin/ael schema export schemas/v1
```

## Evidence language

- A passing unit test proves code behavior only.
- A synthetic backend run does not prove a simulator or physical system.
- A simulator pass does not prove hardware behavior.
- Hardware equivalence is claim-scoped and valid only inside its signed
  Validation Envelope.
- Never silently replace a missing backend or model with a stub.
- An agent may promote generated models only through `conformance_validated`.
  Hardware and production states require hardware evidence and a human actor.

## Safety

- Resolve all user paths against the workspace root and reject escapes.
- Do not expose arbitrary shell, simulator monitor, SCPI, or host-device access
  through MCP or HTTP.
- Generated code runs only through the sandbox abstraction with networking off.
- Preserve unrelated user changes and never reset the worktree.
