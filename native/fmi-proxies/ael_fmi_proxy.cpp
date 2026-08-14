// SPDX-License-Identifier: Apache-2.0
#include "fmi2_types.h"

#include <algorithm>
#include <cctype>
#include <cerrno>
#include <cstdlib>
#include <cstring>
#include <map>
#include <memory>
#include <netdb.h>
#include <sstream>
#include <string>
#include <string_view>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <vector>

#ifndef AEL_PROXY_NAME
#define AEL_PROXY_NAME "AelFmu"
#endif
#ifndef AEL_BACKEND_NAME
#define AEL_BACKEND_NAME "unknown"
#endif
#ifndef AEL_NON_ROLLBACK
#define AEL_NON_ROLLBACK 1
#endif
#ifndef AEL_EVENT_DRIVEN
#define AEL_EVENT_DRIVEN 0
#endif

namespace {
struct State {
    std::string instanceName;
    fmi2CallbackFunctions callbacks{};
    double time{0.0};
    std::map<fmi2ValueReference, double> reals;
    std::map<fmi2ValueReference, int> integers;
    std::map<fmi2ValueReference, int> booleans;
    std::map<fmi2ValueReference, std::string> strings;
};

void log(State* state, fmi2Status status, const char* category, const std::string& message) {
    if (state && state->callbacks.logger) {
        state->callbacks.logger(
            state->callbacks.componentEnvironment,
            state->instanceName.c_str(),
            status,
            category,
            "%s",
            message.c_str());
    }
}

std::string socketEnvironmentName() {
    std::string name = "AEL_FMI_SOCKET_";
    for (char value : std::string(AEL_BACKEND_NAME)) {
        name.push_back(value == '-' ? '_' : static_cast<char>(std::toupper(value)));
    }
    return name;
}

int connectEndpoint(const char* endpoint, std::string& error) {
    const std::string value(endpoint);
    if (value.rfind("tcp://", 0) == 0) {
        const auto delimiter = value.rfind(':');
        if (delimiter == std::string::npos || delimiter <= 6 || delimiter + 1 >= value.size()) {
            error = "invalid TCP FMI endpoint";
            return -1;
        }
        const auto host = value.substr(6, delimiter - 6);
        const auto port = value.substr(delimiter + 1);
        addrinfo hints{};
        hints.ai_family = AF_UNSPEC;
        hints.ai_socktype = SOCK_STREAM;
        addrinfo* addresses = nullptr;
        const auto lookup = ::getaddrinfo(host.c_str(), port.c_str(), &hints, &addresses);
        if (lookup != 0) {
            error = ::gai_strerror(lookup);
            return -1;
        }
        int connected = -1;
        for (auto* address = addresses; address; address = address->ai_next) {
            const int candidate = ::socket(
                address->ai_family, address->ai_socktype, address->ai_protocol);
            if (candidate < 0) {
                continue;
            }
            if (::connect(candidate, address->ai_addr, address->ai_addrlen) == 0) {
                connected = candidate;
                break;
            }
            ::close(candidate);
        }
        ::freeaddrinfo(addresses);
        if (connected < 0) {
            error = std::strerror(errno);
        }
        return connected;
    }

    const int fd = ::socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        error = std::strerror(errno);
        return -1;
    }
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    if (value.size() >= sizeof(address.sun_path)) {
        ::close(fd);
        error = "UNIX socket path is too long";
        return -1;
    }
    std::strncpy(address.sun_path, value.c_str(), sizeof(address.sun_path) - 1);
    if (::connect(fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) != 0) {
        error = std::strerror(errno);
        ::close(fd);
        return -1;
    }
    return fd;
}

