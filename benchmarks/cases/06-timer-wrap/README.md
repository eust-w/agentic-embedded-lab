# 06 - Timer counter wraparound

Causal chain: counter wraps -> signed/branch elapsed math fails -> deadline is misclassified.

Fidelity boundary: Functional virtual timing only.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
