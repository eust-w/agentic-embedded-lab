# Hardware lab gate

Production differential validation targets three same-revision samples each of
STM32F407G-DISC1, HiFive1 Rev B, nRF52840 DK, ESP32-S3-DevKitC-1, and RP2040
Pico. No hardware is required for Foundation development and no hardware claim
is currently made.

Required instrumentation includes a 100 MHz or better logic analyzer, 200 MHz
four-channel scope, SCPI supply, JS220-class power analyzer, -20 to 85 °C
chamber, VNA/spectrum analyzer/attenuator/shield box covering the relevant band.

The outbound Lab Worker and allow-listed operation policy are implemented, but
all board and instrument definitions remain `unverified`. Agent-facing
operations are allow-listed workflows. Raw SCPI is never exposed.
Results must identify board revision, serial/sample, firmware/model hash,
instrument calibration, voltage, temperature, frequency/input range, raw trace,
metric extraction version, threshold, and reviewer.

The default target thresholds from the product plan are templates, not global
truth. Each accepted value belongs to one signed `ValidationEnvelope`; any
change that loosens a threshold creates a new envelope and requires approval.
