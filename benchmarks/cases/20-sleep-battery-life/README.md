# 20 - Sleep leakage and duty-cycle battery-life miss

Mechanism: firmware power states define duty cycle.

Causal chain: firmware power states define duty cycle -> current is integrated -> battery-life oracle runs.

Fidelity boundary: Battery and leakage models require hardware calibration.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
