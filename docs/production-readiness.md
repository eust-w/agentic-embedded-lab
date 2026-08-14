# Production 1.0 readiness

The repository version is `0.2.0.dev0 Development Preview`. “Production 1.0” is
a gated target, not the current release state.

| Gate | Current evidence | State |
|---|---|---|
| Strict contracts, path safety, CLI, evidence, SQLite/CAS | local automated tests | implemented |
| MCP and `/v1` HTTP thin adapters | import/API tests | implemented |
| Process/container adapters and deterministic scheduler | unit tests and qualified local Linux-container execution | implemented |
| 24 faulty/fixed mechanism contracts | qualified local full-suite evidence | `simulation` acceptance supported without CI dependency |
| ARM/RISC-V Zephyr toolchains | pinned Zephyr 4.4.2 and SDK 1.0.1 image | implemented for software evidence |
| Five backend images | inspected Linux image identities and pinned probes | implemented |
| FMI 2.0 proxies and five-domain SSP | local Linux-FMU/OMSimulator acceptance | functional exchange only; no hardware claim |
| PostgreSQL/S3, leases, mTLS, OIDC | local Compose recovery acceptance | `software` acceptance supported without CI dependency |
| Five board/instrument Lab Worker | allow-listed unverified drivers | no real Lab evidence; blocked |
| License/security/recovery approval | SBOM/review workflows defined | blocked pending independent approval |

## Verification snapshot (2026-08-13)

The local software acceptance proves the named mechanisms only. Renode 1.16.1,
ngspice 46, OpenModelica 1.27.0/OMSimulator 2.1.3, ns-3 3.47 and openEMS
0.0.36 execute in inspected Linux containers through Colima. The acceptance
also records Zephyr 4.4.2/SDK 1.0.1 builds, all 24 faulty/fixed pairs, the
five-backend causal chain, non-zero unit-bearing FMI/SSP exchange, 20 identical
quantized traces, and the Compose recovery checks.

Neither GitHub Actions nor a specific hosted runner is a trust root. The
`qualified-execution-evidence` policy accepts the same complete evidence from a
controlled native Linux host or inspected Linux OCI images. CI remains useful
for independent repetition, but an unavailable CI service cannot block a valid
local software claim or strengthen it merely by running the same commands.

## Release rule

Development Preview versions may be tagged only after the `software` gate passes.
Their GitHub releases must be marked as pre-releases, use a development-version
identifier, and state that simulator results do not establish hardware equivalence.

No stable production tag may be created until all 24 faulty/fixed cases are
executable, the five-backend case passes, each reference platform has a current
production-approved capability package, and every production claim resolves to
original source, model, toolchain, hardware trace, calibration, and envelope
evidence.

Neither the `simulation` nor the `software` gate is a hardware-equivalence claim. The
`production` gate intentionally remains failed until five-board differential
evidence, instrument calibration records, approved Validation Envelopes and
independent production approvals exist. No CI job may waive those inputs.
