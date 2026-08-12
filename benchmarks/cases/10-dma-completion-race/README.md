# 10 - DMA completion interrupt race

Causal chain: DMA flag races consumer -> completion is lost -> transfer does not complete.

Fidelity boundary: Requires peripheral and DMA conformance tests.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
