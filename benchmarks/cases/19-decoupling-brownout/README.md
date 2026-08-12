# 19 - Insufficient decoupling causes brownout reset

Causal chain: decoupling is insufficient -> rail transient crosses BOR -> MCU reset is requested.

Fidelity boundary: PCB parasitics remain unverified unless included and calibrated.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
