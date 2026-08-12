# Agentic Embedded Lab

Agentic Embedded Lab (AEL) is an agent-native experimentation, modeling, and
validation control plane for embedded systems. It routes embedded engineering
problems to explicit simulation or hardware backends, records reproducible
evidence, and refuses to silently turn a missing model into a passing result.

The repository is under active development. The software execution plane is
implemented, but this is **not the production 1.0 release**. Production remains
deliberately blocked because no current five-board differential or instrument
calibration evidence exists. A simulator pass never becomes a hardware claim.

## What works now

- Strict, versioned contracts for problems, systems, experiments, models,
  validation envelopes, events, claims, and evidence.
- Capability-aware routing across Renode, ngspice, OpenModelica/OMSimulator,
  ns-3, openEMS, native analysis, and hardware validation.
- A deterministic multi-rate scheduler, checkpointing, safe-stop behavior, and
  out-of-process/container protocols for Renode, ngspice, OpenModelica,
  OMSimulator, ns-3, and openEMS.
- Five C++20 FMI 2.0 Co-Simulation proxies, SSP export, type/unit/loop preflight,
  non-rollback enforcement, and event-driven openEMS result caching.
- SQLite/local CAS and PostgreSQL/S3-compatible server storage with matching
  Run, Model, Claim, Worker lease, and Evidence semantics.
- CMSIS-SVD and SystemRDL import, typed Hardware Behavior IR, Renode C# emission,
  governed model lifecycle, and a default-offline OCI sandbox boundary.
- Nine domain-level MCP tools plus thin `/v1` HTTP and CLI adapters; no shell,
  Renode Monitor, or raw SCPI tool is exposed.
- Twenty-four checked-in faulty/fixed experiment pairs spanning firmware,
  analog/power/thermal, network, RF, and EM, with causal and fidelity boundaries.
- Five unverified Lab Worker board definitions and allow-listed instrument drivers.

## Quick start

Python 3.12 is required.

```bash
python3.12 -m venv .venv
. .venv/bin/activate
python -m pip install -e '.[dev,mcp,server,worker,modeling]'

ael doctor
ael inspect
ael classify examples/problems/uart-ring-buffer.yaml
ael validate examples/experiments/synthetic-smoke.yaml
ael run examples/experiments/synthetic-smoke.yaml
ael benchmark run --case-id 1 --case-id 2 --case-id 3
ael release check --profile foundation
ael release check --profile production  # must fail without real Lab evidence
pytest
```

The smoke experiment intentionally uses `synthetic`, a test-only backend. Its
evidence bundle is marked `unverified` and `synthetic`; it cannot be promoted
to a hardware-equivalence claim.

## Execution environments

macOS ARM64 supports the Python control plane, schemas, model import, native
benchmark subset, and C++ proxy compilation. Ubuntu 24.04 x86_64 GitHub Actions
is the authoritative five-backend platform. It builds pinned images and
Zephyr 4.4.2 ARM/RISC-V firmware with SDK 1.0.1, then publishes ignored
`acceptance/` and `runs/` artifacts. Missing tools block; no mock substitution
is allowed.

## Release gates

- `foundation`: contracts, core tests, native cases, schemas and C++ proxies.
- `simulation`: 24 hashed pairs, five backend aggregates, the five-domain chain,
  FMI/SSP acceptance, and deterministic traces on Ubuntu Actions.
- `production`: simulation plus five-board differential bundles, current
  calibrations/envelopes, deployment/recovery, security and license approval.

The 1.0 release remains blocked until the five reference boards have current
differential bundles, every production claim has a calibrated validation
envelope, and the deployment, recovery, security, signing, and license reviews
have passed. No tag or release workflow is provided before that production gate.

See [docs/architecture.md](docs/architecture.md) and
[docs/production-readiness.md](docs/production-readiness.md) for exact
boundaries.

The core abstraction is intentionally not robotics-specific. Robot dynamics,
ROS 2, and MuJoCo may be future application adapters, but digital firmware,
circuit, power, thermal, network, RF, and electromagnetic problems are routed
through the same contracts and evidence policy.

## License

The AEL core is licensed under Apache-2.0. GPL and other third-party simulators
are not bundled into the core Python distribution. Backend images and adapters
must preserve their upstream license obligations.
