# 05 - GPIO polarity or button debounce error

Mechanism: active-low bounce sampled.

Causal chain: active-low bounce sampled -> polarity/debounce logic misclassifies edge -> input action fails.

Fidelity boundary: Electrical bounce waveform is not yet validated.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
