# 15 - Stack overflow, HardFault, and crash localization

Causal chain: stack allocation unbounded -> guard trips -> HardFault evidence is captured.

Fidelity boundary: Crash localization does not prove absence of other faults.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
