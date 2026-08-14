// SPDX-License-Identifier: Apache-2.0
#include "cases.h"

#include <stdint.h>
#include <zephyr/kernel.h>
#include <zephyr/sys/printk.h>

struct ael_bridge {
    volatile uint32_t case_id;
    volatile uint32_t variant_reserved;
    volatile uint32_t failure;
    volatile uint32_t retries_milli;
    volatile uint32_t current_microamp;
    volatile uint32_t network_retries;
};

#if defined(CONFIG_SOC_SERIES_STM32F4X)
#define AEL_BRIDGE_ADDRESS 0x2001FC00UL
#else
#define AEL_BRIDGE_ADDRESS 0x80003C00UL
#endif

int main(void)
{
    volatile struct ael_bridge *bridge = (void *)AEL_BRIDGE_ADDRESS;
    uint32_t previous_case = UINT32_MAX;
    while (1) {
        uint32_t case_id = bridge->case_id;
        if (case_id != previous_case) {
            struct ael_result result = ael_run_case(case_id, bridge->network_retries);
            bridge->failure = result.failure;
            bridge->retries_milli = result.retries_milli;
            bridge->current_microamp = result.current_microamp;
            printk("AEL_EVENT firmware.mechanism {\"case_id\":%u,\"variant\":\"%s\","
                   "\"failure\":%u,\"cause\":\"%s\"}\n",
                   case_id,
#if defined(CONFIG_AEL_FAULTY_VARIANT)
                   "faulty",
#else
                   "fixed",
#endif
                   result.failure, result.cause);
            previous_case = case_id;
        }
        k_sleep(K_MSEC(1));
    }
    return 0;
}
