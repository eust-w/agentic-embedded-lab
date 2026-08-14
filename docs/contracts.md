# Contracts and claims

Versioned JSON Schemas are generated from strict Pydantic models with:

```bash
ael schema export schemas/v1
```

Unknown fields are rejected. File paths are canonicalized relative to the
workspace and rejected if they escape it. Values with physical units use the
supported UCUM symbol subset and are not silently converted.

## Model lifecycle

```text
draft -> generated -> static_validated -> conformance_validated
      -> hardware_validated -> production_approved -> deprecated
```

Deprecation can occur from any active state. Agent workflows cannot grant
`hardware_validated` or `production_approved`. Those transitions require
independent hardware evidence and explicit human approval; production approval
also requires a signature. A generated model cannot validate itself.

## Claim rule

A claim is scoped by statement, exact model versions, evidence, limitations,
hardware revision, and optionally a `ValidationEnvelope`. AEL has no global
“simulator equals hardware” status. Outside an approved envelope, the status is
always `unverified`.
