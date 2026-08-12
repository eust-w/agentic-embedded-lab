# 17 - Signed OTA power-loss corruption and anti-rollback recovery

Causal chain: power loss interrupts OTA -> active image corrupts -> journal recovery determines boot.

Fidelity boundary: Storage and brownout models are both model-dependent.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
