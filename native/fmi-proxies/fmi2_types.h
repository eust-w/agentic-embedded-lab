// SPDX-License-Identifier: Apache-2.0
#pragma once

#include <cstddef>

#if defined(_WIN32)
#define FMI2_EXPORT __declspec(dllexport)
#else
#define FMI2_EXPORT __attribute__((visibility("default")))
#endif

extern "C" {
typedef void* fmi2Component;
typedef void* fmi2ComponentEnvironment;
typedef void* fmi2FMUstate;
typedef const char* fmi2String;
typedef double fmi2Real;
typedef int fmi2Integer;
typedef int fmi2Boolean;
typedef unsigned int fmi2ValueReference;
typedef char fmi2Byte;

enum fmi2Status {
    fmi2OK = 0,
    fmi2Warning = 1,
    fmi2Discard = 2,
    fmi2Error = 3,
    fmi2Fatal = 4,
    fmi2Pending = 5,
};
enum fmi2Type { fmi2ModelExchange = 0, fmi2CoSimulation = 1 };
enum fmi2StatusKind {
    fmi2DoStepStatus,
    fmi2PendingStatus,
    fmi2LastSuccessfulTime,
    fmi2Terminated,
};

typedef void (*fmi2CallbackLogger)(
    fmi2ComponentEnvironment,
    fmi2String,
    fmi2Status,
    fmi2String,
    fmi2String,
    ...);
typedef void* (*fmi2CallbackAllocateMemory)(std::size_t, std::size_t);
typedef void (*fmi2CallbackFreeMemory)(void*);
typedef void (*fmi2StepFinished)(fmi2ComponentEnvironment, fmi2Status);
struct fmi2CallbackFunctions {
    fmi2CallbackLogger logger;
    fmi2CallbackAllocateMemory allocateMemory;
    fmi2CallbackFreeMemory freeMemory;
    fmi2StepFinished stepFinished;
    fmi2ComponentEnvironment componentEnvironment;
};
}
