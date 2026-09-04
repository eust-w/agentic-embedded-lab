package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	capability "github.com/eust-w/agentic-embedded-lab/internal/acceptance"
	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/lab"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/invopop/jsonschema"
)

func main() {
	output := flag.String("output", "schemas/v2", "schema output directory")
	flag.Parse()
	if err := os.MkdirAll(*output, 0o755); err != nil {
		fatal(err)
	}
	models := map[string]any{
		"thread":                     protocol.Thread{},
		"turn":                       protocol.Turn{},
		"item":                       protocol.Item{},
		"approval-request":           protocol.ApprovalRequest{},
		"agent-spec":                 protocol.AgentSpec{},
		"automation-spec":            protocol.AutomationSpec{},
		"plugin-manifest":            plugins.Manifest{},
		"ael-problem":                ael.Problem{},
		"ael-system":                 ael.System{},
		"ael-experiment":             ael.Experiment{},
		"ael-event":                  ael.Event{},
		"ael-evidence-bundle":        ael.EvidenceBundle{},
		"ael-claim":                  ael.Claim{},
		"validation-envelope":        ael.ValidationEnvelope{},
		"calibration-record":         ael.CalibrationRecord{},
		"instrument-evidence":        ael.InstrumentEvidence{},
		"lab-operation-request":      lab.OperationRequest{},
		"lab-operation-result":       lab.OperationResult{},
		"hardware-behavior-ir":       modeling.IR{},
		"model-package":              modeling.Package{},
		"model-generation-request":   modeling.GenerationRequest{},
		"model-conformance-evidence": modeling.ConformanceEvidence{},
		"capability-acceptance":      capability.CapabilityAcceptance{},
	}
	reflector := &jsonschema.Reflector{AllowAdditionalProperties: false, DoNotReference: true}
	for name, model := range models {
		schema := reflector.Reflect(model)
		payload, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			fatal(err)
		}
		payload = append(payload, '\n')
		if err := os.WriteFile(filepath.Join(*output, name+".schema.json"), payload, 0o644); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
