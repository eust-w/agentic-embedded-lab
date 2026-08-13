# Production 1.0 readiness

The repository version is `0.2.0.dev0 Development Preview`. “Production 1.0” is
a gated target, not the current release state.

| Gate | Current evidence | State |
|---|---|---|
| Strict contracts, path safety, CLI, evidence, SQLite/CAS | local automated tests | implemented |
| MCP and `/v1` HTTP thin adapters | import/API tests | implemented |
| Process/container adapters and deterministic scheduler | unit tests, local five-backend runs and prior Ubuntu Actions | implemented; current-head Ubuntu rerun pending |
| 24 faulty/fixed mechanism contracts | Ubuntu previously passed 01–23; current working tree passes 04–24 locally, including the repaired five-domain case | current-head authoritative `simulation` pending |
| ARM/RISC-V Zephyr reference firmware | pinned Ubuntu firmware build and Renode smoke at the prior successful revision | current-head Ubuntu rerun pending |
| Five backend images | current ARM64 images probe successfully; prior Ubuntu image matrix passed | current-head Ubuntu rerun pending |
| FMI 2.0 proxies and five-domain SSP | current macOS/Colima Linux-FMU acceptance and prior Ubuntu OMSimulator acceptance | functional exchange only; current-head Ubuntu rerun pending |
| PostgreSQL/S3, leases, mTLS, OIDC | current local Compose acceptance passes all recovery checks; prior Ubuntu Software RC passed | current-head authoritative `software` pending |
| Five board/instrument Lab Worker | allow-listed unverified drivers | no real Lab evidence; blocked |
| License/security/recovery approval | SBOM/review workflows defined | blocked pending independent approval |

## Verification snapshot (2026-08-13)

The current local software run proves the named mechanisms only. Renode 1.16.1,
ngspice 46, OpenModelica 1.27.0/OMSimulator 2.1.3, ns-3 3.47 and openEMS
0.0.36 all probed successfully on macOS ARM64 through Colima. Benchmarks 04–24
passed as faulty/fixed pairs, benchmark 24 passed its five-backend causal chain,
the FMI/SSP exchange produced non-zero unit-bearing outputs, and the fixed
five-domain trace repeated identically 20 times. The local Compose topology
also passed OIDC, mTLS, Worker lease/cancel/recovery, PostgreSQL restart, S3
retransmit and migration rollback checks.

This is not the `simulation` release gate because that gate requires a complete
01–24 manifest generated on Ubuntu 24.04 x86_64. The latest GitHub-hosted jobs
were prevented from starting by the repository account's Actions billing/spend
limit state, so no current-head authoritative manifest exists yet. This external
blocker must be cleared and the workflows rerun; it must not be replaced with a
locally relabeled manifest.

## Release rule

No production tag may be created until all 24 faulty/fixed cases are executable, the
five-backend case passes, each reference platform has a current production-
approved capability package, and every production claim resolves to original
source, model, toolchain, hardware trace, calibration, and envelope evidence.

Neither the `simulation` nor the `software` gate is a hardware-equivalence claim. The
`production` gate intentionally remains failed until five-board differential
evidence, instrument calibration records, approved Validation Envelopes and
independent production approvals exist. No CI job may waive those inputs.
