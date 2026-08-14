# 15 - Stack overflow, HardFault, and crash localization

Mechanism: requested stack crosses guard budget.

Causal chain: requested stack crosses guard budget -> fault evidence is emitted -> fault oracle runs.

Fidelity boundary: Crash localization does not prove absence of other faults.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