bool exchange(State* state, double current, double step) {
    const auto variable = socketEnvironmentName();
    const char* path = std::getenv(variable.c_str());
    if (!path) {
        log(state, fmi2Error, "transport", variable + " is not configured");
        return false;
    }
    std::string transportError;
    const int fd = connectEndpoint(path, transportError);
    if (fd < 0) {
        log(state, fmi2Error, "transport", transportError);
        return false;
    }
    std::ostringstream request;
    request << "STEP " << state->instanceName << ' ' << current << ' ' << step;
    for (const auto& [reference, value] : state->reals) {
        request << " r" << reference << '=' << value;
    }
    for (const auto& [reference, value] : state->integers) {
        request << " i" << reference << '=' << value;
    }
    for (const auto& [reference, value] : state->booleans) {
        request << " b" << reference << '=' << value;
    }
    request << '\n';
    const auto payload = request.str();
    if (::write(fd, payload.data(), payload.size()) != static_cast<ssize_t>(payload.size())) {
        ::close(fd);
        log(state, fmi2Error, "transport", "failed to write complete step request");
        return false;
    }
    char buffer[8192]{};
    const auto size = ::read(fd, buffer, sizeof(buffer) - 1);
    ::close(fd);
    if (size <= 0) {
        log(state, fmi2Error, "transport", "backend returned no response");
        return false;
    }
    std::istringstream response(std::string(buffer, static_cast<std::size_t>(size)));
    std::string status;
    response >> status;
    if (status != "OK") {
        log(state, fmi2Error, "backend", std::string(buffer, static_cast<std::size_t>(size)));
        return false;
    }
    std::string item;
    while (response >> item) {
        if (item.size() < 4 || (item[0] != 'r' && item[0] != 'i' && item[0] != 'b')) {
            continue;
        }
        const auto delimiter = item.find('=');
        if (delimiter == std::string::npos) {
            continue;
        }
        const auto reference = static_cast<fmi2ValueReference>(
            std::stoul(item.substr(1, delimiter - 1)));
        if (item[0] == 'r') {
            state->reals[reference] = std::stod(item.substr(delimiter + 1));
        } else if (item[0] == 'i') {
            state->integers[reference] = std::stoi(item.substr(delimiter + 1));
        } else {
            state->booleans[reference] = std::stoi(item.substr(delimiter + 1));
        }
    }
    return true;
}

template <typename Map, typename Value>
fmi2Status setValues(Map& values, const fmi2ValueReference vr[], std::size_t count, const Value input[]) {
    if (!vr || !input) {
        return fmi2Error;
    }
    for (std::size_t index = 0; index < count; ++index) {
        values[vr[index]] = input[index];
    }
    return fmi2OK;
}

template <typename Map, typename Value>
fmi2Status getValues(const Map& values, const fmi2ValueReference vr[], std::size_t count, Value output[]) {
    if (!vr || !output) {
        return fmi2Error;
    }
    for (std::size_t index = 0; index < count; ++index) {
        const auto item = values.find(vr[index]);
        output[index] = item == values.end() ? Value{} : item->second;
    }
    return fmi2OK;
}
}  // namespace

