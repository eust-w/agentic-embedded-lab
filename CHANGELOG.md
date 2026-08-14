# Changelog

All notable changes to Agentic Embedded Lab are documented here.

## Unreleased

### Documentation

- Rebuilt the public project landing page with a bilingual, media-rich README.
- Documented the plugin-first architecture, continuous simulation-in-the-loop,
  and evidence-gated continual-learning and self-evolution direction.
- Added public contribution and security-reporting guidance.
- Moved push-triggered backend and simulation workflows to the default branch.

## 0.2.0.dev0 - 2026-08-14

Development Preview. This is not a production or hardware-equivalence release.

### Added

- Strict problem, system, experiment, event, model, claim and evidence contracts.
- CLI, HTTP and nine domain-level MCP tools backed by one control-plane service.
- Container adapters for Zephyr builds, Renode, ngspice, OpenModelica/OMSimulator,
  ns-3 and openEMS.
- FMI 2.0/SSP multi-rate orchestration and a five-domain reference experiment.
- Twenty-four faulty/fixed embedded benchmark pairs with evidence bundles.
- SQLite/CAS local storage and PostgreSQL/S3-compatible server storage.
- Grounded CMSIS-SVD/SystemRDL and OpenAI/Anthropic model-generation workflows.
- OIDC/mTLS worker topology, leases, recovery checks and allow-listed Lab Worker
  interfaces.

### Validated software boundary

- Local qualified Linux-container execution passed the project `simulation` and
  `software` release gates.
- The acceptance covered Cortex-M and RISC-V Zephyr builds, the five simulator
  backends, FMI/SSP exchange, twenty deterministic trace repetitions and the
  Compose recovery topology.

### Known limitations

- The release is not validated against physical hardware or calibrated instruments.
- Digital benchmark cases 4-17 use a functional firmware mechanism dispatcher and
  bridge; they do not establish peripheral-register, RTOS-scheduling or electrical
  equivalence beyond their recorded fidelity boundaries.
- AEL is the experiment runtime; autonomous reasoning and source edits are supplied
  by an external agent such as Codex or Claude through MCP.
- Missing device models and arbitrary third-party firmware projects can still
  require manual manifests, adapters or model work.
- The `production` gate intentionally fails until hardware differential evidence,
  calibration records, signed Validation Envelopes and human approval exist.

### Release evidence

See `docs/production-readiness.md` and the generated, ignored `acceptance/` and
`runs/` evidence directories produced by `scripts/run-local-software-acceptance.sh`.
