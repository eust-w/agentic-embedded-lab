package modeling

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

const APIVersion = "ael.dev/v2"

type ModelState string

const (
	StateDraft                ModelState = "draft"
	StateGenerated            ModelState = "generated"
	StateStaticValidated      ModelState = "static_validated"
	StateConformanceValidated ModelState = "conformance_validated"
	StateHardwareValidated    ModelState = "hardware_validated"
	StateProductionApproved   ModelState = "production_approved"
	StateDeprecated           ModelState = "deprecated"
)

type UnitValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type Field struct {
	Name       string `json:"name"`
	LSB        int    `json:"lsb"`
	Width      int    `json:"width"`
	Access     string `json:"access"`
	Reset      uint64 `json:"reset"`
	SideEffect string `json:"side_effect,omitempty"`
}

type Register struct {
	Name   string  `json:"name"`
	Offset uint64  `json:"offset"`
	Width  int     `json:"width"`
	Reset  uint64  `json:"reset"`
	Fields []Field `json:"fields"`
}

type Clock struct {
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Frequency   UnitValue `json:"frequency"`
	EnabledWhen string    `json:"enabled_when,omitempty"`
	ResetDomain string    `json:"reset_domain,omitempty"`
}

type Timer struct {
	Name            string `json:"name"`
	Clock           string `json:"clock"`
	Width           int    `json:"width"`
	Direction       string `json:"direction"`
	WrapEvent       string `json:"wrap_event,omitempty"`
	CompareChannels int    `json:"compare_channels"`
}

type Interrupt struct {
	Name           string `json:"name"`
	Line           int    `json:"line"`
	Trigger        string `json:"trigger"`
	Condition      string `json:"condition"`
	ClearCondition string `json:"clear_condition,omitempty"`
}

type DMARequest struct {
	Name        string `json:"name"`
	RequestLine int    `json:"request_line"`
	Condition   string `json:"condition"`
	Direction   string `json:"direction"`
	WidthBits   int    `json:"width_bits"`
}

type Transaction struct {
	Name     string     `json:"name"`
	Protocol string     `json:"protocol"`
	Role     string     `json:"role"`
	Latency  *UnitValue `json:"latency,omitempty"`
}

type FaultBehavior struct {
	Name      string         `json:"name"`
	Trigger   string         `json:"trigger"`
	Effects   map[string]any `json:"effects"`
	RecoverBy string         `json:"recover_by,omitempty"`
}

type PowerState struct {
	Name        string     `json:"name"`
	Current     *UnitValue `json:"current,omitempty"`
	Entry       string     `json:"entry_condition,omitempty"`
	Exit        string     `json:"exit_condition,omitempty"`
	WakeLatency *UnitValue `json:"wake_latency,omitempty"`
}

type IR struct {
	APIVersion    string               `json:"api_version"`
	Kind          string               `json:"kind"`
	Name          string               `json:"name"`
	BusWidth      int                  `json:"bus_width"`
	Size          uint64               `json:"size"`
	Registers     []Register           `json:"registers"`
	StateMachines []map[string]any     `json:"state_machines"`
	Clocks        []Clock              `json:"clocks"`
	Timers        []Timer              `json:"timers"`
	Interrupts    []Interrupt          `json:"interrupts"`
	DMARequests   []DMARequest         `json:"dma_requests"`
	Transactions  []Transaction        `json:"transactions"`
	Faults        []FaultBehavior      `json:"faults"`
	Timing        map[string]UnitValue `json:"timing"`
	PowerStates   []PowerState         `json:"power_states"`
	FMIPorts      []ael.Port           `json:"fmi_ports"`
	Grounding     map[string][]string  `json:"grounding"`
}

type Package struct {
	APIVersion            string            `json:"api_version"`
	Kind                  string            `json:"kind"`
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Version               string            `json:"version"`
	Backend               ael.Backend       `json:"backend"`
	State                 ModelState        `json:"state"`
	SourcePaths           []string          `json:"source_paths"`
	SourceHashes          map[string]string `json:"source_hashes"`
	IRPath                string            `json:"ir_path,omitempty"`
	ArtifactPaths         []string          `json:"artifact_paths"`
	ValidationEvidence    []string          `json:"validation_evidence"`
	Signature             string            `json:"signature,omitempty"`
	GeneratedBy           string            `json:"generated_by,omitempty"`
	GroundingManifestPath string            `json:"grounding_manifest_path,omitempty"`
	GenerationReceiptPath string            `json:"generation_receipt_path,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
}

func (ir IR) Validate() error {
	if ir.APIVersion != APIVersion || ir.Kind != "HardwareBehaviorIR" || ir.Name == "" || ir.Size == 0 {
		return errors.New("invalid Hardware Behavior IR header")
	}
	registers := append([]Register(nil), ir.Registers...)
	sort.Slice(registers, func(i, j int) bool { return registers[i].Offset < registers[j].Offset })
	names := make(map[string]bool)
	var previousEnd uint64
	for index, register := range registers {
		if names[register.Name] || register.Name == "" {
			return fmt.Errorf("register names must be unique and non-empty: %s", register.Name)
		}
		names[register.Name] = true
		if register.Width != 8 && register.Width != 16 && register.Width != 32 && register.Width != 64 {
			return fmt.Errorf("register %s has unsupported width", register.Name)
		}
		end := register.Offset + uint64(register.Width/8)
		if end > ir.Size || (index > 0 && register.Offset < previousEnd) {
			return fmt.Errorf("register %s overlaps or exceeds the peripheral size", register.Name)
		}
		previousEnd = end
		bits := make(map[int]bool)
		fieldNames := make(map[string]bool)
		for _, field := range register.Fields {
			if field.Name == "" || fieldNames[field.Name] || field.LSB < 0 || field.Width <= 0 || field.LSB+field.Width > register.Width {
				return fmt.Errorf("register %s contains an invalid field %s", register.Name, field.Name)
			}
			fieldNames[field.Name] = true
			for bit := field.LSB; bit < field.LSB+field.Width; bit++ {
				if bits[bit] {
					return fmt.Errorf("register %s has overlapping fields", register.Name)
				}
				bits[bit] = true
			}
			if field.Width < 64 && field.Reset >= uint64(1)<<field.Width {
				return fmt.Errorf("field %s.%s reset does not fit", register.Name, field.Name)
			}
			switch field.Access {
			case "ro", "wo", "rw", "w1c", "w1s":
			default:
				return fmt.Errorf("field %s.%s has unsupported access", register.Name, field.Name)
			}
		}
	}
	return nil
}

func CanTransition(from, to ModelState, actor string) bool {
	allowed := map[ModelState][]ModelState{
		StateDraft: {StateGenerated, StateDeprecated}, StateGenerated: {StateStaticValidated, StateDeprecated}, StateStaticValidated: {StateConformanceValidated, StateDeprecated}, StateConformanceValidated: {StateHardwareValidated, StateDeprecated}, StateHardwareValidated: {StateProductionApproved, StateDeprecated}, StateProductionApproved: {StateDeprecated},
	}
	if actor == "agent" && (to == StateHardwareValidated || to == StateProductionApproved) {
		return false
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}
