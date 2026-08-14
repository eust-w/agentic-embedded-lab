# 02 - Devicetree address, IRQ, or pin mapping error

Mechanism: mapping mismatch.

Causal chain: mapping mismatch -> generated devicetree contains wrong address -> oracle rejects mapping.

Fidelity boundary: Requires a board-specific Renode model.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
