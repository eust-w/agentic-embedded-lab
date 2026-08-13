# 23 - Wi-Fi latency, partition, and reconnect policy

Mechanism: Wi-Fi link is partitioned.

Causal chain: Wi-Fi link is partitioned -> retry/backoff state advances -> reconnect budget is checked.

Fidelity boundary: RF front-end and external infrastructure are not implied.

The variants select different controlled assets; neither experiment contains a direct pass/fail selector. Tool logs and mechanism events are retained in the Evidence Bundle. No result is physical-hardware evidence.
