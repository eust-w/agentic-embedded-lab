# 13 - RTOS mutex deadlock

Mechanism: two lock orders form wait-for graph.

Causal chain: two lock orders form wait-for graph -> progress watchdog observes cycle -> deadlock is reported.

Fidelity boundary: RTOS binary and virtual CPU behavior only.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
