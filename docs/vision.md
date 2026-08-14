# Plugin-first, continuous, evidence-driven embedded engineering

This document describes the direction of Agentic Embedded Lab beyond the
current `0.2.0.dev0` Development Preview. It deliberately separates existing
mechanisms from planned capabilities.

## North star

AEL should become an always-available embedded engineering lab in which agents
can formulate hypotheses, compose experiments, execute the appropriate fidelity,
learn from evidence, and improve firmware or models without gaining unrestricted
control over hosts, instruments, or production claims.

Three principles define that lab:

1. **Everything is a plugin.** Agent interfaces, routers, execution backends,
   models, experiment oracles, evidence sinks, workers, instruments, and policy
   gates are replaceable capabilities behind versioned contracts.
2. **Simulation stays in the loop.** A campaign may run continuously, but each
   iteration is deterministic, budgeted, stoppable, replayable, and explicit
   about the fidelity it selected.
3. **Evolution is earned by evidence.** Learning proposes candidates. Independent
   tests, differential evidence, signed envelopes, and human policy decide what
   can be promoted.

## Current foundation versus planned layer

| Capability | `0.2.0.dev0` | Direction |
|---|---|---|
| Backend adapters | In-process registration and versioned process/container protocol | Stable out-of-tree plugin SDK and compatibility policy |
| Model packages | Strict contracts, grounding, receipts, sandboxed generation, lifecycle states | Signed community registry, dependency solver, automated regression selection |
| Experiment execution | Deterministic scheduler, asynchronous workers, checkpoint/replay, comparison | Long-running campaign controller with budgets, stop rules, prioritization |
| Multi-physics | Five domains through FMI 2.0/SSP | Distributed co-simulation, elastic workers, richer fidelity selection |
| Learning | Grounded candidate generation and conformance promotion | Evidence-aware curricula, residual-driven model calibration, candidate ranking |
| Hardware | Lab Worker definitions and allow-listed drivers | Differential calibration and signed Validation Envelopes |
| Governance | Model and release gates; no self-approval | Organization policy packs, approval workflows, transparent model lineage |

## Plugin model

A plugin is a capability package with four mandatory properties:

- a stable type and protocol version;
- a machine-readable capability and fidelity declaration;
- deterministic health and conformance probes;
- an evidence contract describing the artifacts it can produce.

Plugins do not receive ambient host authority. They run in a process, container,
or Lab Worker boundary selected by administrators. An experiment refers to a
capability, not to a shell command or executable path.

### Planned plugin families

#### Agent interfaces

Translate external agent protocols into domain operations. MCP is the first
implementation; future protocols must call the same service behavior and may not
add privileged escape hatches.

#### Routers and planners

Classify a problem, compute required capabilities, select the smallest sufficient
fidelity, and emit an executable plan or a precise verification gap.

#### Execution adapters

Implement `probe`, `prepare`, `inject`, `step`, `snapshot`, `collect`, and
`shutdown` over `ael.dev/backend/v1`. Adapters must never silently replace an
unavailable backend with a synthetic one.

#### Models and model generators

Package executable behavior, sources, FMI ports, tests, fidelity, state, and
Validation Envelopes. Generators create candidates; they do not grant validation.

#### Oracles and analyzers

Turn raw traces and waveforms into named mechanism outcomes and causal events.
An oracle must state the observations it consumed and the limits of its result.

#### Evidence stores and provenance

Persist content-addressed artifacts, events, claims, signatures, calibration
records, and replay metadata without changing their semantic identity between
local and server deployments.

#### Workers, laboratories, and instruments

Advertise bounded capabilities, lease work outbound over mTLS, and expose only
allow-listed operations. An agent never receives raw SCPI or host-device access.

#### Policy and promotion gates

Evaluate whether a model or Claim may move between lifecycle states. Gates are
independent from the generator that created the candidate.

## Always-on simulation campaigns

A campaign is a durable state machine over experiments rather than an infinite
prompt loop.

```text
observe -> hypothesize -> plan -> execute -> analyze -> compare -> decide
    ^                                                           |
    +---------------- evidence + next objective -----------------+
```

Every campaign must define:

- an objective and measurable oracle;
- allowed plugins, model states, and fidelity levels;
- compute, wall-time, token, and hardware budgets;
- concurrency and retry policies;
- regression suites and protected baseline Claims;
- success, saturation, uncertainty, and safety stop conditions;
- an approval policy for source, model, envelope, or deployment changes.

The campaign controller should be resumable and idempotent. A restart restores
the last signed decision and immutable evidence; it does not infer progress from
logs or repeat side effects blindly.

### Fidelity scheduling

Continuous execution becomes affordable only when fidelity is selected
hierarchically:

1. schema, static, analytical, and host-native checks;
2. functional firmware or protocol models;
3. virtual MCU and discrete network simulation;
4. circuit, continuous, RF, and multi-domain co-simulation;
5. HIL or physical differential validation.

The scheduler can promote a candidate to a more expensive tier when uncertainty
or risk requires it. It may not downgrade silently to make an experiment pass.

## Continual learning and controlled self-evolution

AEL separates **learning**, **validation**, and **promotion**.

### 1. Learn

Collect structured residuals, failed assertions, causal events, traces, model
coverage, and human decisions. Propose new firmware patches, experiments,
parameters, or model candidates grounded in hashed source material.

### 2. Validate

Execute independent static checks, compilation, property tests, driver tests,
reference traces, regression experiments, cross-model comparisons, and—when
available—physical differential measurements.

### 3. Promote

Apply the fixed lifecycle:

```text
draft -> generated -> static_validated -> conformance_validated
      -> hardware_validated -> production_approved -> deprecated
```

An agent can automatically reach only `conformance_validated`. Hardware evidence
is required for `hardware_validated`; a signed Validation Envelope and human
approval are additionally required for `production_approved`.

### 4. Monitor and roll back

New candidates run in shadow mode against protected experiments. Regressions,
distribution shift, envelope violations, signature failures, or missing evidence
disable selection and return to the last approved model.

## What self-evolution must never mean

- a generated model using only its own generated tests to prove correctness;
- an agent changing fidelity or validation state to make a result pass;
- rewriting historical Evidence Bundles or Claim lineage;
- adding a backend through an arbitrary shell command supplied by an agent;
- training on credentials, private lab topology, or unapproved external data;
- automatically extending hardware equivalence beyond a signed envelope;
- changing production firmware or instruments without an explicit approval policy.

## Near-term milestones

1. Specify the stable plugin manifest and compatibility contract.
2. Add signed out-of-tree adapter loading and a local plugin development kit.
3. Implement the durable campaign state machine, budgets, and stop policies.
4. Add regression-aware model ranking and shadow selection.
5. Calibrate the five reference hardware platforms and produce claim-scoped
   Validation Envelopes.
6. Publish a signed community registry with transparent provenance and revocation.

Until those milestones land, README and release notes must label them as design
direction rather than completed production capability.
