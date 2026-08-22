# Aether Native

Aether Native is an **experimental local companion UI and plugin-runtime
prototype** for Agentic Embedded Lab. It demonstrates a desktop agent shell,
plugin registry, working memory, streamed reasoning events, diff review, and a
development-only plugin evolution harness.

It is not part of the AEL `production` gate and must not be used to claim
hardware equivalence or production-approved self-evolution.

## Run the local server

Install the repository with the existing server dependencies, then bind to the
loopback interface:

```bash
python3.12 -m venv .venv
. .venv/bin/activate
python -m pip install -e '.[server]'
python -m aether --server-only --host 127.0.0.1 --port 8765
```

Open <http://127.0.0.1:8765>.

Provider credentials are read only from environment variables:

```bash
export DEEPSEEK_API_KEY='...'
# or ANTHROPIC_API_KEY / OPENAI_API_KEY
```

No provider key is stored in `~/.aether/config.json`, returned by the config
API, or embedded in the browser assets.

## Desktop shells

- Electron development shell: run `npm ci` and `npm start` in
  `aether/desktop/`.
- Native macOS bundle helper: `scripts/build_mac_app.sh` (requires a prepared
  `.venv` with the desktop dependencies).

`node_modules/`, desktop build products, Python caches, and `.aether/` runtime
state are intentionally excluded from Git.

## Security boundary

- The server accepts loopback binding by default. Remote binding requires an
  explicit `AETHER_ALLOW_REMOTE=1` and must sit behind an authenticated proxy.
- The terminal endpoint and command plugin expose only fixed read-only
  diagnostics; arbitrary shell commands are rejected.
- Project, task, and session identifiers are validated before becoming paths.
- In-process plugin evolution is disabled in the server and agent defaults.
  The current harness can be enabled only programmatically for isolated
  development tests; it is not an OCI security boundary.
- Generated plugins cannot promote AEL hardware or production model states.

For the production security model, see [the repository security policy](../SECURITY.md)
and [AEL security design](../docs/security.md).
