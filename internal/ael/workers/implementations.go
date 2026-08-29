package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

func ImplementationFor(backend ael.Backend) (Implementation, error) {
	switch backend {
	case ael.BackendRenode:
		return Renode{}, nil
	case ael.BackendNgspice:
		return &Ngspice{}, nil
	case ael.BackendModelica:
		return Modelica{}, nil
	case ael.BackendOMSimulator:
		return OMSimulator{}, nil
	case ael.BackendNS3:
		return NS3{}, nil
	case ael.BackendOpenEMS:
		return OpenEMS{}, nil
	case ael.BackendZephyr:
		return Zephyr{}, nil
	default:
		return nil, fmt.Errorf("unsupported backend %s", backend)
	}
}

type Renode struct{}

func (Renode) Backend() ael.Backend                  { return ael.BackendRenode }
func (Renode) ExpectedVersion() string               { return "1.16.1" }
func (Renode) Commands() []string                    { return []string{"renode"} }
func (Renode) VersionArguments() []string            { return []string{"-v", "--version"} }
func (Renode) Prepare(context.Context, *State) error { return nil }

var registerPattern = regexp.MustCompile(`AEL_REGISTER:([A-Za-z0-9_.-]+):(?:0x)?([0-9A-Fa-f]+)`)

func (Renode) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	model, err := WorkspacePath(state, state.Component.Model, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	lines, err := renodeInitialisation(state, model)
	if err != nil {
		return ael.StepResult{}, err
	}
	lines = append(lines, fmt.Sprintf("emulation RunFor \"%.9f\"", float64(state.VirtualTimeUS+stepUS)/1_000_000))
	outputs := propertyMap(state.Component.Properties, "output_registers")
	keys := sortedKeys(outputs)
	for _, name := range keys {
		address, ok := integer(outputs[name])
		if !ok || address < 0 {
			return ael.StepResult{}, errors.New("Renode output register addresses must be non-negative integers")
		}
		lines = append(lines, fmt.Sprintf("python \"print('AEL_REGISTER:%s:%%x' %% self.Machine.SystemBus.ReadDoubleWord(%d))\"", name, address))
	}
	lines = append(lines, "quit")
	script := filepath.Join(state.RuntimeDir, "ael-step.resc")
	if err := os.WriteFile(script, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return ael.StepResult{}, err
	}
	output, err := RunTool(ctx, state, []string{"--disable-gui", "--console", script}, durationProperty(state.Component.Properties, "timeout_s", 300*time.Second), nil)
	if err != nil {
		return ael.StepResult{}, err
	}
	logPath := filepath.Join(state.RuntimeDir, fmt.Sprintf("step-%d.log", state.VirtualTimeUS+stepUS))
	_ = os.WriteFile(logPath, output, 0o600)
	metrics, events := ParseOutput(state, output, state.VirtualTimeUS+stepUS)
	resultOutputs := make(map[string]float64)
	scales := propertyMap(state.Component.Properties, "output_scales")
	sentinels := propertyMap(state.Component.Properties, "output_sentinels")
	for _, match := range registerPattern.FindAllStringSubmatch(string(output), -1) {
		value, _ := strconv.ParseUint(match[2], 16, 64)
		if sentinel, ok := integer(sentinels[match[1]]); ok && value == uint64(sentinel) {
			return ael.StepResult{}, fmt.Errorf("Renode output register %s remained at sentinel %#x", match[1], value)
		}
		scale := numberOr(scales[match[1]], 1)
		resultOutputs[match[1]] = float64(value) * scale
	}
	return ael.StepResult{Outputs: resultOutputs, Metrics: metrics, Events: events, Artifacts: map[string]string{"script": relativeArtifact(state.Workspace, script), "log": relativeArtifact(state.Workspace, logPath)}}, nil
}

