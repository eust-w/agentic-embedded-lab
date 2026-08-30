package ael

import "testing"

func TestRouteProblemFindsExtensionPacksAndPreservesClaimBoundary(t *testing.T) {
	for category, pack := range map[string]string{"rtl": "rtl-timer", "motor": "electromechanical-motor", "battery": "battery-aging", "sensor": "sensor-error", "pcb": "pcb-parasitic", "emc": "emc-eft"} {
		route, err := RouteProblem(Problem{APIVersion: APIVersion, ID: category, Category: category})
		if err != nil || !route.Executable || len(route.ReferencePacks) != 1 || route.ReferencePacks[0] != pack {
			t.Fatalf("route %s: %#v %v", category, route, err)
		}
	}
	route, err := RouteProblem(Problem{APIVersion: APIVersion, ID: "p", Category: "motor", ExpectedClaim: "hardware validated"})
	if err != nil || route.ValidationGap == "" {
		t.Fatalf("hardware boundary missing: %#v %v", route, err)
	}
}

func TestRouteProblemReturnsExplicitUnknownGap(t *testing.T) {
	route, err := RouteProblem(Problem{APIVersion: APIVersion, ID: "unknown", Category: "quantum"})
	if err != nil || route.Executable || route.ValidationGap == "" {
		t.Fatalf("unknown category did not fail explicitly: %#v %v", route, err)
	}
}
