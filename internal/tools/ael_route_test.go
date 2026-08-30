package tools

import (
	"context"
	"testing"
)

func TestAELRouteToolExposesExecutableExtension(t *testing.T) {
	result, err := (AELRouteTool{}).Execute(context.Background(), map[string]any{"id": "motor", "category": "motor", "expected_claim": "simulation"})
	if err != nil || result.Output["route"] == nil {
		t.Fatalf("route: %#v %v", result, err)
	}
}
