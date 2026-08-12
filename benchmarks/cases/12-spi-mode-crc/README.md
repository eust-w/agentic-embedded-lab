# 12 - SPI mode or CRC mismatch

Causal chain: SPI mode/CRC differs -> frame verification fails -> sample is rejected.

Fidelity boundary: Digital protocol only.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
