# 01 - Kconfig dependency removes a feature from the build

Causal chain: Kconfig dependency false -> feature object omitted -> requested behavior absent.

Fidelity boundary: Build-system evidence only.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
