# 16 - Watchdog reset loop

Causal chain: health check never reaches feed -> watchdog resets -> boot loop repeats.

Fidelity boundary: Watchdog model conformance is required.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