func renodeInitialisation(state *State, model string) ([]string, error) {
	var lines []string
	if filepath.Ext(model) == ".repl" {
		lines = []string{"using sysbus", fmt.Sprintf("mach create \"ael-%s\"", state.Component.ID), "machine LoadPlatformDescription @" + model}
	} else {
		lines = []string{"include @" + model}
	}
	if firmware := stringProperty(state.Component.Properties, "firmware"); firmware != "" {
		path, err := WorkspacePath(state, firmware, true)
		if err != nil {
			return nil, err
		}
		lines = append(lines, "sysbus LoadELF @"+path)
	}
	inputRegisters := propertyMap(state.Component.Properties, "input_registers")
	for _, name := range sortedInputKeys(state.Inputs) {
		address, exists := inputRegisters[name]
		if !exists {
			continue
		}
		addressValue, ok := integer(address)
		value, valueOK := integer(state.Inputs[name])
		if !ok || !valueOK || addressValue < 0 {
			return nil, errors.New("Renode register bridge values must be non-negative integers")
		}
		lines = append(lines, fmt.Sprintf("sysbus WriteDoubleWord %#x %#x", addressValue, uint64(value)&0xffffffff))
	}
	return lines, nil
}

type Ngspice struct{ brownout bool }

func (*Ngspice) Backend() ael.Backend                    { return ael.BackendNgspice }
func (*Ngspice) ExpectedVersion() string                 { return "46" }
func (*Ngspice) Commands() []string                      { return []string{"ngspice"} }
func (*Ngspice) VersionArguments() []string              { return []string{"--version", "-v"} }
func (n *Ngspice) Prepare(context.Context, *State) error { n.brownout = false; return nil }

func (n *Ngspice) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	model, err := WorkspacePath(state, state.Component.Model, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	content, err := os.ReadFile(model)
	if err != nil {
		return ael.StepResult{}, err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return ael.StepResult{}, errors.New("ngspice model is empty")
	}
	values := map[string]any{"source_resistance_ohm": 0.15, "capacitance_uF": 47.0, "load_microamp": 60000.0, "rf_retries": 0.0}
	for key, value := range propertyMap(state.Component.Properties, "parameters") {
		values[key] = value
	}
	for key, value := range state.Inputs {
		values[key] = value
	}
	parameters := make([]string, 0, len(values))
	for _, key := range sortedKeys(values) {
		parameters = append(parameters, fmt.Sprintf(".param AEL_%s=%v", key, values[key]))
	}
	deck := filepath.Join(state.RuntimeDir, "ael.cir")
	deckText := strings.Join(append(append([]string{lines[0]}, parameters...), lines[1:]...), "\n")
	if err := os.WriteFile(deck, []byte(deckText), 0o600); err != nil {
		return ael.StepResult{}, err
	}
	logPath := filepath.Join(state.RuntimeDir, fmt.Sprintf("step-%d.log", state.VirtualTimeUS+stepUS))
	output, err := RunTool(ctx, state, []string{"-b", "-o", logPath, deck}, durationProperty(state.Component.Properties, "timeout_s", 120*time.Second), nil)
	if err != nil {
		return ael.StepResult{}, err
	}
	logData, _ := os.ReadFile(logPath)
	combined := append(output, logData...)
	metrics, events := ParseOutput(state, combined, state.VirtualTimeUS+stepUS)
	if voltage, ok := metrics["supply_voltage"]; ok {
		threshold := numberOr(state.Component.Properties["bor_threshold_V"], 2.7)
		crossed := !n.brownout && voltage < threshold
		n.brownout = n.brownout || crossed
		metrics["failure"] = boolMetric(n.brownout)
		if crossed {
			events = append(events, ael.Event{VirtualTimeUS: state.VirtualTimeUS + stepUS, Source: state.Component.ID, Type: "power.brownout_threshold_crossed", Payload: map[string]any{"rail_voltage_V": voltage, "bor_threshold_V": threshold}, FidelityRef: "ngspice:tool-executed"})
		}
	}
	return ael.StepResult{Outputs: copyMetrics(metrics), Metrics: metrics, Events: events, Artifacts: map[string]string{"log": relativeArtifact(state.Workspace, logPath), "deck": relativeArtifact(state.Workspace, deck)}}, nil
}

type Modelica struct{}

