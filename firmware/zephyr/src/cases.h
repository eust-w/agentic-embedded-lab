// SPDX-License-Identifier: Apache-2.0
#pragma once

#include <stdint.h>

struct ael_result {
    uint32_t failure;
    uint32_t retries_milli;
    uint32_t current_microamp;
    const char *cause;
};

struct ael_result ael_run_case(uint32_t case_id, int fixed);
