#include "ns3/core-module.h"
#include "ns3/energy-module.h"
#include "ns3/lr-wpan-module.h"
#include "ns3/mobility-module.h"
#include "ns3/network-module.h"
#include "ns3/wifi-module.h"

#include <algorithm>
#include <iostream>
#include <vector>

using namespace ns3;

namespace {
uint32_t g_received = 0;
uint32_t g_phyTx = 0;

void RxTrace(Ptr<const Packet>) { ++g_received; }
void TxTrace(Ptr<const Packet>) { ++g_phyTx; }

void SendPacket(Ptr<NetDevice> source, Address destination)
{
    source->Send(Create<Packet>(48), destination, 0x0800);
}

void SetPosition(Ptr<Node> node, double x)
{
    node->GetObject<MobilityModel>()->SetPosition(Vector(x, 0.0, 0.0));
}
} // namespace

int main(int argc, char* argv[])
{
    double interferenceDbm = -95.0;
    double partitionMs = 0.0;
    double rfLossDb = 0.0;
    uint32_t retryLimit = 8;
    uint32_t protocol = 0;
    uint32_t seed = 1;
    uint64_t stopUs = 1000;
    CommandLine command(__FILE__);
    command.AddValue("interference_dbm", "Interferer transmit condition in dBm", interferenceDbm);
    command.AddValue("partition_ms", "Receiver partition duration", partitionMs);
    command.AddValue("retry_limit", "Firmware retry budget", retryLimit);
    command.AddValue("protocol", "0=802.15.4, 1=Wi-Fi", protocol);
    command.AddValue("rf_loss_db", "Additional RF path loss from EM solver", rfLossDb);
    command.AddValue("seed", "Deterministic experiment seed", seed);
    command.AddValue("stopUs", "AEL synchronization time", stopUs);
    command.Parse(argc, argv);
    RngSeedManager::SetSeed(seed == 0 ? 1 : seed);
    RngSeedManager::SetRun(1);

    NodeContainer nodes;
    nodes.Create(3); // source, receiver, in-channel interferer
    MobilityHelper mobility;
    mobility.SetMobilityModel("ns3::ConstantPositionMobilityModel");
    mobility.Install(nodes);
    SetPosition(nodes.Get(0), 0.0);
    SetPosition(nodes.Get(1), 1.0 + rfLossDb * 6.0);
    SetPosition(nodes.Get(2), interferenceDbm > -60.0 ? 0.5 : 500.0);

    NetDeviceContainer devices;
    std::vector<Ptr<energy::SimpleDeviceEnergyModel>> energyModels;
    if (protocol == 0)
    {
        LrWpanHelper helper;
        devices = helper.Install(nodes);
        helper.CreateAssociatedPan(devices, 0x1234);
        auto sourceMac = DynamicCast<lrwpan::LrWpanNetDevice>(devices.Get(0))->GetMac();
        auto receiverMac = DynamicCast<lrwpan::LrWpanNetDevice>(devices.Get(1))->GetMac();
        sourceMac->TraceConnectWithoutContext("MacTx", MakeCallback(&TxTrace));
        receiverMac->TraceConnectWithoutContext("MacRx", MakeCallback(&RxTrace));
        BasicEnergySourceHelper energy;
        energy.Set("BasicEnergySourceInitialEnergyJ", DoubleValue(100.0));
        energy::EnergySourceContainer sources = energy.Install(nodes);
        for (uint32_t index = 0; index < devices.GetN(); ++index)
        {
            auto model = CreateObject<energy::SimpleDeviceEnergyModel>();
            model->SetEnergySource(sources.Get(index));
            model->SetNode(nodes.Get(index));
            model->SetCurrentA(index == 2 ? 0.020 : 0.012);
            sources.Get(index)->AppendDeviceEnergyModel(model);
            energyModels.push_back(model);
        }
    }
    else
    {
        YansWifiChannelHelper channel = YansWifiChannelHelper::Default();
        YansWifiPhyHelper phy;
        phy.SetChannel(channel.Create());
        WifiHelper wifi;
        wifi.SetStandard(WIFI_STANDARD_80211g);
        WifiMacHelper mac;
        mac.SetType("ns3::AdhocWifiMac");
        devices = wifi.Install(phy, mac, nodes);
        auto sourceMac = DynamicCast<WifiNetDevice>(devices.Get(0))->GetMac();
        auto receiverMac = DynamicCast<WifiNetDevice>(devices.Get(1))->GetMac();
        sourceMac->TraceConnectWithoutContext("MacTx", MakeCallback(&TxTrace));
        receiverMac->TraceConnectWithoutContext("MacRx", MakeCallback(&RxTrace));
    }
    constexpr uint32_t packets = 20;
    for (uint32_t index = 0; index < packets; ++index)
    {
        const Time at = MilliSeconds(10 + index * 10);
        Simulator::Schedule(at, &SendPacket, devices.Get(0), devices.Get(1)->GetAddress());
        if (interferenceDbm > -90.0)
        {
            Simulator::Schedule(at, &SendPacket, devices.Get(2), devices.Get(0)->GetAddress());
        }
    }
    if (partitionMs > 0.0)
    {
        Simulator::Schedule(MilliSeconds(40), &SetPosition, nodes.Get(1), 10000.0);
        Simulator::Schedule(
            MilliSeconds(40 + partitionMs), &SetPosition, nodes.Get(1), 1.0 + rfLossDb * 6.0);
    }
    const double durationMs = std::max(250.0, 60.0 + partitionMs);
    Simulator::Stop(MilliSeconds(durationMs));
    Simulator::Run();

    const double packetLoss = 1.0 - static_cast<double>(g_received) / packets;
    const uint32_t retries = g_phyTx > packets ? g_phyTx - packets : packets - g_received;
    const double latencyMs = partitionMs > 0.0 ? partitionMs + 10.0 : 10.0 + retries;
    const double failure = (g_received < packets || retries > retryLimit) ? 1.0 : 0.0;
    double energyJ = 0.0;
    for (const auto& model : energyModels)
    {
        energyJ += model->GetTotalEnergyConsumption();
    }
    std::cout << "AEL_METRIC packet_loss=" << std::clamp(packetLoss, 0.0, 1.0) << "\n";
    std::cout << "AEL_METRIC retries=" << retries << "\n";
    std::cout << "AEL_METRIC latency_ms=" << latencyMs << "\n";
    std::cout << "AEL_METRIC energy_j=" << energyJ << "\n";
    std::cout << "AEL_METRIC failure=" << failure << "\n";
    std::cout << "AEL_EVENT ns3.protocol {\"protocol\":\""
              << (protocol == 0 ? "802.15.4" : "wifi")
              << "\",\"received\":" << g_received << ",\"phy_tx\":" << g_phyTx
              << ",\"calibrated\":false}\n";
    Simulator::Destroy();
    return 0;
}
