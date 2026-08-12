# Production 1.0 readiness

The repository version is `0.1.0.dev0`. The name “Agentic Embedded Lab 1.0” is
the target architecture, not the current release state.

| Gate | Current evidence | State |
|---|---|---|
| Strict contracts, path safety, CLI, evidence, SQLite/CAS | local automated tests | implemented |
| MCP and `/v1` HTTP thin adapters | import/API tests | implemented |
| Process/container adapters and deterministic scheduler | unit tests and Ubuntu Actions | simulation-validated |
| 24 faulty/fixed benchmark contracts | executable pairs, Evidence Bundles and Nightly | simulation-validated |
| ARM/RISC-V Zephyr reference firmware | pinned Ubuntu firmware build and Renode smoke | simulation-validated |
| Five backend images | pinned Dockerfiles, probes and backend image matrix | simulation-validated |
| FMI 2.0 proxies and five-domain SSP | C++ ABI checks and Ubuntu OMSimulator acceptance | simulation-validated; functional exchange only |
| PostgreSQL/S3, leases, mTLS, OIDC | tests, fault recovery and deployment rendering | software-validated, production unapproved |
| Five board/instrument Lab Worker | allow-listed unverified drivers | no real Lab evidence; blocked |
| License/security/recovery approval | SBOM/review workflows defined | blocked pending independent approval |

## Release rule

`1.0` must not be tagged until all 24 faulty/fixed cases are executable, the
five-backend case passes, each reference platform has a current production-
approved capability package, and every production claim resolves to original
source, model, toolchain, hardware trace, calibration, and envelope evidence.

The software `simulation` gate is not a hardware-equivalence claim. The
`production` gate intentionally remains failed until five-board differential
evidence, instrument calibration records, approved Validation Envelopes and
independent production approvals exist. No CI job may waive those inputs.
