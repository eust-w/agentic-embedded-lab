# 20 - Sleep leakage and duty-cycle battery-life miss

Causal chain: sleep leakage is high -> average current rises -> battery-life target is missed.

Fidelity boundary: Battery and leakage models require hardware calibration.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
