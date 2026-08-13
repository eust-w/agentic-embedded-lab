# 24 - Antenna detuning drives retries, power, and heat

Mechanism: antenna geometry detunes and openEMS computes S11.

Causal chain: antenna geometry detunes and openEMS computes S11 -> mismatch loss changes ns-3 delivery and retries -> firmware retry policy changes current -> ngspice rail and Modelica temperature respond.

Fidelity boundary: Five-backend production evidence and hardware calibration are required.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
