package ael

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type EvidenceWriter struct {
	Workspace       string
	ArtifactSources map[string]string
}

func (w EvidenceWriter) Write(bundle EvidenceBundle) (string, error) {
	if bundle.RunID == "" || strings.ContainsAny(bundle.RunID, `/\\`) {
		return "", errors.New("evidence run id is invalid")
	}
	root, err := filepath.Abs(w.Workspace)
	if err != nil {
		return "", err
	}
	runsRoot := filepath.Join(root, "runs")
	if err := os.MkdirAll(runsRoot, 0o700); err != nil {
		return "", err
	}
	finalPath := filepath.Join(runsRoot, bundle.RunID)
	if _, err := os.Stat(finalPath); err == nil {
		return "", errors.New("evidence bundle already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary, err := os.MkdirTemp(runsRoot, ".ael-evidence-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	if err := os.MkdirAll(filepath.Join(temporary, "snapshots"), 0o700); err != nil {
		return "", err
	}
	if len(w.ArtifactSources) > 0 {
		if err := os.MkdirAll(filepath.Join(temporary, "artifacts"), 0o700); err != nil {
			return "", err
		}
		for key, source := range w.ArtifactSources {
			relative, digest, err := copyEvidenceArtifact(temporary, key, source)
			if err != nil {
				return "", err
			}
			bundle.Artifacts[key] = relative + "#sha256=" + digest
		}
	}
	files := map[string]any{
		"system.resolved.json":     bundle.System,
		"experiment.resolved.json": bundle.Experiment,
		"provenance.json": map[string]any{
			"api_version":      bundle.APIVersion,
			"run_id":           bundle.RunID,
			"source_revision":  bundle.SourceRevision,
			"trace_sha256":     bundle.TraceSHA256,
			"started_at":       bundle.StartedAt,
			"finished_at":      bundle.FinishedAt,
			"fidelity":         bundle.Fidelity,
			"failure":          bundle.Failure,
			"hardware_claimed": false,
		},
		"assertions.json": bundle.Assertions,
		"artifacts.json":  bundle.Artifacts,
	}
	for name, value := range files {
		if err := writeJSONFile(filepath.Join(temporary, name), value); err != nil {
			return "", err
		}
	}
	if err := writeEvents(filepath.Join(temporary, "events.jsonl"), bundle.Events); err != nil {
		return "", err
	}
	if err := writeJUnit(filepath.Join(temporary, "junit.xml"), bundle); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(temporary, "summary.md"), []byte(evidenceSummary(bundle)), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func copyEvidenceArtifact(root, key, source string) (string, string, error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", errors.New("evidence artifact must be a regular file")
	}
	safeKey := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(key)
	relative := filepath.ToSlash(filepath.Join("artifacts", safeKey, filepath.Base(source)))
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", "", err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", "", err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return "", "", copyErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	return relative, hex.EncodeToString(hash.Sum(nil)), nil
}

func writeJSONFile(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o600)
}

func writeEvents(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return writer.Flush()
}

type junitSuite struct {
	XMLName  xml.Name    `xml:"testsuite"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name    string        `xml:"name,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeJUnit(path string, bundle EvidenceBundle) error {
	suite := junitSuite{Name: bundle.Experiment.ID, Tests: len(bundle.Assertions)}
	for _, assertion := range bundle.Assertions {
		item := junitCase{Name: assertion.ID}
		if !assertion.Passed {
			suite.Failures++
			item.Failure = &junitFailure{Message: assertion.Message, Text: fmt.Sprintf("observed=%g expected=%g", assertion.Observed, assertion.Expected)}
		}
		suite.Cases = append(suite.Cases, item)
	}
	payload, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}
	payload = append([]byte(xml.Header), append(payload, '\n')...)
	return os.WriteFile(path, payload, 0o600)
}

func evidenceSummary(bundle EvidenceBundle) string {
	passed := 0
	for _, assertion := range bundle.Assertions {
		if assertion.Passed {
			passed++
		}
	}
	status := "完成"
	if bundle.Failure != nil {
		status = "失败：" + bundle.Failure.Message
	} else if passed != len(bundle.Assertions) {
		status = "断言未全部通过"
	}
	return fmt.Sprintf(`# AEL 实验证据摘要

- 运行 ID：%s
- 实验：%s
- 状态：%s
- 断言：%d/%d 通过
- Trace SHA-256：%s

## 已证明

本次运行只证明记录的工具、模型版本、输入、断言与 Fidelity 边界内的软件/仿真行为。

## 未证明

- 未证明真实硬件行为、寄存器完整等价、实际时序、电气、功耗、热、RF 或 EMI/EMC 性能。
- 未提供签名 Validation Envelope，因此不得生成 hardware-validated 或 production-approved Claim。
`, bundle.RunID, bundle.Experiment.ID, status, passed, len(bundle.Assertions), bundle.TraceSHA256)
}
