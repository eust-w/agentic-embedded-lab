# 11 - I2C NACK, bus busy, and recovery failure

Mechanism: target leaves I2C bus busy.

Causal chain: target leaves I2C bus busy -> recovery clocks and STOP are attempted -> retry limit is checked.

Fidelity boundary: Digital protocol only; rise time is not represented.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
