# Architecture

AEL separates agent intent from simulator commands. The CLI is the behavior
source; HTTP and MCP delegate to the same `AelService` and never expose a shell,
Renode Monitor, SCPI, or an unrestricted host path.

```text
Agent / CLI / API
       |
   AelService
       |
Problem Router -- Model Registry
       |
Experiment + deterministic schedule / FMI 2.0 + SSP
       |
Process/container adapter -- Evidence -- SQLite/CAS or PostgreSQL/S3
       |
Renode | ngspice | Modelica | ns-3 | openEMS | Lab Worker
```

## Execution plane

Backends speak `ael.dev/backend/v1` over JSON lines. Operations are probe,
prepare, inject, step, snapshot and shutdown. The process command or OCI image
is fixed by administrator configuration. A request cannot supply an executable
or shell command. OCI execution is read-only, capability-free, network-off and
resource-bounded; the workspace supplies explicit model and evidence paths.

The synthetic adapter exists to test the control plane. It is never eligible
for simulation-validated, hardware-validated, or production-approved claims.

## Multi-rate and FMI boundary

The `SystemManifest` is the semantic topology. `ael.fmi` validates direct-
feedthrough loops, computes a communication quantum, and exports an SSP system
description. The C++20 FMI 2.0 proxies are separately built components:

- `RenodeFmu`: discrete virtual MCU and firmware execution;
- `NgspiceFmu`: transient circuit exchange;
- `ModelicaFmu`: continuous thermal, power, and electromechanical models;
- `Ns3Fmu`: discrete network events;
- `OpenEmsFmu`: cached batch FDTD results at parameter events.

openEMS never participates in millisecond lockstep. Non-rollback components
checkpoint only at synchronization boundaries. An unsolved algebraic loop is a
preflight error, not a scheduler guess.

OMSimulator executes generated SSP packages at global communication points.
Renode/ns-3 are discrete, ngspice/Modelica own internal continuous steps, and
openEMS solves only on parameter changes with content-addressed caching.

## Persistence and workers

Local mode uses SQLite and SHA-256 CAS. Server mode requires PostgreSQL and an
S3-compatible bucket. Workers register capabilities over outbound mTLS, lease
compatible tasks transactionally, heartbeat, recover expired leases, upload
hashed evidence and honor cancellation. Human APIs use OIDC issuer, audience
and JWKS validation. Deployment acceptance remains a production gate.
