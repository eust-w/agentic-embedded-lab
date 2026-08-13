# 07 - UART baud rate or frame mismatch

Mechanism: UART frame is encoded.

Causal chain: UART frame is encoded -> baud/parity configuration is checked -> peer accepts or rejects frame.

Fidelity boundary: Protocol behavior without analog signal integrity.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