extern "C" {
FMI2_EXPORT fmi2String fmi2GetTypesPlatform() { return "default"; }
FMI2_EXPORT fmi2String fmi2GetVersion() { return "2.0"; }
FMI2_EXPORT fmi2Status fmi2SetDebugLogging(
    fmi2Component component,
    fmi2Boolean,
    std::size_t,
    const fmi2String[]) {
    return component ? fmi2OK : fmi2Error;
}
FMI2_EXPORT fmi2Component fmi2Instantiate(
    fmi2String instanceName,
    fmi2Type fmuType,
    fmi2String,
    fmi2String,
    const fmi2CallbackFunctions* callbacks,
    fmi2Boolean,
    fmi2Boolean) {
    if (!instanceName || fmuType != fmi2CoSimulation || !callbacks) {
        return nullptr;
    }
    auto state = std::make_unique<State>();
    state->instanceName = instanceName;
    state->callbacks = *callbacks;
    log(state.get(), fmi2OK, "lifecycle", std::string("instantiated ") + AEL_PROXY_NAME);
    return state.release();
}
FMI2_EXPORT void fmi2FreeInstance(fmi2Component component) {
    delete static_cast<State*>(component);
}
FMI2_EXPORT fmi2Status fmi2SetupExperiment(
    fmi2Component component, fmi2Boolean, fmi2Real, fmi2Real start, fmi2Boolean, fmi2Real) {
    if (!component) return fmi2Error;
    static_cast<State*>(component)->time = start;
    return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2EnterInitializationMode(fmi2Component component) {
    return component ? fmi2OK : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2ExitInitializationMode(fmi2Component component) {
    return component ? fmi2OK : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2Terminate(fmi2Component component) {
    return component ? fmi2OK : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2Reset(fmi2Component component) {
    if (!component) return fmi2Error;
    auto* state = static_cast<State*>(component);
    state->time = 0.0;
    state->reals.clear();
    state->integers.clear();
    state->booleans.clear();
    state->strings.clear();
    return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2DoStep(
    fmi2Component component, fmi2Real current, fmi2Real step, fmi2Boolean) {
    if (!component || step <= 0.0) return fmi2Error;
    auto* state = static_cast<State*>(component);
    if (!exchange(state, current, step)) return fmi2Error;
    state->time = current + step;
    return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2CancelStep(fmi2Component) { return fmi2Discard; }
FMI2_EXPORT fmi2Status fmi2SetReal(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, const fmi2Real v[]) {
    return c ? setValues(static_cast<State*>(c)->reals, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetReal(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, fmi2Real v[]) {
    return c ? getValues(static_cast<State*>(c)->reals, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2SetInteger(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, const fmi2Integer v[]) {
    return c ? setValues(static_cast<State*>(c)->integers, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetInteger(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, fmi2Integer v[]) {
    return c ? getValues(static_cast<State*>(c)->integers, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2SetBoolean(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, const fmi2Boolean v[]) {
    return c ? setValues(static_cast<State*>(c)->booleans, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetBoolean(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, fmi2Boolean v[]) {
    return c ? getValues(static_cast<State*>(c)->booleans, vr, n, v) : fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2SetString(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, const fmi2String v[]) {
    if (!c || !vr || !v) return fmi2Error;
    auto& values = static_cast<State*>(c)->strings;
    for (std::size_t index = 0; index < n; ++index) values[vr[index]] = v[index] ? v[index] : "";
    return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2GetString(fmi2Component c, const fmi2ValueReference vr[], std::size_t n, fmi2String v[]) {
    if (!c || !vr || !v) return fmi2Error;
    auto& values = static_cast<State*>(c)->strings;
    for (std::size_t index = 0; index < n; ++index) v[index] = values[vr[index]].c_str();
    return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2GetFMUstate(fmi2Component component, fmi2FMUstate* state) {
#if AEL_NON_ROLLBACK
    (void)component; (void)state; return fmi2Error;
#else
    if (!component || !state) return fmi2Error;
    *state = new State(*static_cast<State*>(component));
    return fmi2OK;
#endif
}
FMI2_EXPORT fmi2Status fmi2SetFMUstate(fmi2Component component, fmi2FMUstate state) {
#if AEL_NON_ROLLBACK
    (void)component; (void)state; return fmi2Error;
#else
    if (!component || !state) return fmi2Error;
    *static_cast<State*>(component) = *static_cast<State*>(state);
    return fmi2OK;
#endif
}
FMI2_EXPORT fmi2Status fmi2FreeFMUstate(fmi2Component, fmi2FMUstate* state) {
#if AEL_NON_ROLLBACK
    (void)state; return fmi2Error;
#else
    if (!state) return fmi2Error;
    delete static_cast<State*>(*state); *state = nullptr; return fmi2OK;
#endif
}
// FMI 2.0 declares these entry points optional according to the capability
// flags in modelDescription.xml.  OMSimulator 2.1.3's bundled fmi4c loader
// nevertheless resolves the complete function table before instantiation.
// Keep the symbols present and report unsupported operations at runtime; this
// preserves the advertised capabilities without making the FMU unloadable.
FMI2_EXPORT fmi2Status fmi2SerializedFMUstateSize(
    fmi2Component, fmi2FMUstate, std::size_t*) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2SerializeFMUstate(
    fmi2Component, fmi2FMUstate, fmi2Byte[], std::size_t) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2DeSerializeFMUstate(
    fmi2Component, const fmi2Byte[], std::size_t, fmi2FMUstate*) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetDirectionalDerivative(
    fmi2Component,
    const fmi2ValueReference[],
    std::size_t,
    const fmi2ValueReference[],
    std::size_t,
    const fmi2Real[],
    fmi2Real[]) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2SetRealInputDerivatives(
    fmi2Component,
    const fmi2ValueReference[],
    std::size_t,
    const fmi2Integer[],
    const fmi2Real[]) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetRealOutputDerivatives(
    fmi2Component,
    const fmi2ValueReference[],
    std::size_t,
    const fmi2Integer[],
    fmi2Real[]) {
    return fmi2Error;
}
FMI2_EXPORT fmi2Status fmi2GetStatus(fmi2Component, fmi2StatusKind, fmi2Status*) { return fmi2Discard; }
FMI2_EXPORT fmi2Status fmi2GetRealStatus(fmi2Component c, fmi2StatusKind kind, fmi2Real* value) {
    if (!c || !value || kind != fmi2LastSuccessfulTime) return fmi2Discard;
    *value = static_cast<State*>(c)->time; return fmi2OK;
}
FMI2_EXPORT fmi2Status fmi2GetIntegerStatus(fmi2Component, fmi2StatusKind, fmi2Integer*) { return fmi2Discard; }
FMI2_EXPORT fmi2Status fmi2GetBooleanStatus(fmi2Component, fmi2StatusKind, fmi2Boolean*) { return fmi2Discard; }
FMI2_EXPORT fmi2Status fmi2GetStringStatus(fmi2Component, fmi2StatusKind, fmi2String*) { return fmi2Discard; }
}
