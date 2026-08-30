package tools

import (
	"context"
	"errors"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type AELRouteTool struct{}

func (AELRouteTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "embedded_problem_route", Description: "Resolve an embedded problem category to executable AEL backends and model packs without running tools", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}, "expected_claim": map[string]any{"type": "string"}}, "required": []string{"id", "category"}}}
}
func (AELRouteTool) Operation(map[string]any) approval.Operation {
	return approval.Operation{Tool: "embedded_problem_route", Action: "inspect", Risk: protocol.RiskLow}
}
func (AELRouteTool) Execute(_ context.Context, arguments map[string]any) (Result, error) {
	id, _ := arguments["id"].(string)
	category, _ := arguments["category"].(string)
	claim, _ := arguments["expected_claim"].(string)
	if id == "" || category == "" {
		return Result{}, errors.New("problem id and category are required")
	}
	route, err := ael.RouteProblem(ael.Problem{APIVersion: ael.APIVersion, ID: id, Category: category, ExpectedClaim: claim})
	return Result{Output: map[string]any{"route": route}}, err
}
