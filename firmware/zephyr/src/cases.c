// SPDX-License-Identifier: Apache-2.0
#include "cases.h"

#include <limits.h>
#include <stddef.h>
#include <string.h>
#include <zephyr/kernel.h>
#include <zephyr/device.h>
#include <zephyr/devicetree.h>
#include <zephyr/drivers/watchdog.h>
#include <zephyr/fatal.h>
#include <zephyr/linker/sections.h>
#include <zephyr/sys/atomic.h>
#include <zephyr/sys/util.h>
#if CONFIG_AEL_MECHANISM_ID == 17
#include <zephyr/dfu/mcuboot.h>
#include <zephyr/sys/reboot.h>
extern int boot_set_pending(int permanent);
#endif

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

#if CONFIG_AEL_MECHANISM_ID == 13
K_MUTEX_DEFINE(case13_mutex_a);
K_MUTEX_DEFINE(case13_mutex_b);
K_SEM_DEFINE(case13_ready, 0, 2);
K_SEM_DEFINE(case13_go, 0, 2);
K_THREAD_STACK_DEFINE(case13_stack_a, 1024);
K_THREAD_STACK_DEFINE(case13_stack_b, 1024);
static struct k_thread case13_thread_a;
static struct k_thread case13_thread_b;
static atomic_t case13_timeouts;
static atomic_t case13_progress;

static void case13_worker(void *first_pointer, void *second_pointer, void *unused)
{
    ARG_UNUSED(unused);
    struct k_mutex *first = first_pointer;
    struct k_mutex *second = second_pointer;
    if (k_mutex_lock(first, K_MSEC(50)) != 0) {
        atomic_inc(&case13_timeouts);
        return;
    }
    if (FAULTY) {
        k_sem_give(&case13_ready);
        k_sem_take(&case13_go, K_FOREVER);
    }
    if (k_mutex_lock(second, K_MSEC(20)) != 0) {
        atomic_inc(&case13_timeouts);
    } else {
        atomic_inc(&case13_progress);
        k_mutex_unlock(second);
    }
    k_mutex_unlock(first);
}

static uint32_t rtos_deadlock_failure(void)
{
    atomic_clear(&case13_timeouts);
    atomic_clear(&case13_progress);
    while (k_sem_take(&case13_ready, K_NO_WAIT) == 0) {}
    while (k_sem_take(&case13_go, K_NO_WAIT) == 0) {}
    k_thread_create(&case13_thread_a, case13_stack_a, K_THREAD_STACK_SIZEOF(case13_stack_a),
                    case13_worker, &case13_mutex_a, &case13_mutex_b, NULL, 3, 0, K_NO_WAIT);
    k_thread_create(&case13_thread_b, case13_stack_b, K_THREAD_STACK_SIZEOF(case13_stack_b),
                    case13_worker, FAULTY ? &case13_mutex_b : &case13_mutex_a,
                    FAULTY ? &case13_mutex_a : &case13_mutex_b, NULL, 3, 0, K_NO_WAIT);
    if (FAULTY) {
        k_sem_take(&case13_ready, K_MSEC(50));
        k_sem_take(&case13_ready, K_MSEC(50));
        k_sem_give(&case13_go);
        k_sem_give(&case13_go);
    }
    k_thread_join(&case13_thread_a, K_MSEC(100));
    k_thread_join(&case13_thread_b, K_MSEC(100));
    return FAULTY ? atomic_get(&case13_timeouts) == 2 : atomic_get(&case13_progress) != 2;
}
#else
static uint32_t rtos_deadlock_failure(void) { return 1U; }
#endif

#if CONFIG_AEL_MECHANISM_ID == 14
K_MUTEX_DEFINE(case14_mutex);
K_SEM_DEFINE(case14_resource, 1, 1);
K_SEM_DEFINE(case14_ready, 0, 2);
K_SEM_DEFINE(case14_complete, 0, 1);
K_THREAD_STACK_DEFINE(case14_low_stack, 1024);
K_THREAD_STACK_DEFINE(case14_medium_stack, 1024);
K_THREAD_STACK_DEFINE(case14_high_stack, 1024);
static struct k_thread case14_low_thread, case14_medium_thread, case14_high_thread;
static uint64_t case14_wait_us;

