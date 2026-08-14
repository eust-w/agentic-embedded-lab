// SPDX-License-Identifier: Apache-2.0
#include <stdint.h>
#include <zephyr/kernel.h>

#if defined(CONFIG_AEL_FEATURE)
const uint32_t ael_feature_marker = 0xa31fea7u;
#endif

#if defined(CONFIG_AEL_FORCE_OVERFLOW)
volatile uint8_t ael_linker_pressure[256 * 1024];
#else
volatile uint8_t ael_linker_pressure[1024];
#endif

int main(void)
{
    /*
     * Keep the linker fixture independent from the devicetree fixture.  The
     * case-2 oracle inspects Zephyr's generated DTS directly, so referencing
     * its synthetic property here would make unrelated cases depend on it.
     */
    ael_linker_pressure[0] = 0;
    return 0;
}
