# 17 - Signed OTA power-loss corruption and anti-rollback recovery

Mechanism: power loss interrupts candidate commit.

Causal chain: power loss interrupts candidate commit -> boot journal selects a slot -> rollback oracle checks committed image.

Fidelity boundary: Storage and brownout models are both model-dependent.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