static void case14_low(void *a, void *b, void *c)
{
    ARG_UNUSED(a); ARG_UNUSED(b); ARG_UNUSED(c);
    if (FAULTY) { k_sem_take(&case14_resource, K_FOREVER); } else { k_mutex_lock(&case14_mutex, K_FOREVER); }
    k_sem_give(&case14_ready); k_sem_give(&case14_ready);
    k_busy_wait(2000);
    if (FAULTY) { k_sem_give(&case14_resource); } else { k_mutex_unlock(&case14_mutex); }
}
static void case14_medium(void *a, void *b, void *c)
{ ARG_UNUSED(a); ARG_UNUSED(b); ARG_UNUSED(c); k_sem_take(&case14_ready, K_FOREVER); k_busy_wait(5000); }
static void case14_high(void *a, void *b, void *c)
{
    ARG_UNUSED(a); ARG_UNUSED(b); ARG_UNUSED(c); k_sem_take(&case14_ready, K_FOREVER);
    uint64_t start = k_cycle_get_64();
    if (FAULTY) { k_sem_take(&case14_resource, K_FOREVER); k_sem_give(&case14_resource); }
    else { k_mutex_lock(&case14_mutex, K_FOREVER); k_mutex_unlock(&case14_mutex); }
    case14_wait_us = k_cyc_to_us_floor64(k_cycle_get_64() - start); k_sem_give(&case14_complete);
}
static uint32_t priority_inversion_failure(void)
{
    case14_wait_us = 0;
    k_thread_create(&case14_low_thread, case14_low_stack, K_THREAD_STACK_SIZEOF(case14_low_stack), case14_low, NULL, NULL, NULL, 5, 0, K_NO_WAIT);
    k_sleep(K_MSEC(1));
    k_thread_create(&case14_high_thread, case14_high_stack, K_THREAD_STACK_SIZEOF(case14_high_stack), case14_high, NULL, NULL, NULL, 1, 0, K_NO_WAIT);
    k_thread_create(&case14_medium_thread, case14_medium_stack, K_THREAD_STACK_SIZEOF(case14_medium_stack), case14_medium, NULL, NULL, NULL, 3, 0, K_NO_WAIT);
    if (k_sem_take(&case14_complete, K_MSEC(100)) != 0) { return 1U; }
    k_thread_join(&case14_low_thread, K_MSEC(100)); k_thread_join(&case14_medium_thread, K_MSEC(100)); k_thread_join(&case14_high_thread, K_MSEC(100));
    return case14_wait_us >= 4000U;
}
#else
static uint32_t priority_inversion_failure(void) { return 1U; }
#endif

#if CONFIG_AEL_MECHANISM_ID == 15
#if defined(CONFIG_SOC_SERIES_STM32F4X)
#define AEL_FATAL_BRIDGE_ADDRESS 0x2001FC00UL
#else
#define AEL_FATAL_BRIDGE_ADDRESS 0x80003C00UL
#endif
K_THREAD_STACK_DEFINE(case15_stack, 512);
static struct k_thread case15_thread;
static volatile uint32_t case15_sink;
__attribute__((noinline)) static void case15_overflow(unsigned depth)
{ volatile uint8_t pressure[128]; memset((void *)pressure, (int)depth, sizeof(pressure)); if (depth > 0) { case15_overflow(depth - 1U); } case15_sink += pressure[depth & 127U]; }
static void case15_worker(void *a, void *b, void *c)
{ ARG_UNUSED(a); ARG_UNUSED(b); ARG_UNUSED(c); if (FAULTY) { case15_overflow(6U); } }
void k_sys_fatal_error_handler(unsigned int reason, const struct arch_esf *esf)
{
    ARG_UNUSED(esf); volatile uint32_t *bridge = (void *)AEL_FATAL_BRIDGE_ADDRESS;
    bridge[2] = 1U; bridge[3] = reason; printk("AEL_EVENT firmware.hardfault {\"reason\":%u}\n", reason);
    while (1) { k_cpu_idle(); }
}
static uint32_t stack_fault_failure(void)
{
    k_thread_create(&case15_thread, case15_stack, K_THREAD_STACK_SIZEOF(case15_stack), case15_worker, NULL, NULL, NULL, 2, 0, K_NO_WAIT);
    if (!FAULTY) { return k_thread_join(&case15_thread, K_MSEC(100)) != 0; }
    k_thread_join(&case15_thread, K_MSEC(100)); return 0U;
}
#else
static uint32_t stack_fault_failure(void) { return 1U; }
#endif

