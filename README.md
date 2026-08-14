# Agentic Embedded Lab

Agentic Embedded Lab (AEL) is an agent-native experimentation, modeling, and
validation control plane for embedded systems. It routes embedded engineering
problems to explicit simulation or hardware backends, records reproducible
evidence, and refuses to silently turn a missing model into a passing result.

The repository is at **`0.2.0.dev0` Development Preview**. The software execution plane is
implemented, but this is **not a production release**. Production remains
deliberately blocked because no current five-board differential or instrument
calibration evidence exists. A simulator pass never becomes a hardware claim.

## What works now

- Strict, versioned contracts for problems, systems, experiments, models,
  validation envelopes, events, claims, and evidence.
- Capability-aware routing across Zephyr builds, Renode, ngspice,
  OpenModelica/OMSimulator, ns-3, openEMS, native control-plane tests, and
  hardware validation.
- A deterministic multi-rate scheduler, checkpointing, safe-stop behavior, and
  out-of-process/container protocols for Renode, ngspice, OpenModelica,
  OMSimulator, ns-3, and openEMS.
- Five C++20 FMI 2.0 Co-Simulation proxies, SSP export, type/unit/loop preflight,
  non-rollback enforcement, and event-driven openEMS result caching.
- SQLite/local CAS and PostgreSQL/S3-compatible server storage with matching
  Run, Model, Claim, Worker lease, and Evidence semantics.
- CMSIS-SVD and SystemRDL import plus grounded OpenAI/Anthropic structured
  generation, typed Hardware Behavior IR, Renode C# emission, per-field
  grounding/receipts, governed lifecycle, and a default-offline OCI sandbox boundary.
- Nine domain-level MCP tools plus thin `/v1` HTTP and CLI adapters; no shell,
  Renode Monitor, or raw SCPI tool is exposed.
- Twenty-four checked-in faulty/fixed experiment pairs spanning actual Zephyr
  build inputs, firmware/RTOS logic, simulator peripherals, SPICE, Modelica,
  ns-3 and openEMS, with raw mechanism evidence and explicit fidelity boundaries.
- A PostgreSQL/MinIO/OIDC/mTLS/Envoy/Worker Compose topology with executable
  lease recovery, cancellation, storage outage, migration and restart acceptance.
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
ael benchmark run --case-id 1 --case-id 2 --case-id 3  # needs pinned Zephyr toolchain
ael release check --profile foundation
ael release check --profile software    # requires qualified simulation + software evidence
ael release check --profile production  # must fail without real Lab evidence
pytest
```

The smoke experiment intentionally uses `synthetic`, a test-only backend. Its
evidence bundle is marked `unverified` and `synthetic`; it cannot be promoted
to a hardware-equivalence claim.

## Execution environments

macOS ARM64 supports the Python control plane, schemas, model import, C++ proxy
compilation, all five backend containers, containerized Linux FMU/OMSimulator
acceptance, and the local Compose topology through Colima. Release authority is
evidence-based rather than CI-provider-based: a native or containerized Linux
environment qualifies when it records immutable source identity, inspected image
identity, pinned tool probes, complete mechanism evidence, and reproducible traces.
GitHub Actions is an optional remote reproduction environment, not a release-gate
dependency. Missing tools block; no mock substitution is allowed.

Run the complete local software acceptance with:

```bash
scripts/run-local-software-acceptance.sh
```

The script builds Zephyr 4.4.2 with SDK 1.0.1 and all five backend images, runs
all 24 pairs, FMI/SSP, 20-run determinism, the Compose recovery topology, and the
`simulation` and `software` gates. Generated evidence remains ignored.

## Release gates

- `foundation`: contracts, core tests, schemas and C++ proxies.
- `simulation`: 24 hashed mechanism pairs, Zephyr plus five backend aggregates,
  the five-domain chain,
  FMI/SSP acceptance, deterministic traces, and a qualified Linux execution
  environment.
- `software`: simulation plus the PostgreSQL/S3, OIDC/mTLS, Worker recovery,
  migration/rollback, SBOM, signature and license machine evidence.
- `production`: software plus five-board differential bundles, current
  calibrations/envelopes and independent human approval.

The production release remains blocked until the five reference boards have current
differential bundles, every production claim has a calibrated validation
envelope, and the deployment, recovery, security, signing, and license reviews
have passed. Development Preview tags such as `v0.2.0.dev0` may be published
after the `software` gate passes, but they must be marked as GitHub pre-releases
and must not claim hardware equivalence. Stable production tags remain blocked
until the `production` gate passes.

Run `scripts/run-compose-acceptance.sh` to exercise the local software topology;
it generates ephemeral development certificates, removes project volumes after
the check, and never creates hardware evidence. CI workflows may repeat the same
checks remotely, but do not grant stronger simulation authority merely by running
on GitHub-hosted infrastructure.

See [docs/architecture.md](docs/architecture.md), [docs/benchmark.md](docs/benchmark.md)
and [docs/production-readiness.md](docs/production-readiness.md) for exact boundaries.

The core abstraction is intentionally not robotics-specific. Robot dynamics,
ROS 2, and MuJoCo may be future application adapters, but digital firmware,
circuit, power, thermal, network, RF, and electromagnetic problems are routed
through the same contracts and evidence policy.

## License

The AEL core is licensed under Apache-2.0. GPL and other third-party simulators
are not bundled into the core Python distribution. Backend images and adapters
must preserve their upstream license obligations.