func (Modelica) Backend() ael.Backend                  { return ael.BackendModelica }
func (Modelica) ExpectedVersion() string               { return "1.27.0" }
func (Modelica) Commands() []string                    { return []string{"omc"} }
func (Modelica) VersionArguments() []string            { return []string{"--version"} }
func (Modelica) Prepare(context.Context, *State) error { return nil }
func (Modelica) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	modelPath, err := WorkspacePath(state, state.Component.Model, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return ael.StepResult{}, err
	}
	text := string(data)
	values := propertyMap(state.Component.Properties, "parameters")
	for key, value := range state.Inputs {
		values[key] = value
	}
	for key, value := range values {
		text = strings.ReplaceAll(text, "{{"+key+"}}", fmt.Sprint(value))
	}
	endPattern := regexp.MustCompile(`end\s+([A-Za-z0-9_]+)\s*;`)
	match := endPattern.FindStringSubmatchIndex(text)
	var script string
	if match != nil {
		className := text[match[2]:match[3]]
		model := filepath.Join(state.RuntimeDir, className+".mo")
		script = filepath.Join(state.RuntimeDir, strings.ToLower(className)+".mos")
		if err := os.WriteFile(model, []byte(strings.TrimSpace(text[:match[1]])+"\n"), 0o600); err != nil {
			return ael.StepResult{}, err
		}
		commands := strings.TrimSpace(text[match[1]:])
		if err := os.WriteFile(script, []byte(fmt.Sprintf("loadFile(\"%s.mo\");\n%s\n", className, commands)), 0o600); err != nil {
			return ael.StepResult{}, err
		}
	} else if filepath.Ext(modelPath) == ".mos" {
		script = filepath.Join(state.RuntimeDir, filepath.Base(modelPath))
		if err := os.WriteFile(script, []byte(text), 0o600); err != nil {
			return ael.StepResult{}, err
		}
	} else {
		return ael.StepResult{}, errors.New("OpenModelica model must contain end <ModelName>; or be a .mos script")
	}
	return scriptWorkerStep(ctx, state, stepUS, []string{script}, "modelica")
}

type OMSimulator struct{}

func (OMSimulator) Backend() ael.Backend                  { return ael.BackendOMSimulator }
func (OMSimulator) ExpectedVersion() string               { return "2.1.3" }
func (OMSimulator) Commands() []string                    { return []string{"OMSimulator", "omsimulator"} }
func (OMSimulator) VersionArguments() []string            { return []string{"--version", "-v"} }
func (OMSimulator) Prepare(context.Context, *State) error { return nil }
func (OMSimulator) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	model, err := WorkspacePath(state, state.Component.Model, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	return scriptWorkerStep(ctx, state, stepUS, []string{model, fmt.Sprintf("--startTime=%f", float64(state.VirtualTimeUS)/1e6), fmt.Sprintf("--stopTime=%f", float64(state.VirtualTimeUS+stepUS)/1e6)}, "omsimulator")
}

type NS3 struct{}

func (NS3) Backend() ael.Backend                  { return ael.BackendNS3 }
func (NS3) ExpectedVersion() string               { return "3.47" }
func (NS3) Commands() []string                    { return []string{"ns3"} }
func (NS3) VersionArguments() []string            { return []string{"--version"} }
func (NS3) Prepare(context.Context, *State) error { return nil }
func (NS3) DetectVersion(ctx context.Context, tool, workspace string) string {
	if binary := os.Getenv("AEL_NS3_PRECOMPILED"); binary != "" {
		if _, err := os.Stat(binary); err == nil {
			return "3.47"
		}
	}
	output, _ := exec.CommandContext(ctx, tool, "show", "version").CombinedOutput()
	if strings.Contains(string(output), "3.47") {
		return "3.47"
	}
	return ""
}
func (NS3) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	args := []string{}
	for _, key := range sortedInputKeys(state.Inputs) {
		args = append(args, fmt.Sprintf("--%s=%v", key, state.Inputs[key]))
	}
	args = append(args, fmt.Sprintf("--seed=%d", state.Seed), fmt.Sprintf("--stopUs=%d", state.VirtualTimeUS+stepUS))
	if binary := os.Getenv("AEL_NS3_PRECOMPILED"); binary != "" {
		original := state.Tool
		state.Tool = binary
		defer func() { state.Tool = original }()
		return scriptWorkerStep(ctx, state, stepUS, args, "ns3")
	}
	program := stringProperty(state.Component.Properties, "program")
	if program == "" {
		program = "scratch/ael-network"
	}
	return scriptWorkerStep(ctx, state, stepUS, []string{"run", program + " " + strings.Join(args, " ")}, "ns3")
}

type OpenEMS struct{}

