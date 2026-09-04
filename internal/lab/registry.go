package lab

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     string       `yaml:"version" json:"version"`
	Status      string       `yaml:"status" json:"status"`
	Instruments []Instrument `yaml:"instruments" json:"instruments"`
}

type Instrument struct {
	ID       string `yaml:"id" json:"id"`
	Kind     string `yaml:"kind" json:"kind"`
	Driver   string `yaml:"driver" json:"driver"`
	Resource string `yaml:"resource" json:"-"`
}

type OperationRequest struct {
	APIVersion    string         `json:"api_version"`
	InstrumentID  string         `json:"instrument_id"`
	CalibrationID string         `json:"calibration_id"`
	Operation     string         `json:"operation"`
	Parameters    map[string]any `json:"parameters"`
}

type OperationResult struct {
	InstrumentID  string             `json:"instrument_id"`
	CalibrationID string             `json:"calibration_id"`
	Operation     string             `json:"operation"`
	Observations  map[string]float64 `json:"observations"`
	Units         map[string]string  `json:"units"`
	ArtifactPath  string             `json:"artifact_path"`
}

type Registry struct {
	instruments map[string]Instrument
}

var operationAllowlist = map[string]map[string]bool{
	"power_supply":      set("set_voltage", "set_current_limit", "set_output", "measure_voltage", "measure_current"),
	"oscilloscope":      set("configure_channel", "capture_waveform", "measure_voltage", "measure_frequency"),
	"logic_analyzer":    set("capture_digital", "decode_protocol"),
	"power_analyzer":    set("capture_power", "measure_energy", "measure_current"),
	"thermal_chamber":   set("set_temperature", "measure_temperature"),
	"vna":               set("sweep_sparameters"),
	"spectrum_analyzer": set("sweep_spectrum", "measure_channel_power"),
	"attenuator":        set("set_attenuation"),
}

func Load(reader io.Reader) (*Registry, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if config.Version == "" || config.Status != "unverified" {
		return nil, errors.New("lab inventory must be versioned and explicitly unverified before enrollment")
	}
	registry := &Registry{instruments: map[string]Instrument{}}
	for _, instrument := range config.Instruments {
		if instrument.ID == "" || instrument.Driver == "" || instrument.Resource == "" || operationAllowlist[instrument.Kind] == nil || registry.instruments[instrument.ID].ID != "" {
			return nil, fmt.Errorf("invalid or duplicate instrument %q", instrument.ID)
		}
		registry.instruments[instrument.ID] = instrument
	}
	return registry, nil
}

func (r *Registry) Validate(request OperationRequest) (Instrument, error) {
	if request.APIVersion != "ael.lab/v1" || request.InstrumentID == "" || request.CalibrationID == "" || request.Operation == "" {
		return Instrument{}, errors.New("typed lab operation, instrument, and calibration are required")
	}
	if strings.Contains(strings.ToLower(request.Operation), "scpi") || request.Parameters["command"] != nil || request.Parameters["raw"] != nil {
		return Instrument{}, errors.New("raw SCPI and arbitrary instrument commands are prohibited")
	}
	instrument := r.instruments[request.InstrumentID]
	if instrument.ID == "" {
		return Instrument{}, errors.New("instrument is not enrolled")
	}
	if !operationAllowlist[instrument.Kind][request.Operation] {
		return Instrument{}, fmt.Errorf("operation %s is not allowed for %s", request.Operation, instrument.Kind)
	}
	return instrument, nil
}

func (r *Registry) Capabilities() map[string][]string {
	result := map[string][]string{}
	for id, instrument := range r.instruments {
		for operation := range operationAllowlist[instrument.Kind] {
			result[id] = append(result[id], operation)
		}
		sort.Strings(result[id])
	}
	return result
}

func set(values ...string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
