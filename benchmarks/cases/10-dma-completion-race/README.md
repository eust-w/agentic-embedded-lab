# 10 - DMA completion interrupt race

Mechanism: memory transfer completes.

Causal chain: memory transfer completes -> completion flag races consumer -> copied bytes and completion are checked.

Fidelity boundary: Requires peripheral and DMA conformance tests.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
