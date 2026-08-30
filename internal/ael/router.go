package ael

import (
	"errors"
	"sort"
	"strings"
)

type CapabilityRoute struct {
	ProblemID      string    `json:"problem_id"`
	Category       string    `json:"category"`
	Backends       []Backend `json:"backends"`
	ReferencePacks []string  `json:"reference_packs"`
	Executable     bool      `json:"executable"`
	ValidationGap  string    `json:"validation_gap,omitempty"`
}

func RouteProblem(problem Problem) (CapabilityRoute, error) {
	if problem.APIVersion != APIVersion || problem.ID == "" || strings.TrimSpace(problem.Category) == "" {
		return CapabilityRoute{}, errors.New("v2 problem id and category are required")
	}
	category := strings.ToLower(strings.TrimSpace(problem.Category))
	route := CapabilityRoute{ProblemID: problem.ID, Category: category}
	add := func(backend Backend, pack string) {
		route.Backends = append(route.Backends, backend)
		route.ReferencePacks = append(route.ReferencePacks, pack)
	}
	switch category {
	case "firmware", "digital", "rtos", "mcu", "peripheral":
		add(BackendZephyr, "digital-build")
		add(BackendRenode, "virtual-mcu")
	case "power", "analog", "pcb", "signal-integrity", "emc":
		add(BackendNgspice, map[string]string{"pcb": "pcb-parasitic", "emc": "emc-eft"}[category])
	case "thermal":
		add(BackendModelica, "thermal-domain")
	case "battery":
		add(BackendModelica, "battery-aging")
	case "motor", "electromechanical", "mechanical":
		add(BackendModelica, "electromechanical-motor")
	case "sensor", "sensor-dynamics":
		add(BackendModelica, "sensor-error")
	case "network", "802.15.4", "wifi":
		add(BackendNS3, "network-scenario")
	case "rf", "electromagnetic", "antenna":
		add(BackendOpenEMS, "antenna-detuning")
	case "rtl", "fpga", "systemverilog":
		add(BackendVerilator, "rtl-timer")
	case "cross-domain", "multiphysics":
		route.Backends = []Backend{BackendRenode, BackendNgspice, BackendModelica, BackendNS3, BackendOpenEMS}
		route.ReferencePacks = []string{"five-domain-fmi-ssp"}
	default:
		route.ValidationGap = "no executable backend/model pack is registered for this category"
		return route, nil
	}
	for index, pack := range route.ReferencePacks {
		if pack == "" {
			route.ReferencePacks[index] = "generic-" + category
		}
	}
	sort.Slice(route.Backends, func(i, j int) bool { return route.Backends[i] < route.Backends[j] })
	sort.Strings(route.ReferencePacks)
	route.Executable = len(route.Backends) > 0
	if strings.Contains(strings.ToLower(problem.ExpectedClaim), "hardware") || strings.Contains(strings.ToLower(problem.ExpectedClaim), "production") {
		route.ValidationGap = "execution is available, but hardware/production claims require a signed Validation Envelope"
	}
	return route, nil
}
