# 06 - Timer counter wraparound

Mechanism: 32-bit counter wraps.

Causal chain: 32-bit counter wraps -> elapsed-time implementation evaluates boundary -> deadline is classified.

Fidelity boundary: Functional virtual timing only.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
