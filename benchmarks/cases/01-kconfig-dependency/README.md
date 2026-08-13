# 01 - Kconfig dependency removes a feature from the build

Mechanism: Kconfig dependency false.

Causal chain: Kconfig dependency false -> feature object omitted -> requested behavior absent.

Fidelity boundary: Build-system evidence only.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
