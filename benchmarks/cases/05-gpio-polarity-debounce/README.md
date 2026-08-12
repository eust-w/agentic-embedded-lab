# 05 - GPIO polarity or button debounce error

Causal chain: polarity/debounce wrong -> edge is misclassified -> input action is incorrect.

Fidelity boundary: Electrical bounce waveform is not yet validated.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
