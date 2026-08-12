# Production 1.0 readiness

The repository version is `0.1.0.dev0`. The name “Agentic Embedded Lab 1.0” is
the target architecture, not the current release state.

| Gate | Current evidence | State |
|---|---|---|
| Strict contracts, path safety, CLI, evidence, SQLite/CAS | local automated tests | implemented |
| MCP and `/v1` HTTP thin adapters | import/API tests | implemented |
| Process/container adapters and deterministic scheduler | unit tests and Actions definitions | implemented, awaiting Actions evidence |
| 24 faulty/fixed benchmark contracts | assets and catalog validation | executable, not accepted here |
| ARM/RISC-V Zephyr reference firmware | source and pinned build workflow | awaiting Ubuntu build evidence |
| Five backend images | pinned Dockerfiles and probes | awaiting backend workflow evidence |
| FMI 2.0 proxies and five-domain SSP | local C++ build; Ubuntu run pending | partially evidenced |
| PostgreSQL/S3, leases, mTLS, OIDC | code, tests and deployment manifests | software implemented, production unapproved |
| Five board/instrument Lab Worker | allow-listed unverified drivers | no real Lab evidence; blocked |
| License/security/recovery approval | SBOM/review workflows defined | blocked pending independent approval |

## Release rule

`1.0` must not be tagged until all 24 faulty/fixed cases are executable, the
five-backend case passes, each reference platform has a current production-
approved capability package, and every production claim resolves to original
source, model, toolchain, hardware trace, calibration, and envelope evidence.
