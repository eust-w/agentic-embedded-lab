# 11 - I2C NACK, bus busy, and recovery failure

Causal chain: I2C NACK leaves bus busy -> recovery clocks omitted -> transaction retries exhaust.

Fidelity boundary: Digital protocol only; rise time is not represented.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
