// SPDX-License-Identifier: Apache-2.0
#include "cases.h"

#include <stdint.h>
#include <zephyr/kernel.h>
#include <zephyr/sys/printk.h>

struct ael_bridge {
    volatile uint32_t case_id;
    volatile uint32_t fixed;
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
    uint32_t previous_fixed = UINT32_MAX;
    while (1) {
        uint32_t case_id = bridge->case_id;
        uint32_t fixed = bridge->fixed;
        if (case_id != previous_case || fixed != previous_fixed) {
            struct ael_result result = ael_run_case(case_id, fixed != 0U);
            if (case_id == 24U && bridge->network_retries > 3U) {
                result.retries_milli = bridge->network_retries * 1000U;
                result.current_microamp += bridge->network_retries * 3500U;
                result.failure = fixed != 0U && bridge->network_retries <= 5U ? 0U : 1U;
                result.cause = "RF loss propagated into the firmware retry policy";
            }
            bridge->failure = result.failure;
            bridge->retries_milli = result.retries_milli;
            bridge->current_microamp = result.current_microamp;
            printk("AEL_EVENT firmware.case {\"case_id\":%u,\"fixed\":%u,"
                   "\"failure\":%u,\"cause\":\"%s\"}\n",
                   case_id, fixed, result.failure, result.cause);
            previous_case = case_id;
            previous_fixed = fixed;
        }
        k_sleep(K_MSEC(1));
    }
    return 0;
}
