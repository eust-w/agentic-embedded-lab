# 18 - LDO load-step dropout

Causal chain: load step exceeds headroom -> LDO enters dropout -> rail minimum crosses limit.

Fidelity boundary: Valid only for the selected component model and conditions.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
