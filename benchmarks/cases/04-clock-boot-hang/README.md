# 04 - Clock initialization causes a boot hang

Mechanism: clock ready condition absent.

Causal chain: clock ready condition absent -> bounded boot poll expires -> boot assertion fails.

Fidelity boundary: Clock fidelity is model-dependent.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
