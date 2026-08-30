package tools

import "testing"

func TestCapabilityToolDefinitionsAreReadOnlyAndTyped(t *testing.T) {
	for _, name := range []string{"inspect_capabilities", "get_acceptance_status", "get_acceptance_evidence"} {
		tool := CapabilityTool{Workspace: t.TempDir(), Mode: name}
		if tool.Definition().Name != name || tool.Operation(nil).Action != "inspect" {
			t.Fatal(name)
		}
	}
}
