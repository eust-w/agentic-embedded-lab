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

Pull requests execute actual Kconfig, Devicetree and linker mechanisms plus the
unit/security suite. Nightly builds both Zephyr
architectures and all independently licensed backend images, then runs 24
pairs, FMI/SSP, a 20-run deterministic hash and the simulation gate. Evidence
is an Actions artifact, never source-controlled. Production adds real
differential evidence and therefore remains blocked in this repository.

`native` is restricted to control-plane tests and is rejected as a formal
benchmark backend. A case is also rejected when faulty/fixed assets are
identical, an experiment contains a direct result selector such as `mcu.fixed`
or `fault_scale`, or its bundle lacks both tool events and raw artifacts.

Mechanism evidence is not a blanket fidelity claim. For example, a Zephyr
algorithm executed on a virtual MCU can prove the compiled control path and its
oracle, while the case boundary may still mark register timing, analog behavior
or silicon errata as unverified.
