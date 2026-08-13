# 12 - SPI mode or CRC mismatch

Mechanism: SPI frame and CRC are produced.

Causal chain: SPI frame and CRC are produced -> target verifies CRC -> sample is accepted or rejected.

Fidelity boundary: Digital protocol only.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
