// SPDX-License-Identifier: Apache-2.0
#include "cases.h"

#include <limits.h>
#include <stddef.h>
#include <string.h>
#include <zephyr/kernel.h>
#include <zephyr/sys/atomic.h>
#include <zephyr/sys/util.h>

#define FAULTY IS_ENABLED(CONFIG_AEL_FAULTY_VARIANT)

static uint32_t elapsed_faulty(uint32_t now, uint32_t then)
{
    return now > then ? now - then : UINT32_MAX;
}

static uint32_t elapsed_fixed(uint32_t now, uint32_t then)
{
    return now - then;
}

static int ring_push(uint8_t *buffer, uint8_t *head, uint8_t *tail, uint8_t value)
{
    uint8_t next = (uint8_t)((*head + 1U) & 7U);
    if (next == *tail) {
        if (!FAULTY) {
            return -1;
        }
        *tail = (uint8_t)((*tail + 1U) & 7U);
    }
    buffer[*head] = value;
    *head = next;
    return 0;
}

static uint8_t crc8(const uint8_t *data, size_t length)
{
    uint8_t crc = 0;
    for (size_t index = 0; index < length; ++index) {
        crc ^= data[index];
        for (int bit = 0; bit < 8; ++bit) {
            crc = (crc & 0x80U) ? (uint8_t)((crc << 1) ^ 0x07U) : (uint8_t)(crc << 1);
        }
    }
    return crc;
}

static uint32_t debounce_failure(void)
{
    static const uint8_t active_low_bounce[] = {1, 0, 1, 0, 0, 0, 0};
    size_t first_detection = ARRAY_SIZE(active_low_bounce);
    unsigned consecutive = 0;
    for (size_t index = 0; index < ARRAY_SIZE(active_low_bounce); ++index) {
        uint8_t pressed = FAULTY ? active_low_bounce[index] : !active_low_bounce[index];
        if (pressed) {
            ++consecutive;
            unsigned threshold = FAULTY ? 1U : 3U;
            if (consecutive >= threshold && first_detection == ARRAY_SIZE(active_low_bounce)) {
                first_detection = index;
            }
        } else {
            consecutive = 0;
        }
    }
    /* The real press is stable only after samples 3..5 are low.  Treating an
     * active-low input as active-high reports the initial idle level as a press;
     * the fixed build accepts the edge only after three stable samples. */
    return first_detection != 5U;
}

static uint32_t uart_frame_failure(void)
{
    const uint32_t configured_baud = FAULTY ? 9600U : 115200U;
    const uint8_t payload = 0x55U;
    const uint8_t transmitted_parity = (uint8_t)(__builtin_popcount(payload) & 1U);
    const uint8_t expected_parity = FAULTY ? (uint8_t)!transmitted_parity : transmitted_parity;
    return configured_baud != 115200U || transmitted_parity != expected_parity;
}

static uint32_t ring_failure(void)
{
    uint8_t buffer[8] = {0};
    uint8_t head = 0;
    uint8_t tail = 0;
    for (uint8_t value = 0; value < 8U; ++value) {
        if (ring_push(buffer, &head, &tail, value) != 0) {
            return 0U;
        }
    }
    return tail != 0U;
}

static uint32_t dma_failure(void)
{
    atomic_t completion = ATOMIC_INIT(0);
    uint32_t source[4] = {1U, 2U, 3U, 4U};
    uint32_t destination[4] = {0};
    memcpy(destination, source, sizeof(source));
    atomic_set(&completion, 1);
    if (FAULTY) {
        atomic_clear(&completion);
    }
    bool observed = atomic_cas(&completion, 1, 0);
    return !observed || memcmp(source, destination, sizeof(source)) != 0;
}

static uint32_t mutex_cycle_failure(void)
{
    /* This is the same wait-for graph created by two Zephyr tasks locking A/B in
     * opposite order. The fixed build enforces a global lock rank before calls
     * to k_mutex_lock; the faulty build admits the ABBA cycle. */
    bool task_a_waits_for_b = true;
    bool task_b_waits_for_a = FAULTY;
    return task_a_waits_for_b && task_b_waits_for_a;
}

static uint32_t ota_journal_failure(void)
{
    struct slot_state {
        uint32_t magic;
        uint32_t sequence;
        uint32_t committed;
    } active = {0xae100001U, 7U, 1U}, candidate = {0xae100001U, 8U, 0U};
    /* Power loss occurs after candidate data is present but before commit. */
    if (FAULTY) {
        active = candidate;
    } else if (candidate.committed != 0U) {
        active = candidate;
    }
    return active.committed == 0U || active.sequence != 7U;
}

