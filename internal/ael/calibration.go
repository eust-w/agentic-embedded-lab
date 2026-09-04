package ael

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidateCalibrationRecord(record CalibrationRecord, at time.Time) error {
	if record.APIVersion != APIVersion || record.ID == "" || record.InstrumentID == "" || record.InstrumentKind == "" || record.SerialNumber == "" {
		return errors.New("calibration identity and instrument metadata are required")
	}
	if !sha256Pattern.MatchString(record.CertificateSHA256) {
		return errors.New("calibration certificate must have a lowercase SHA-256 digest")
	}
	if record.CalibratedAt.IsZero() || record.ExpiresAt.IsZero() || !record.ExpiresAt.After(record.CalibratedAt) || !record.ExpiresAt.After(at) {
		return errors.New("calibration validity interval is missing or expired")
	}
	if len(record.Uncertainty) == 0 || len(record.Units) == 0 || record.ApprovedBy == "" || record.Signature == "" {
		return errors.New("calibration uncertainty, units, approval, and signature are required")
	}
	for name, uncertainty := range record.Uncertainty {
		if name == "" || uncertainty < 0 || math.IsNaN(uncertainty) || math.IsInf(uncertainty, 0) || record.Units[name] == "" {
			return fmt.Errorf("invalid calibration uncertainty %q", name)
		}
	}
	return nil
}

func ValidateInstrumentEvidence(evidence InstrumentEvidence) error {
	if evidence.APIVersion != APIVersion || evidence.ID == "" || evidence.RunID == "" || evidence.InstrumentID == "" || evidence.CalibrationID == "" || evidence.Operation == "" {
		return errors.New("instrument evidence identity, run, calibration, and operation are required")
	}
	if !sha256Pattern.MatchString(evidence.RawArtifactSHA256) {
		return errors.New("instrument artifact must have a lowercase SHA-256 digest")
	}
	if evidence.CapturedAt.IsZero() || evidence.Signature == "" || len(evidence.Observations) == 0 || len(evidence.Units) == 0 {
		return errors.New("instrument timestamp, observations, units, and signature are required")
	}
	for name, value := range evidence.Observations {
		if name == "" || math.IsNaN(value) || math.IsInf(value, 0) || evidence.Units[name] == "" {
			return fmt.Errorf("invalid instrument observation %q", name)
		}
	}
	return nil
}

func ValidateEnvelope(envelope ValidationEnvelope, at time.Time) error {
	if envelope.ID == "" || envelope.ModelID == "" || envelope.ModelVersion == "" || envelope.HardwareRevision == "" || len(envelope.BoardIDs) == 0 {
		return errors.New("validation envelope identity, model version, hardware revision, and boards are required")
	}
	if len(envelope.Conditions) == 0 || len(envelope.Tolerances) == 0 || len(envelope.EvidenceRunIDs) == 0 || len(envelope.CalibrationIDs) == 0 || len(envelope.InstrumentEvidenceIDs) == 0 {
		return errors.New("validation envelope conditions, tolerances, runs, calibrations, and instrument evidence are required")
	}
	if len(envelope.ModelSHA256) == 0 || len(envelope.ToolDigests) == 0 || envelope.CreatedAt.IsZero() || envelope.ExpiresAt.IsZero() || !envelope.ExpiresAt.After(envelope.CreatedAt) || !envelope.ExpiresAt.After(at) {
		return errors.New("validation envelope provenance or validity interval is incomplete")
	}
	for name, digest := range envelope.ModelSHA256 {
		if name == "" || !sha256Pattern.MatchString(digest) {
			return fmt.Errorf("invalid model digest %q", name)
		}
	}
	for name, digest := range envelope.ToolDigests {
		if name == "" || !sha256Pattern.MatchString(digest) {
			return fmt.Errorf("invalid tool digest %q", name)
		}
	}
	for name, tolerance := range envelope.Tolerances {
		if name == "" || tolerance < 0 || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
			return fmt.Errorf("invalid tolerance %q", name)
		}
	}
	if envelope.SignedBy == "" || envelope.Signature == "" {
		return errors.New("validation envelope reviewer signature is required")
	}
	return nil
}