func (OpenEMS) Backend() ael.Backend                  { return ael.BackendOpenEMS }
func (OpenEMS) ExpectedVersion() string               { return "0.0.36" }
func (OpenEMS) Commands() []string                    { return []string{"openEMS"} }
func (OpenEMS) VersionArguments() []string            { return []string{"--version", "-v"} }
func (OpenEMS) Prepare(context.Context, *State) error { return nil }
func (OpenEMS) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	model, err := WorkspacePath(state, state.Component.Model, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	modelData, err := os.ReadFile(model)
	if err != nil {
		return ael.StepResult{}, err
	}
	digest := sha256.Sum256(append(modelData, mustJSON(state.Inputs)...))
	cache := filepath.Join(state.Workspace, ".ael", "openems-cache", hex.EncodeToString(digest[:])+".json")
	if data, err := os.ReadFile(cache); err == nil {
		metrics := map[string]float64{}
		_ = json.Unmarshal(data, &metrics)
		return ael.StepResult{Outputs: copyMetrics(metrics), Metrics: metrics, Events: []ael.Event{{Type: "openems.cache_hit", FidelityRef: "openems:tool-executed-cached"}}, Artifacts: map[string]string{"cache": relativeArtifact(state.Workspace, cache)}}, nil
	}
	octave, err := exec.LookPath("octave-cli")
	if err != nil {
		return ael.StepResult{}, err
	}
	original := state.Tool
	state.Tool = octave
	defer func() { state.Tool = original }()
	result, err := scriptWorkerStep(ctx, state, stepUS, []string{"--no-gui", "--quiet", model}, "openems")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(cache), 0o700)
		_ = os.WriteFile(cache, mustJSON(result.Metrics), 0o600)
	}
	return result, err
}

type Zephyr struct{}

func (Zephyr) Backend() ael.Backend       { return ael.BackendZephyr }
func (Zephyr) ExpectedVersion() string    { return "4.4.2" }
func (Zephyr) Commands() []string         { return []string{"west"} }
func (Zephyr) VersionArguments() []string { return []string{"--version"} }
func (Zephyr) DetectVersion(ctx context.Context, tool, workspace string) string {
	base := os.Getenv("ZEPHYR_BASE")
	if base == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(base, "VERSION"))
	if err != nil {
		return ""
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values["VERSION_MAJOR"] + "." + values["VERSION_MINOR"] + "." + values["PATCHLEVEL"]
}
func (Zephyr) Prepare(context.Context, *State) error { return nil }
func (Zephyr) Step(ctx context.Context, state *State, stepUS int64) (ael.StepResult, error) {
	source := stringProperty(state.Component.Properties, "source")
	sourcePath, err := WorkspacePath(state, source, true)
	if err != nil {
		return ael.StepResult{}, err
	}
	board := stringProperty(state.Component.Properties, "board")
	if board == "" {
		board = "stm32f4_disco"
	}
	build := filepath.Join(state.RuntimeDir, "build")
	extra := []string{"-DUSER_CACHE_DIR=" + filepath.Join(state.RuntimeDir, "zephyr-cache")}
	caseID, caseOK := integer(state.Component.Properties["case_id"])
	variant := stringProperty(state.Component.Properties, "variant")
	if caseOK && caseID >= 1 && caseID <= 3 && (variant == "faulty" || variant == "fixed") {
		config, err := WorkspacePath(state, fmt.Sprintf("firmware/zephyr-build/conf/case%d-%s.conf", caseID, variant), true)
		if err != nil {
			return ael.StepResult{}, err
		}
		overlayName := "reference.overlay"
		if caseID == 2 {
			overlayName = fmt.Sprintf("case2-%s.overlay", variant)
		}
		overlay, err := WorkspacePath(state, filepath.Join("firmware/zephyr-build/overlays", overlayName), true)
		if err != nil {
			return ael.StepResult{}, err
		}
		extra = append(extra, "-DEXTRA_CONF_FILE="+config, "-DDTC_OVERLAY_FILE="+overlay)
	}
	args := []string{"build", "-p", "always", "-b", board, sourcePath, "-d", build, "--"}
	args = append(args, extra...)
	if caseOK && caseID >= 1 && caseID <= 3 {
		output, exitCode, err := RunToolObserved(ctx, state, args, durationProperty(state.Component.Properties, "timeout_s", 300*time.Second), nil)
		if err != nil {
			return ael.StepResult{}, err
		}
		logPath := filepath.Join(state.RuntimeDir, "zephyr-build.log")
		_ = os.WriteFile(logPath, output, 0o600)
		configData, _ := os.ReadFile(filepath.Join(build, "zephyr", ".config"))
		dtsData, _ := os.ReadFile(filepath.Join(build, "zephyr", "zephyr.dts"))
		mechanismFailed := false
		detail := ""
		switch caseID {
		case 1:
			mechanismFailed = !strings.Contains(string(configData), "CONFIG_AEL_FEATURE=y")
			detail = "resolved Kconfig feature dependency"
		case 2:
			pattern := regexp.MustCompile(`ael-probe-address\s*=\s*<\s*(0x[0-9a-fA-F]+)`)
			match := pattern.FindStringSubmatch(string(dtsData))
			resolved := uint64(0)
			if match != nil {
				resolved, _ = strconv.ParseUint(match[1], 0, 64)
			}
			mechanismFailed = resolved != 0x40011000
			detail = fmt.Sprintf("resolved devicetree address=%#x", resolved)
		case 3:
			mechanismFailed = exitCode != 0
			detail = fmt.Sprintf("link returncode=%d", exitCode)
		}
		value := 0.0
		if mechanismFailed {
			value = 1
		}
		artifacts := map[string]string{"build_log": relativeArtifact(state.Workspace, logPath)}
		for label, path := range map[string]string{"dotconfig": filepath.Join(build, "zephyr", ".config"), "devicetree": filepath.Join(build, "zephyr", "zephyr.dts"), "firmware_elf": filepath.Join(build, "zephyr", "zephyr.elf")} {
			if _, err := os.Stat(path); err == nil {
				artifacts[label] = relativeArtifact(state.Workspace, path)
			}
		}
		event := ael.Event{VirtualTimeUS: state.VirtualTimeUS + stepUS, Source: state.Component.ID, Type: fmt.Sprintf("zephyr.build.case%d", caseID), Payload: map[string]any{"variant": variant, "returncode": exitCode, "mechanism_failed": mechanismFailed, "detail": detail}, FidelityRef: "zephyr_build:tool-executed"}
		return ael.StepResult{Outputs: map[string]float64{"failure": value}, Metrics: map[string]float64{"failure": value}, Events: []ael.Event{event}, Artifacts: artifacts}, nil
	}
	result, err := scriptWorkerStep(ctx, state, stepUS, args, "zephyr_build")
	if err != nil {
		return result, err
	}
	if _, err := os.Stat(filepath.Join(build, "zephyr", "zephyr.elf")); err == nil {
		result.Artifacts["firmware_elf"] = relativeArtifact(state.Workspace, filepath.Join(build, "zephyr", "zephyr.elf"))
	}
	return result, nil
}

