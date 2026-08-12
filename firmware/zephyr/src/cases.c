// SPDX-License-Identifier: Apache-2.0
#include "cases.h"

#include <limits.h>
#include <stddef.h>
#include <string.h>

static uint32_t elapsed_faulty(uint32_t now, uint32_t then)
{
    return now > then ? now - then : UINT32_MAX;
}

static uint32_t elapsed_fixed(uint32_t now, uint32_t then)
{
    return now - then;
}

static int ring_push(uint8_t *head, uint8_t *tail, int fixed)
{
    uint8_t next = (uint8_t)((*head + 1U) & 7U);
    if (next == *tail) {
        return fixed ? -1 : 0;
    }
    *head = next;
    return 1;
}

struct ael_result ael_run_case(uint32_t case_id, int fixed)
{
    struct ael_result result = {0U, 0U, 4200U, "no fault"};
    switch (case_id) {
    case 4:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "clock source ready" : "clock ready bit never observed";
        break;
    case 5:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "active-low input debounced" : "GPIO polarity inverted";
        break;
    case 6:
        result.failure = (fixed ? elapsed_fixed(4U, UINT32_MAX - 3U)
                                : elapsed_faulty(4U, UINT32_MAX - 3U)) > 8U;
        result.cause = fixed ? "unsigned wrap-safe elapsed time" : "timer wrap underflow";
        break;
    case 7:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "115200 8N1" : "baud divisor mismatch";
        break;
    case 8: {
        uint8_t head = 7U;
        uint8_t tail = 0U;
        result.failure = ring_push(&head, &tail, fixed) == 0;
        result.cause = fixed ? "ring full is rejected" : "ISR overwrote unread byte";
        break;
    }
    case 9:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "IRQ priorities bounded" : "high-rate IRQ starved control task";
        break;
    case 10:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "DMA completion flag cleared atomically" : "DMA flag race";
        break;
    case 11:
        result.retries_milli = fixed ? 1000U : 10000U;
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "I2C recovery issued nine clocks" : "bus busy was never recovered";
        break;
    case 12:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "SPI mode and CRC agree" : "SPI CPHA/CRC mismatch";
        break;
    case 13:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "ordered mutex acquisition" : "ABBA mutex deadlock";
        break;
    case 14:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "priority inheritance enabled" : "deadline missed by inversion";
        break;
    case 15:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "bounded stack allocation" : "stack guard would HardFault";
        break;
    case 16:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "watchdog fed after health check" : "reset counter loop";
        break;
    case 17:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "A/B image journal recovered" : "power loss corrupted active OTA slot";
        break;
    case 18:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "LDO headroom preserved" : "load step entered LDO dropout";
        break;
    case 19:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "brownout margin restored" : "rail transient crossed BOR threshold";
        break;
    case 20:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "sleep duty cycle budgeted" : "sleep leakage exceeded budget";
        break;
    case 21:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "thermal throttling hysteresis" : "over-temperature protection late";
        break;
    case 22:
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "bounded 802.15.4 retries" : "interference caused retry storm";
        break;
    case 23:
    case 24:
        result.retries_milli = fixed ? 500U : 12000U;
        result.current_microamp = fixed ? 6800U : 52000U;
        result.failure = fixed ? 0U : 1U;
        result.cause = fixed ? "retry budget and backoff enforced" : "unbounded radio retry loop";
        break;
    default:
        result.failure = 1U;
        result.cause = "unsupported firmware benchmark case";
        break;
    }
    return result;
}
