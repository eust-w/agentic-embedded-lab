# 14 - Priority inversion and deadline miss

Causal chain: low task holds lock -> high task waits -> medium task causes deadline miss.

Fidelity boundary: Deadline claims require calibrated time models.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
