// SPDX-License-Identifier: Apache-2.0
#include <stdint.h>
#include <zephyr/devicetree.h>
#include <zephyr/kernel.h>

#if defined(CONFIG_AEL_FEATURE)
const uint32_t ael_feature_marker = 0xa31fea7u;
#endif

#if defined(CONFIG_AEL_FORCE_OVERFLOW)
volatile uint8_t ael_linker_pressure[256 * 1024];
#else
volatile uint8_t ael_linker_pressure[1024];
#endif

const uint32_t ael_probe_address = DT_PROP(DT_PATH(chosen), ael_probe_address);

int main(void)
{
    ael_linker_pressure[0] = (uint8_t)ael_probe_address;
    return 0;
}