static uint32_t case16_error;
#if CONFIG_AEL_MECHANISM_ID == 16
static uint32_t watchdog_failure(void)
{
    case16_error = 0U;
#if defined(CONFIG_SOC_SERIES_STM32F4X)
    volatile uint32_t *pwr_cr = (void *)0x40007000UL;
    *pwr_cr |= BIT(8);
    volatile uint32_t *bridge = (void *)0x40002850UL;
#else
    volatile uint32_t *bridge = (void *)0x80003C00UL;
#endif
    if (FAULTY && bridge[1] == 0xae16b007U) { bridge[1] = 0U; return 1U; }
    bridge[1] = FAULTY ? 0xae16b007U : 0U;
    const struct device *watchdog = DEVICE_DT_GET_OR_NULL(DT_NODELABEL(iwdg));
    if (watchdog == NULL || !device_is_ready(watchdog)) { case16_error = 1U; return 1U; }
    struct wdt_timeout_cfg timeout = { .window = { .min = 0U, .max = 20U }, .callback = NULL, .flags = WDT_FLAG_RESET_SOC };
    int channel = wdt_install_timeout(watchdog, &timeout); if (channel < 0) { case16_error = 2U; return 1U; }
    if (wdt_setup(watchdog, 0U) != 0) { case16_error = 3U; return 1U; }
    if (FAULTY) { k_sleep(K_MSEC(50)); return 1U; }
    if (wdt_feed(watchdog, channel) != 0) { case16_error = 4U; return 1U; }
    return 0U;
}
#else
static uint32_t watchdog_failure(void) { return 1U; }
#endif

static uint32_t case17_error;
#if CONFIG_AEL_MECHANISM_ID == 17
static uint32_t ota_journal_failure(void)
{
    case17_error = 0U;
    struct mcuboot_img_header header;
    uint8_t active = boot_fetch_active_slot();
    if (boot_read_bank_header(active, &header, sizeof(header)) != 0 || header.mcuboot_version != 1U) { case17_error = 1U; return 1U; }
    if (header.h.v1.sem_ver.major == 0U) {
        int request = boot_set_pending(0);
        if (request != 0) { case17_error = 300U + (uint32_t)request; return 1U; }
        sys_reboot(SYS_REBOOT_COLD);
        return 1U;
    }
    if (FAULTY) { case17_error = boot_is_img_confirmed() ? 0U : 3U; return case17_error != 0U; }
    if (boot_write_img_confirmed() != 0) { case17_error = 4U; return 1U; }
    if (!boot_is_img_confirmed()) { case17_error = 5U; return 1U; }
    return 0U;
}
#else
static uint32_t ota_journal_failure(void) { return 1U; }
#endif

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
        result.failure = rtos_deadlock_failure();
        result.cause = result.failure ? "ABBA wait-for cycle" : "global mutex rank preserved";
        break;
    case 14: {
        result.failure = priority_inversion_failure();
        result.cause = result.failure ? "priority inversion missed deadline" : "inherited priority met deadline";
        break;
    }
    case 15: {
        result.failure = stack_fault_failure();
        result.cause = result.failure ? "stack request exceeds guard boundary" : "stack allocation bounded";
        break;
    }
    case 16: {
        result.failure = watchdog_failure();
        result.retries_milli = case16_error * 1000U;
        result.cause = result.failure ? "watchdog expired before health progress" : "watchdog fed after health check";
        break;
    }
    case 17:
        result.failure = ota_journal_failure();
        result.retries_milli = case17_error * 1000U;
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
