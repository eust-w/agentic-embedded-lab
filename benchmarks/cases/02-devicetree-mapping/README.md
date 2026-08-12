# 02 - Devicetree address, IRQ, or pin mapping error

Causal chain: mapping mismatch -> driver binds wrong resource -> peripheral access fails.

Fidelity boundary: Requires a board-specific Renode model.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