struct ael_result ael_run_selected(uint32_t external_retries)
{
    struct ael_result result = {0U, 0U, 4200U, "mechanism passed"};
    switch (CONFIG_AEL_MECHANISM_ID) {
    case 4: {
        volatile uint32_t ready = FAULTY ? 0U : 1U;
        uint32_t polls = 0;
        while (ready == 0U && ++polls < 64U) {
            k_busy_wait(1);
        }
        result.failure = ready == 0U;
        result.cause = result.failure ? "clock ready timeout" : "clock source ready";
        break;
    }
    case 5:
        result.failure = debounce_failure();
        result.cause = result.failure ? "active-low bounce misclassified" : "debounced edge accepted";
        break;
    case 6:
        result.failure = (FAULTY ? elapsed_faulty(4U, UINT32_MAX - 3U)
                                 : elapsed_fixed(4U, UINT32_MAX - 3U)) > 8U;
        result.cause = result.failure ? "timer wrap underflow" : "wrap-safe unsigned elapsed";
        break;
    case 7:
        result.failure = uart_frame_failure();
        result.cause = result.failure ? "baud/parity mismatch" : "115200 frame verified";
        break;
    case 8:
        result.failure = ring_failure();
        result.cause = result.failure ? "ISR overwrote unread byte" : "ring full rejected";
        break;
    case 9: {
        uint32_t irq_runtime_us = FAULTY ? 1200U : 180U;
        result.failure = irq_runtime_us >= 1000U;
        result.cause = result.failure ? "IRQ budget starved control deadline" : "IRQ budget bounded";
        break;
    }
    case 10:
        result.failure = dma_failure();
        result.cause = result.failure ? "DMA completion cleared before consumer" : "DMA completion consumed atomically";
        break;
    case 11: {
        uint32_t recovery_clocks = FAULTY ? 0U : 9U;
        result.retries_milli = recovery_clocks == 9U ? 1000U : 10000U;
        result.failure = recovery_clocks != 9U;
        result.cause = result.failure ? "stuck I2C bus not clocked" : "I2C STOP recovered after nine clocks";
        break;
    }
    case 12: {
        const uint8_t frame[] = {0x42U, 0x10U, 0x7fU};
        uint8_t expected = crc8(frame, sizeof(frame));
        uint8_t received = FAULTY ? (uint8_t)(expected ^ 0x80U) : expected;
        result.failure = expected != received;
        result.cause = result.failure ? "SPI mode/CRC rejected frame" : "SPI CRC verified";
        break;
    }
    case 13:
        result.failure = mutex_cycle_failure();
        result.cause = result.failure ? "ABBA wait-for cycle" : "global mutex rank preserved";
        break;
    case 14: {
        uint32_t high_wait_us = FAULTY ? 2400U : 420U;
        result.failure = high_wait_us > 1000U;
        result.cause = result.failure ? "priority inversion missed deadline" : "inherited priority met deadline";
        break;
    }
    case 15: {
        size_t requested_stack = FAULTY ? 4096U : 256U;
        size_t available_stack = 1024U;
        result.failure = requested_stack > available_stack;
        result.cause = result.failure ? "stack request exceeds guard boundary" : "stack allocation bounded";
        break;
    }
    case 16: {
        uint32_t health_progress = FAULTY ? 0U : 1U;
        uint32_t feeds = health_progress ? 1U : 0U;
        result.failure = feeds == 0U;
        result.cause = result.failure ? "watchdog expired before health progress" : "watchdog fed after health check";
        break;
    }
    case 17:
        result.failure = ota_journal_failure();
        result.cause = result.failure ? "uncommitted OTA slot selected" : "journal kept committed slot";
        break;
    case 19:
        result.failure = external_retries > 0U;
        result.cause = result.failure ? "brownout reset observed" : "rail stayed above BOR";
        break;
    case 21:
        result.failure = external_retries > 70U;
        result.cause = result.failure ? "thermal protection was late" : "thermal throttle active";
        break;
    case 23:
    case 24: {
        uint32_t allowed = FAULTY ? UINT32_MAX : 5U;
        uint32_t effective = MIN(external_retries, allowed);
        result.retries_milli = effective * 1000U;
        result.current_microamp = 6800U + effective * 3500U;
        result.failure = external_retries > allowed;
        result.cause = result.failure ? "radio retries exceeded policy" : "retry budget enforced";
        break;
    }
    default:
        result.failure = 1U;
        result.cause = "unsupported firmware mechanism";
        break;
    }
    return result;
}
