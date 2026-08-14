# Contributing to Agentic Embedded Lab

Thank you for helping build an open, agent-native embedded engineering lab.
AEL welcomes contributions to control-plane code, simulator adapters, model
packages, benchmark mechanisms, experiment oracles, evidence tooling,
documentation, and reproducible validation.

## Before you start

- Open an issue for large architectural changes or a new public contract.
- Keep the core general to embedded systems. Robotics and other verticals belong
  in optional adapters or examples.
- Preserve evidence boundaries: a test pass proves code behavior; a simulator
  pass does not prove hardware behavior.
- Do not add a silent mock or fallback for a missing backend, model, tool, or
  physical capability.
- Never commit API keys, private certificates, lab credentials, raw sensitive
  instrument data, or generated `runs/` and `.ael/` state.

## Development setup

Python 3.12 is required.

```bash
python3.12 -m venv .venv
. .venv/bin/activate
python -m pip install -e '.[dev,mcp,server,worker,modeling]'
```

Run the required checks before opening a pull request:

```bash
.venv/bin/ruff check .
.venv/bin/pytest
.venv/bin/ael doctor
```

When a public contract changes, regenerate schemas and review the diff:

```bash
.venv/bin/ael schema export schemas/v1
git diff -- schemas/v1
```

The complete software topology can be checked with:

```bash
scripts/run-compose-acceptance.sh
```

It uses ephemeral development certificates, removes project containers and
volumes afterward, and does not produce hardware evidence.

## Contribution types

### Backend adapter

An adapter must implement the versioned process/container operations, provide a
deterministic probe, declare capability and fidelity, terminate safely, and
extract raw evidence. It must not allow an experiment or agent to provide an
arbitrary executable.

### Model package

A model contribution must include source provenance, versioned inputs/outputs,
units, tests, fidelity, limitations, and lifecycle state. Generated tests cannot
be the only conformance evidence. Hardware validation requires independent
physical evidence and a Validation Envelope.

### Benchmark case

Every executable case needs:

- distinct faulty and fixed assets;
- a named mechanism and an explicit oracle;
- a fixed seed where randomness exists;
- raw mechanism evidence and a structured causal chain;
- replay and regression coverage;
- a clear statement of what the case does **not** prove.

### Documentation

Prefer runnable commands and links to source contracts. Label future designs as
roadmap or vision, and keep English and Simplified Chinese landing pages aligned
when changing public positioning.

## Pull requests

Keep pull requests focused and explain:

- what changed and why;
- the user or developer impact;
- the checks and execution environment used;
- the evidence and fidelity boundary;
- any security, license, compatibility, or migration implications.

All contributions are accepted under the repository's Apache-2.0 license.