func scriptWorkerStep(ctx context.Context, state *State, stepUS int64, args []string, label string) (ael.StepResult, error) {
	output, err := RunTool(ctx, state, args, durationProperty(state.Component.Properties, "timeout_s", 120*time.Second), nil)
	if err != nil {
		return ael.StepResult{}, err
	}
	logPath := filepath.Join(state.RuntimeDir, fmt.Sprintf("step-%d.log", state.VirtualTimeUS+stepUS))
	_ = os.WriteFile(logPath, output, 0o600)
	metrics, events := ParseOutput(state, output, state.VirtualTimeUS+stepUS)
	events = append(events, ael.Event{VirtualTimeUS: state.VirtualTimeUS + stepUS, Source: state.Component.ID, Type: label + ".step_completed", Payload: map[string]any{}, FidelityRef: label + ":tool-executed"})
	return ael.StepResult{Outputs: copyMetrics(metrics), Metrics: metrics, Events: events, Artifacts: map[string]string{"log": relativeArtifact(state.Workspace, logPath)}}, nil
}

func propertyMap(properties map[string]any, key string) map[string]any {
	value, ok := properties[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}
func stringProperty(properties map[string]any, key string) string {
	value, _ := properties[key].(string)
	return value
}
func durationProperty(properties map[string]any, key string, fallback time.Duration) time.Duration {
	value, ok := numeric(properties[key])
	if !ok {
		return fallback
	}
	return time.Duration(value) * time.Second
}
func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func sortedInputKeys(values map[string]any) []string { return sortedKeys(values) }
func integer(value any) (int64, bool) {
	number, ok := numeric(value)
	return int64(number), ok && number == float64(int64(number))
}
func numberOr(value any, fallback float64) float64 {
	number, ok := numeric(value)
	if !ok {
		return fallback
	}
	return number
}
func copyMetrics(source map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func mustJSON(value any) []byte { payload, _ := json.Marshal(value); return payload }
