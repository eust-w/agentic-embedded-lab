package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
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
	if err := specializeFirmwareSystems(filepath.Join(*output, "systems")); err != nil {
		fatal(err)
	}
	if err := migrateExperiments(filepath.Join(*source, "cases"), filepath.Join(*source, "systems"), filepath.Join(*output, "experiments")); err != nil {
		fatal(err)
	}
	if err := migrateCatalog(filepath.Join(*source, "catalog.yaml"), filepath.Join(*output, "catalog.yaml")); err != nil {
		fatal(err)
	}
}

func migrateCatalog(source, output string) error {
	catalog, err := benchmark.Load(".", source)
	if err != nil {
		return err
	}
	for index := range catalog.Cases {
		item := &catalog.Cases[index]
		prefix := fmt.Sprintf("%02d-%s", item.ID, item.Slug)
		item.FaultyExperiment = filepath.ToSlash(filepath.Join("benchmarks", "v2", "experiments", prefix+"-faulty.yaml"))
		item.FixedExperiment = filepath.ToSlash(filepath.Join("benchmarks", "v2", "experiments", prefix+"-fixed.yaml"))
		item.Experiment = item.FixedExperiment
		item.FaultyAsset = item.FaultyExperiment
		item.FixedAsset = item.FixedExperiment
		if (item.ID >= 4 && item.ID <= 17) || item.ID == 19 || item.ID == 21 || item.ID == 23 || item.ID == 24 {
			item.Mechanism.FaultyAssets = []string{fmt.Sprintf("firmware/zephyr/conf/case%02d-faulty.conf", item.ID)}
			item.Mechanism.FixedAssets = []string{fmt.Sprintf("firmware/zephyr/conf/case%02d-fixed.conf", item.ID)}
		}
	}
	return writeYAML(output, catalog)
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
		caseID, _ := strconv.Atoi(strings.Split(filepath.Base(filepath.Dir(path)), "-")[0])
		variant := "fixed"
		if strings.Contains(filepath.Base(path), "faulty") {
			variant = "faulty"
		}
		if caseID >= 4 && caseID <= 17 {
			systemID = fmt.Sprintf("renode-zephyr-case%02d-%s", caseID, variant)
		}
		experiment := ael.Experiment{APIVersion: ael.APIVersion, ID: legacy.Name, SystemID: systemID, DurationUS: legacy.DurationUS, MacroStepUS: legacy.MacroStepUS, Seed: legacy.Seed, Timeout: time.Duration(legacy.TimeoutS) * time.Second,
			RequiredFidelity: ael.Fidelity{Firmware: ael.FidelityFunctional, Register: ael.FidelitySynthetic, Protocol: ael.FidelityFunctional, Timing: ael.FidelityFunctional, Physical: ael.FidelityUnsupported, HardwareValidated: false}}
		if caseID >= 13 && caseID <= 16 {
			experiment.DurationUS = 100000
			experiment.MacroStepUS = 100000
		}
		if caseID == 16 {
			experiment.DurationUS = 500000
			experiment.MacroStepUS = 500000
		}
		if caseID == 17 {
			experiment.DurationUS = 10000000
			experiment.MacroStepUS = 10000000
		}
		for _, stimulus := range legacy.Stimuli {
			if stimulus.Target == "mcu.case_id" {
				continue
			}
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

func specializeFirmwareSystems(output string) error {
	for _, variant := range []string{"faulty", "fixed"} {
		basePath := filepath.Join(output, "renode-digital-"+variant+".yaml")
		var base ael.System
		if err := decodeV2YAML(basePath, &base); err != nil {
			return err
		}
		for caseID := 4; caseID <= 17; caseID++ {
			clone, err := cloneSystem(base)
			if err != nil {
				return err
			}
			clone.ID = fmt.Sprintf("renode-zephyr-case%02d-%s", caseID, variant)
			specializeMCUFirmware(&clone, caseID, variant)
			if err := writeYAML(filepath.Join(output, fmt.Sprintf("renode-case%02d-%s.yaml", caseID, variant)), clone); err != nil {
				return err
			}
		}
	}
	for _, item := range []struct {
		file    string
		caseID  int
		variant string
	}{
		{"power-renode-faulty.yaml", 19, "faulty"}, {"power-renode-fixed.yaml", 19, "fixed"},
		{"thermal-renode-faulty.yaml", 21, "faulty"}, {"thermal-renode-fixed.yaml", 21, "fixed"},
		{"network-renode-faulty.yaml", 23, "faulty"}, {"network-renode-fixed.yaml", 23, "fixed"},
		{"five-domain-faulty.yaml", 24, "faulty"}, {"five-domain-fixed.yaml", 24, "fixed"},
	} {
		path := filepath.Join(output, item.file)
		var system ael.System
		if err := decodeV2YAML(path, &system); err != nil {
			return err
		}
		specializeMCUFirmware(&system, item.caseID, item.variant)
		if err := writeYAML(path, system); err != nil {
			return err
		}
	}
	riscvPath := filepath.Join(output, "renode-riscv-smoke.yaml")
	var riscv ael.System
	if err := decodeV2YAML(riscvPath, &riscv); err != nil {
		return err
	}
	removeCasePort(&riscv)
	if err := writeYAML(riscvPath, riscv); err != nil {
		return err
	}
	return nil
}

func specializeMCUFirmware(system *ael.System, caseID int, variant string) {
	for index := range system.Components {
		component := &system.Components[index]
		if component.ID != "mcu" {
			continue
		}
		if component.Properties == nil {
			component.Properties = map[string]any{}
		}
		firmware := fmt.Sprintf("firmware/zephyr/build-case%02d-%s/zephyr/zephyr.elf", caseID, variant)
		if caseID == 17 {
			firmware = fmt.Sprintf("firmware/zephyr/build-case17-%s/merged.hex", variant)
		}
		component.Properties["firmware"] = firmware
	}
	removeCasePort(system)
}

func removeCasePort(system *ael.System) {
	for index := range system.Components {
		component := &system.Components[index]
		if component.ID != "mcu" {
			continue
		}
		if inputs, ok := component.Properties["input_registers"].(map[string]any); ok {
			delete(inputs, "case_id")
		}
		ports := component.Ports[:0]
		for _, port := range component.Ports {
			if port.Name != "case_id" {
				ports = append(ports, port)
			}
		}
		component.Ports = ports
	}
}

func cloneSystem(source ael.System) (ael.System, error) {
	payload, err := json.Marshal(source)
	if err != nil {
		return ael.System{}, err
	}
	var target ael.System
	err = json.Unmarshal(payload, &target)
	return target, err
}
func decodeV2YAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var normalized any
	if err := yaml.Unmarshal(data, &normalized); err != nil {
		return err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
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
