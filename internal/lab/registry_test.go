package lab

import (
	"strings"
	"testing"
)

func TestLabRegistryExposesOnlyTypedCalibratedOperations(t *testing.T) {
	registry, err := Load(strings.NewReader(`version: "1"
status: unverified
instruments:
  - {id: scope, kind: oscilloscope, driver: scpi-generic, resource: TCPIP::example::INSTR}
`))
	if err != nil {
		t.Fatal(err)
	}
	valid := OperationRequest{APIVersion: "ael.lab/v1", InstrumentID: "scope", CalibrationID: "cal-1", Operation: "capture_waveform", Parameters: map[string]any{"channel": 1}}
	if _, err := registry.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, request := range []OperationRequest{
		{APIVersion: "ael.lab/v1", InstrumentID: "scope", Operation: "capture_waveform"},
		{APIVersion: "ael.lab/v1", InstrumentID: "scope", CalibrationID: "cal-1", Operation: "raw_scpi", Parameters: map[string]any{"command": "*IDN?"}},
		{APIVersion: "ael.lab/v1", InstrumentID: "scope", CalibrationID: "cal-1", Operation: "set_voltage"},
	} {
		if _, err := registry.Validate(request); err == nil {
			t.Fatalf("unsafe request was accepted: %#v", request)
		}
	}
}
