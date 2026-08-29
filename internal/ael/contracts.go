package ael

import "time"

const APIVersion = "ael.dev/v2"

type Backend string

const (
	BackendZephyr      Backend = "zephyr_build"
	BackendRenode      Backend = "renode"
	BackendNgspice     Backend = "ngspice"
	BackendModelica    Backend = "openmodelica"
	BackendOMSimulator Backend = "omsimulator"
	BackendNS3         Backend = "ns3"
	BackendOpenEMS     Backend = "openems"
	BackendHardware    Backend = "hardware"
)

type FidelityLevel string

const (
	FidelityUnsupported FidelityLevel = "unsupported"
	FidelitySynthetic   FidelityLevel = "synthetic"
	FidelityFunctional  FidelityLevel = "functional"
	FidelityRegister    FidelityLevel = "register_accurate"
	FidelityTiming      FidelityLevel = "timing_accurate"
	FidelityPhysical    FidelityLevel = "physical"
)

type Fidelity struct {
	Firmware          FidelityLevel `json:"firmware"`
	Register          FidelityLevel `json:"register"`
	Protocol          FidelityLevel `json:"protocol"`
	Timing            FidelityLevel `json:"timing"`
	Physical          FidelityLevel `json:"physical"`
	HardwareValidated bool          `json:"hardware_validated"`
	Limitations       []string      `json:"limitations"`
}

type Problem struct {
	APIVersion    string         `json:"api_version"`
	ID            string         `json:"id"`
	Category      string         `json:"category"`
	Phenomenon    string         `json:"phenomenon"`
	Goal          string         `json:"goal"`
	Constraints   map[string]any `json:"constraints"`
	ExpectedClaim string         `json:"expected_claim"`
}

type Port struct {
	Name      string `json:"name"`
	Direction string `json:"direction"`
	Type      string `json:"type"`
	Unit      string `json:"unit,omitempty"`
}

type Component struct {
	ID          string         `json:"id"`
	Backend     Backend        `json:"backend"`
	Model       string         `json:"model"`
	StepUS      int64          `json:"step_us"`
	Rollback    bool           `json:"rollback"`
	EventDriven bool           `json:"event_driven"`
	Ports       []Port         `json:"ports"`
	Properties  map[string]any `json:"properties"`
	Fidelity    Fidelity       `json:"fidelity"`
}

type Connection struct {
	SourceComponent string `json:"source_component"`
	SourcePort      string `json:"source_port"`
	TargetComponent string `json:"target_component"`
	TargetPort      string `json:"target_port"`
	Unit            string `json:"unit,omitempty"`
}

type System struct {
	APIVersion  string       `json:"api_version"`
	ID          string       `json:"id"`
	Components  []Component  `json:"components"`
	Connections []Connection `json:"connections"`
}

type Stimulus struct {
	AtUS   int64   `json:"at_us"`
	Target string  `json:"target"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit,omitempty"`
}

type Fault struct {
	AtUS       int64          `json:"at_us"`
	Target     string         `json:"target"`
	Kind       string         `json:"kind"`
	Parameters map[string]any `json:"parameters"`
}

type Assertion struct {
	ID       string  `json:"id"`
	Metric   string  `json:"metric"`
	Operator string  `json:"operator"`
	Expected float64 `json:"expected"`
	Unit     string  `json:"unit,omitempty"`
}

type Experiment struct {
	APIVersion       string        `json:"api_version"`
	ID               string        `json:"id"`
	SystemID         string        `json:"system_id"`
	DurationUS       int64         `json:"duration_us"`
	MacroStepUS      int64         `json:"macro_step_us"`
	Seed             int64         `json:"seed"`
	Timeout          time.Duration `json:"timeout"`
	Stimuli          []Stimulus    `json:"stimuli"`
	Faults           []Fault       `json:"faults"`
	Assertions       []Assertion   `json:"assertions"`
	RequiredFidelity Fidelity      `json:"required_fidelity"`
}

type Event struct {
	APIVersion    string         `json:"api_version"`
	Sequence      int64          `json:"sequence"`
	VirtualTimeUS int64          `json:"virtual_time_us"`
	Source        string         `json:"source"`
	Type          string         `json:"type"`
	CausalParents []int64        `json:"causal_parents,omitempty"`
	Payload       map[string]any `json:"payload"`
	FidelityRef   string         `json:"fidelity_ref"`
}

type AssertionResult struct {
	ID       string  `json:"id"`
	Passed   bool    `json:"passed"`
	Observed float64 `json:"observed"`
	Expected float64 `json:"expected"`
	Message  string  `json:"message"`
}

type EvidenceBundle struct {
	APIVersion     string            `json:"api_version"`
	RunID          string            `json:"run_id"`
	Experiment     Experiment        `json:"experiment"`
	System         System            `json:"system"`
	Events         []Event           `json:"events"`
	Assertions     []AssertionResult `json:"assertions"`
	Artifacts      map[string]string `json:"artifacts"`
	SourceRevision string            `json:"source_revision"`
	TraceSHA256    string            `json:"trace_sha256"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at"`
	Fidelity       Fidelity          `json:"fidelity"`
}

type ValidationEnvelope struct {
	ID               string             `json:"id"`
	ModelID          string             `json:"model_id"`
	HardwareRevision string             `json:"hardware_revision"`
	Conditions       map[string]string  `json:"conditions"`
	Tolerances       map[string]float64 `json:"tolerances"`
	EvidenceRunIDs   []string           `json:"evidence_run_ids"`
	SignedBy         string             `json:"signed_by"`
	Signature        string             `json:"signature"`
}

type Claim struct {
	ID                string              `json:"id"`
	Statement         string              `json:"statement"`
	Status            string              `json:"status"`
	EvidenceRunIDs    []string            `json:"evidence_run_ids"`
	Envelope          *ValidationEnvelope `json:"envelope,omitempty"`
	Limitations       []string            `json:"limitations"`
	HardwareValidated bool                `json:"hardware_validated"`
}
