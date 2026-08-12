# 09 - IRQ priority starvation

Causal chain: IRQ priority unbounded -> control task starved -> deadline is missed.

Fidelity boundary: Interrupt timing is model-dependent.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
