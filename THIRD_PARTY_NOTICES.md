# Third-party notices

AEL core is Apache-2.0. It does not bundle the simulators below in its Python
wheel. The independently built backend images download upstream projects and
remain subject to their licenses:

| Component | Pinned version | Upstream | License family |
|---|---:|---|---|
| Renode | 1.16.1 | https://github.com/renode/renode | MIT |
| Zephyr | 4.4.2 | https://github.com/zephyrproject-rtos/zephyr | Apache-2.0 and notices |
| Zephyr SDK | 1.0.1 | https://github.com/zephyrproject-rtos/sdk-ng | mixed toolchain notices |
| ngspice | 46 | https://github.com/imr/ngspice | BSD/GPL/LGPL mixed source notices |
| OpenModelica | 1.27.0 | https://github.com/OpenModelica/OpenModelica | OSMC-PL and third-party notices |
| OMSimulator | 2.1.3 | https://github.com/OpenModelica/OMSimulator | OSMC-PL |
| ns-3 | 3.47 | https://www.nsnam.org | GPL-2.0-only |
| openEMS | 0.0.36 | https://github.com/thliebig/openEMS-Project | GPL-3.0-or-later and dependencies |

This inventory is machine-checked for presence and version pins. It is not a
legal approval; the production release gate requires an explicit license review.
