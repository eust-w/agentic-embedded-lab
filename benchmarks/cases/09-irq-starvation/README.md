# 09 - IRQ priority starvation

Mechanism: IRQ consumes control budget.

Causal chain: IRQ consumes control budget -> 1 kHz deadline is evaluated -> starvation is recorded.

Fidelity boundary: Interrupt timing is model-dependent.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
