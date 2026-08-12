# Benchmark policy

`benchmarks/catalog.yaml` is the canonical 24-case inventory. All entries name
checked-in faulty/fixed assets, assertions, a fixed seed, structured causal
chain and fidelity boundary. `executable` describes those runnable contracts;
it does not say Actions or physical hardware passed them.

For a case to change to `executable`, the catalog must name:

1. a minimal faulty asset;
2. an expected failing assertion;
3. a corrected asset;
4. a post-fix regression experiment;
5. a structured causal event chain;
6. a fixed-seed evidence bundle;
7. an explicit fidelity and non-claim boundary.

Pull requests execute the native fast subset. Nightly builds both Zephyr
architectures and all independently licensed backend images, then runs 24
pairs, FMI/SSP, a 20-run deterministic hash and the simulation gate. Evidence
is an Actions artifact, never source-controlled. Production adds real
differential evidence and therefore remains blocked in this repository.
