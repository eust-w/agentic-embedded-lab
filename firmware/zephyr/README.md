# Reference firmware

This Zephyr 4.4.2 application is architecture-neutral and builds for
`stm32f4_disco` and `hifive1_revb`. It exposes a synthetic SRAM bridge used by
Renode conformance experiments. The bridge is functional test instrumentation;
it is not a model of a physical peripheral, bus, encoder, radio, or circuit.

Builds are produced only in CI with Zephyr SDK 1.0.1. Generated build trees are
ignored and are never accepted as hardware evidence.
