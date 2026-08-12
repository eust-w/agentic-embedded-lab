# 07 - UART baud rate or frame mismatch

Causal chain: baud/frame mismatch -> receiver framing fails -> UART assertion fails.

Fidelity boundary: Protocol behavior without analog signal integrity.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
