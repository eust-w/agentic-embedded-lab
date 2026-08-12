# 23 - Wi-Fi latency, partition, and reconnect policy

Causal chain: network partition occurs -> reconnect retries are unbounded -> policy assertion fails.

Fidelity boundary: RF front-end and external infrastructure are not implied.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
