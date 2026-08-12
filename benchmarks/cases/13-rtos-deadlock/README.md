# 13 - RTOS mutex deadlock

Causal chain: mutex order ABBA -> tasks block cyclically -> forward progress stops.

Fidelity boundary: RTOS binary and virtual CPU behavior only.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
