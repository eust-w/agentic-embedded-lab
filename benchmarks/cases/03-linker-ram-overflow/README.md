# 03 - Linker section and RAM overflow

Causal chain: section exceeds RAM -> linker rejects image -> firmware is not produced.

Fidelity boundary: Linker evidence does not establish runtime safety.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
