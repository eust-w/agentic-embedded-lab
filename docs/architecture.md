# Aether Desktop and AEL Engine architecture

The Go rewrite separates the foreground desktop UI, the durable background
agent, and external simulator workers.

```text
Wails / React Desktop
        |
  authenticated Unix socket
        |
      aetherd
        |
  +-----+------+---------+----------+
  |            |         |          |
Agent Core   Executor   Browser   AEL Engine
  |            |         |          |
Responses   Seatbelt   Chromium   Backend v2 JSONL
  |            |         |          |
Threads     Git/PTY    CDP/macOS   Renode/ngspice/Modelica/ns-3/openEMS
```

## Desktop and daemon

`cmd/aether-desktop` contains the Wails entrypoint. It exposes only generated
Wails bindings to the React UI. `cmd/aetherd` owns state, model calls, command
execution, agents, automations, plugins and simulation. The two processes use a
mode-0600 Unix socket and a capability token stored in macOS Keychain.

The daemon stores versioned Thread, Turn and Item events in SQLite WAL. Large
artifacts use a SHA-256 content-addressed store. A UI restart therefore does not
lose an active thread or rewrite immutable evidence.

## Agent execution and permissions

OpenAI Responses is the only model provider. Model output can request only
typed tools. The approval engine evaluates file roots, command actions, network,
external writes, destructive changes, sites and applications before dispatch.

Commands use argument vectors, never `sh -c`. On macOS, read-only and
workspace-write profiles are converted into Seatbelt profiles. Full access is
explicit and still asks for destructive or external effects. Git operations
accept explicit paths and refs and managed write agents use isolated worktrees.

## Extension plane

- `AGENTS.md` is layered from global through the active project subtree.
- Skills expose metadata first and load complete `SKILL.md` instructions only
  when selected.
- Hooks run concurrently at defined lifecycle events and may block a tool.
- Heavy plugins are signed out-of-process gRPC packages; lightweight policies
  and oracles use wazero/WASM.
- MCP clients support namespaced STDIO and HTTPS/loopback HTTP tools. OAuth and
  resource pagination remain an active migration gate.
- Memories are opt-in, scope-aware, locally stored and secret-redacted.

## Browser and Computer Use

The production app will bundle a pinned Chromium build. `internal/browser`
provides CDP navigation, DOM, screenshots and input behind persistent site
permissions. `internal/computeruse` checks per-bundle authorization plus macOS
Accessibility and Screen Recording permissions before posting native events.

The browser preview currently uses deterministic development content. It is not
evidence that Chromium has been bundled or that a website was exercised.

## AEL Engine

AEL v2 has new Go contracts for Problem, System, Experiment, Event, Evidence,
Claim and Validation Envelope. The deterministic scheduler sorts components,
advances explicit communication points, rejects missing backends and non-finite
metrics, evaluates named assertions and hashes the event trace.

External backends speak `ael.backend/v2` over JSON Lines. The executable is
administrator/container configured; experiments cannot supply an arbitrary
command. Simulator success remains model-dependent and cannot create a hardware
or production Claim without independent physical evidence.
