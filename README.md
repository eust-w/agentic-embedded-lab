<div align="center">
  <img src="docs/design/aether-desktop-1.0/chat-workspace.png" alt="Aether Desktop coding workspace" width="100%" />
  <h1>Aether Desktop + AEL Engine</h1>
  <p><b>A macOS-native engineering agent and evidence-driven embedded laboratory.</b></p>
  <p>
    <img alt="Go" src="https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white">
    <img alt="Wails" src="https://img.shields.io/badge/Wails-v2.12-DF0000?style=flat-square">
    <img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=111">
    <img alt="Foundation" src="https://img.shields.io/badge/Go%20foundation-passing-20b26b?style=flat-square">
    <img alt="AEL simulation" src="https://img.shields.io/badge/AEL%20simulation-migration%20in%20progress-f59e0b?style=flat-square">
    <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-f59e0b?style=flat-square"></a>
  </p>
  <p><b>English</b> | <a href="README.zh-CN.md">简体中文</a></p>
</div>

> [!IMPORTANT]
> This branch is the active **Go rewrite toward Aether Desktop 1.0**. The Go
> foundation and React production build pass, but the five simulator backends,
> 24 mechanism benchmarks, bundled Chromium, LaunchAgent installation, signing,
> and notarization are not complete. No simulator or hardware Claim is implied.

## Product split

- **Aether Desktop** is the macOS coding-agent experience: threads, approvals,
  terminal, Git/worktrees, subagents, skills, plugins, MCP, memory, browser,
  Computer Use, and background automations.
- **AEL Engine** is the general embedded laboratory: deterministic scheduling,
  virtual hardware, multi-physics backends, evidence, fidelity, claims, and
  validation envelopes.

## Implemented in the Go rewrite

- Wails v2 + React/TypeScript/Vite desktop shell with Chat, Diff, Browser and
  Simulation states matching the checked-in visual specification.
- Versioned `aether.desktop/v1` Thread, Turn, Item, Approval, Agent, Worktree,
  Automation and plugin contracts.
- SQLite WAL event store, content-addressed artifacts, crash-persistent threads,
  memories, permissions, automations and jobs.
- OpenAI Responses streaming client with cancellation, idempotency, structured
  errors and retry/backoff; default model is configurable and starts at `gpt-5.6`.
- macOS Keychain-backed OpenAI credentials and daemon capability token.
- Authenticated Unix-socket `aetherd`, plus Wails-only bindings for the UI.
- Typed permission engine and macOS Seatbelt command preparation; no model-built
  shell command is exposed.
- Git status/diff/stage/restore/commit/push primitives and managed worktrees with
  dirty tracked-patch transfer.
- Hierarchical `AGENTS.md`, skills, concurrent lifecycle hooks, Ed25519-signed
  plugins, wazero/WASM, MCP STDIO/HTTP and local redacted memory.
- Independent child-agent threads, persistent RRULE automation, controlled CDP
  browser APIs and per-application Computer Use permissions.
- Initial AEL v2 contracts, deterministic scheduler, evidence trace hashes,
  explicit Fidelity aggregation and the Go backend JSONL protocol.

## Not yet accepted

- Renode, ngspice, OpenModelica/OMSimulator, ns-3 and openEMS Go workers are not
  all wired into the container images.
- The existing 24 faulty/fixed benchmarks have not all passed through the Go runner.
- Bundled Chromium and Chrome Native Messaging distribution are not installed.
- ScreenCaptureKit screenshot capture and Accessibility element-tree inspection
  are not complete; input primitives and permission checks exist.
- `SMAppService`, Sparkle updates, Developer ID signing and Apple notarization
  require the production packaging phase and full Xcode.
- Hardware validation remains unavailable without the physical laboratory.

## Development

Required local versions: Go 1.24.x and Node 22 LTS. The current machine can
compile the Go binaries with Command Line Tools; a complete signed `.app` needs
full Xcode.

```bash
npm --prefix frontend install
npm --prefix frontend run build
go test ./...
go vet ./...
go run ./cmd/schema-export --output schemas/v2
```

Run the frontend visual shell:

```bash
npm --prefix frontend run dev -- --host 127.0.0.1
```

The Wails CLI is pinned to v2.12.0. The app entrypoint is
`cmd/aether-desktop`; the background process is `cmd/aetherd`.

## Evidence boundary

The design intentionally keeps these states separate:

| State | Meaning |
|---|---|
| Software tested | The named Go/React behavior passed its test |
| Simulation validated | A named tool-executed mechanism passed within recorded fidelity |
| Hardware validated | Physical differential evidence passed inside a signed envelope |
| Production approved | Hardware evidence, policy and an independent human approval are present |

The current branch proves only the checked software foundation. See
[the visual specification](docs/design/aether-desktop-1.0/DESIGN.md) and
[the archived Python implementation](https://github.com/eust-w/agentic-embedded-lab/tree/archive/python-aether-pre-go).

## License

The core is licensed under [Apache-2.0](LICENSE). External simulator containers
retain their upstream license obligations.
