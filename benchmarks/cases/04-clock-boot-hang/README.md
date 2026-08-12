# 04 - Clock initialization causes a boot hang

Causal chain: clock source never ready -> boot wait does not exit -> application does not start.

Fidelity boundary: Clock fidelity is model-dependent.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
