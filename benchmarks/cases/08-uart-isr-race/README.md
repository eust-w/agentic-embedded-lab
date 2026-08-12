# 08 - UART ring buffer ISR race and packet loss

Causal chain: ISR races ring head -> unread byte overwritten -> packet loss is observed.

Fidelity boundary: Scheduler and interrupt models must be validated.

`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. Neither result is physical hardware evidence.
