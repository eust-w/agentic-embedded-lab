package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"gopkg.in/yaml.v3"
)

type legacyPort struct {
	Name      string `yaml:"name"`
	Direction string `yaml:"direction"`
	DataType  string `yaml:"data_type"`
	Unit      string `yaml:"unit"`
}

type legacyComponent struct {
	ID          string         `yaml:"id"`
	Backend     ael.Backend    `yaml:"backend"`
	Model       string         `yaml:"model"`
	StepUS      int64          `yaml:"step_us"`
	EventDriven bool           `yaml:"event_driven"`
	Ports       []legacyPort   `yaml:"ports"`
	Properties  map[string]any `yaml:"properties"`
}

type legacyConnection struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	Unit   string `yaml:"unit"`
}

type legacySystem struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Components  []legacyComponent  `yaml:"components"`
	Connections []legacyConnection `yaml:"connections"`
}

type legacyStimulus struct {
	AtUS   int64   `yaml:"at_us"`
	Target string  `yaml:"target"`
	Value  float64 `yaml:"value"`
	Unit   string  `yaml:"unit"`
}

type legacyFault struct {
	AtUS       int64          `yaml:"at_us"`
	Target     string         `yaml:"target"`
	Kind       string         `yaml:"kind"`
	Parameters map[string]any `yaml:"parameters"`
}

type legacyAssertion struct {
	Metric   string  `yaml:"metric"`
	Operator string  `yaml:"operator"`
	Expected float64 `yaml:"expected"`
}

type legacyExperiment struct {
	Name        string            `yaml:"name"`
	System      string            `yaml:"system"`
	DurationUS  int64             `yaml:"duration_us"`
	MacroStepUS int64             `yaml:"macro_step_us"`
	TimeoutS    int64             `yaml:"timeout_s"`
	Seed        int64             `yaml:"seed"`
	Stimuli     []legacyStimulus  `yaml:"stimuli"`
	Faults      []legacyFault     `yaml:"faults"`
	Assertions  []legacyAssertion `yaml:"assertions"`
}

func main() {
	source := flag.String("source", "benchmarks", "legacy benchmark directory")
	output := flag.String("output", "benchmarks/v2", "v2 benchmark directory")
	flag.Parse()
	if err := migrateSystems(filepath.Join(*source, "systems"), filepath.Join(*output, "systems")); err != nil {
		fatal(err)
	}
	if err := migrateExperiments(filepath.Join(*source, "cases"), filepath.Join(*source, "systems"), filepath.Join(*output, "experiments")); err != nil {
		fatal(err)
	}
}

func migrateSystems(source, output string) error {
	files, err := filepath.Glob(filepath.Join(source, "*.yaml"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	for _, path := range files {
		var legacy legacySystem
		if err := decodeYAML(path, &legacy); err != nil {
			return err
		}
		system := ael.System{APIVersion: ael.APIVersion, ID: legacy.Name}
		for _, component := range legacy.Components {
			converted := ael.Component{ID: component.ID, Backend: component.Backend, Model: component.Model, StepUS: component.StepUS, EventDriven: component.EventDriven, Properties: component.Properties,
				Fidelity: ael.Fidelity{Firmware: ael.FidelityFunctional, Register: ael.FidelitySynthetic, Protocol: ael.FidelityFunctional, Timing: ael.FidelityFunctional, Physical: ael.FidelityUnsupported, HardwareValidated: false, Limitations: []string{"migrated functional model; no hardware calibration"}}}
			for _, port := range component.Ports {
				converted.Ports = append(converted.Ports, ael.Port{Name: port.Name, Direction: port.Direction, Type: port.DataType, Unit: port.Unit})
			}
			system.Components = append(system.Components, converted)
		}
		for _, connection := range legacy.Connections {
			sourceComponent, sourcePort, ok := strings.Cut(connection.Source, ".")
			if !ok {
				return fmt.Errorf("invalid source connection %s", connection.Source)
			}
			targetComponent, targetPort, ok := strings.Cut(connection.Target, ".")
			if !ok {
				return fmt.Errorf("invalid target connection %s", connection.Target)
			}
			system.Connections = append(system.Connections, ael.Connection{SourceComponent: sourceComponent, SourcePort: sourcePort, TargetComponent: targetComponent, TargetPort: targetPort, Unit: connection.Unit})
		}
		if err := writeYAML(filepath.Join(output, filepath.Base(path)), system); err != nil {
			return err
		}
	}
	return nil
}

func migrateExperiments(source, systems, output string) error {
	files, err := filepath.Glob(filepath.Join(source, "*", "*.yaml"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	systemIDs, err := readLegacySystemIDs(systems)
	if err != nil {
		return err
	}
	for _, path := range files {
		var legacy legacyExperiment
		if err := decodeYAML(path, &legacy); err != nil {
			return err
		}
		systemFile := strings.TrimSuffix(filepath.Base(legacy.System), filepath.Ext(legacy.System))
		systemID := systemIDs[systemFile]
		if systemID == "" {
			return fmt.Errorf("experiment %s references unknown system %s", path, legacy.System)
		}
		experiment := ael.Experiment{APIVersion: ael.APIVersion, ID: legacy.Name, SystemID: systemID, DurationUS: legacy.DurationUS, MacroStepUS: legacy.MacroStepUS, Seed: legacy.Seed, Timeout: time.Duration(legacy.TimeoutS) * time.Second,
			RequiredFidelity: ael.Fidelity{Firmware: ael.FidelityFunctional, Register: ael.FidelitySynthetic, Protocol: ael.FidelityFunctional, Timing: ael.FidelityFunctional, Physical: ael.FidelityUnsupported, HardwareValidated: false}}
		for _, stimulus := range legacy.Stimuli {
			experiment.Stimuli = append(experiment.Stimuli, ael.Stimulus{AtUS: stimulus.AtUS, Target: stimulus.Target, Value: stimulus.Value, Unit: stimulus.Unit})
		}
		for _, fault := range legacy.Faults {
			experiment.Faults = append(experiment.Faults, ael.Fault{AtUS: fault.AtUS, Target: fault.Target, Kind: fault.Kind, Parameters: fault.Parameters})
		}
		for index, assertion := range legacy.Assertions {
			operator := map[string]string{"eq": "==", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[assertion.Operator]
			if operator == "" {
				return fmt.Errorf("unsupported assertion operator %s", assertion.Operator)
			}
			experiment.Assertions = append(experiment.Assertions, ael.Assertion{ID: fmt.Sprintf("A%02d", index+1), Metric: assertion.Metric, Operator: operator, Expected: assertion.Expected})
		}
		name := filepath.Base(filepath.Dir(path)) + "-" + filepath.Base(path)
		if err := writeYAML(filepath.Join(output, name), experiment); err != nil {
			return err
		}
	}
	return nil
}

func readLegacySystemIDs(directory string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.yaml"))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(files))
	for _, path := range files {
		var system legacySystem
		if err := decodeYAML(path, &system); err != nil {
			return nil, err
		}
		result[strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))] = system.Name
	}
	return result, nil
}

func decodeYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeYAML(path string, value any) error {
	jsonPayload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var normalized any
	if err := json.Unmarshal(jsonPayload, &normalized); err != nil {
		return err
	}
	payload, err := yaml.Marshal(normalized)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
