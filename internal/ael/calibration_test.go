package ael

import (
	"strings"
	"testing"
	"time"
)

func TestCalibrationAndEnvelopeFailClosed(t *testing.T) {
	now := time.Now().UTC()
	digest := strings.Repeat("a", 64)
	record := CalibrationRecord{APIVersion: APIVersion, ID: "cal", InstrumentID: "scope", InstrumentKind: "oscilloscope", SerialNumber: "serial", CertificateSHA256: digest, CalibratedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), Uncertainty: map[string]float64{"voltage": 0.01}, Units: map[string]string{"voltage": "V"}, ApprovedBy: "lab", Signature: "signature"}
	if err := ValidateCalibrationRecord(record, now); err != nil {
		t.Fatal(err)
	}
	record.ExpiresAt = now.Add(-time.Minute)
	if err := ValidateCalibrationRecord(record, now); err == nil {
		t.Fatal("expired calibration was accepted")
	}
	evidence := InstrumentEvidence{APIVersion: APIVersion, ID: "e", RunID: "run", InstrumentID: "scope", CalibrationID: "cal", Operation: "capture", RawArtifactSHA256: digest, Observations: map[string]float64{"voltage": 3.3}, Units: map[string]string{"voltage": "V"}, CapturedAt: now, Signature: "signature"}
	if err := ValidateInstrumentEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	envelope := ValidationEnvelope{ID: "env", ModelID: "model", ModelVersion: "1", HardwareRevision: "A", BoardIDs: []string{"board-1"}, Conditions: map[string]string{"temperature": "25 Cel"}, Tolerances: map[string]float64{"voltage": 0.1}, EvidenceRunIDs: []string{"run"}, CalibrationIDs: []string{"cal"}, InstrumentEvidenceIDs: []string{"e"}, ModelSHA256: map[string]string{"model": digest}, ToolDigests: map[string]string{"scope-driver": digest}, CreatedAt: now, ExpiresAt: now.Add(time.Hour), SignedBy: "reviewer", Signature: "signature"}
	if err := ValidateEnvelope(envelope, now); err != nil {
		t.Fatal(err)
	}
	envelope.CalibrationIDs = nil
	if err := ValidateEnvelope(envelope, now); err == nil {
		t.Fatal("envelope without calibration chain was accepted")
	}
}
