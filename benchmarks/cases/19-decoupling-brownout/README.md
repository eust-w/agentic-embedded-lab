# 19 - Insufficient decoupling causes brownout reset

Mechanism: load step crosses BOR threshold.

Causal chain: load step crosses BOR threshold -> power FMU emits brownout -> firmware reset policy observes event.

Fidelity boundary: PCB parasitics remain unverified unless included and calibrated.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
