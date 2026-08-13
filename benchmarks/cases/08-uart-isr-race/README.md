# 08 - UART ring buffer ISR race and packet loss

Mechanism: RX producer fills ring.

Causal chain: RX producer fills ring -> ISR push meets full-buffer policy -> byte preservation oracle runs.

Fidelity boundary: Scheduler and interrupt models must be validated.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
