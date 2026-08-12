cmake_minimum_required(VERSION 3.20)
project(ael_fmi_proxies LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

function(add_ael_proxy target backend non_rollback event_driven)
  add_library(${target} SHARED ael_fmi_proxy.cpp)
  target_compile_definitions(${target} PRIVATE
    AEL_PROXY_NAME="${target}"
    AEL_BACKEND_NAME="${backend}"
    AEL_NON_ROLLBACK=${non_rollback}
    AEL_EVENT_DRIVEN=${event_driven}
  )
  target_compile_options(${target} PRIVATE -Wall -Wextra -Wpedantic -Werror)
  set_target_properties(${target} PROPERTIES PREFIX "")
endfunction()

add_ael_proxy(RenodeFmu renode 1 0)
add_ael_proxy(NgspiceFmu ngspice 0 0)
add_ael_proxy(ModelicaFmu openmodelica 0 0)
add_ael_proxy(Ns3Fmu ns3 1 0)
add_ael_proxy(OpenEmsFmu openems 1 1)
