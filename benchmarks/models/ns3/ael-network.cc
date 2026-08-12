#include "ns3/core-module.h"

#include <algorithm>
#include <iostream>

using namespace ns3;

int main(int argc, char* argv[]) {
    double faultScale = 0.0;
    double rfLossDb = 0.0;
    uint32_t seed = 1;
    uint64_t stopUs = 1000;
    CommandLine command(__FILE__);
    command.AddValue("fault_scale", "Functional interference/partition control", faultScale);
    command.AddValue("rf_loss_db", "Additional RF path loss", rfLossDb);
    command.AddValue("seed", "Deterministic experiment seed", seed);
    command.AddValue("stopUs", "AEL synchronization time", stopUs);
    command.Parse(argc, argv);
    RngSeedManager::SetSeed(seed == 0 ? 1 : seed);
    const double loss = std::clamp(0.01 + faultScale * 0.45 + rfLossDb / 80.0, 0.0, 0.99);
    const double retries = loss * 12.0;
    const double latencyMs = 2.0 + loss * 80.0;
    const double failure = loss > 0.25 ? 1.0 : 0.0;
    Simulator::Stop(MicroSeconds(stopUs));
    Simulator::Run();
    std::cout << "AEL_METRIC packet_loss=" << loss << "\n";
    std::cout << "AEL_METRIC retries=" << retries << "\n";
    std::cout << "AEL_METRIC latency_ms=" << latencyMs << "\n";
    std::cout << "AEL_METRIC failure=" << failure << "\n";
    std::cout << "AEL_EVENT ns3.network {\"calibrated\":false}\n";
    Simulator::Destroy();
    return 0;
}
