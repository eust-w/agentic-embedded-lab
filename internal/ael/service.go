package ael

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/google/uuid"
)

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type RunRequest struct {
	ProjectID      string `json:"project_id"`
	ExperimentPath string `json:"experiment_path"`
	SystemPath     string `json:"system_path"`
	SourceRevision string `json:"source_revision"`
}

type RunRecord struct {
	ID           string          `json:"id"`
	Request      RunRequest      `json:"request"`
	Status       RunStatus       `json:"status"`
	EvidencePath string          `json:"evidence_path,omitempty"`
	Bundle       *EvidenceBundle `json:"bundle,omitempty"`
	Error        string          `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Comparison struct {
	LeftRunID       string `json:"left_run_id"`
	RightRunID      string `json:"right_run_id"`
	TraceEqual      bool   `json:"trace_equal"`
	AssertionsLeft  int    `json:"assertions_left"`
	AssertionsRight int    `json:"assertions_right"`
	LeftFailed      bool   `json:"left_failed"`
	RightFailed     bool   `json:"right_failed"`
}

type RunManager struct {
	store             *store.Store
	backendExecutable string
	mu                sync.Mutex
	projects          map[string]string
	cancels           map[string]context.CancelFunc
}

func NewRunManager(state *store.Store, backendExecutable string) *RunManager {
	return &RunManager{store: state, backendExecutable: backendExecutable, projects: make(map[string]string), cancels: make(map[string]context.CancelFunc)}
}

func (m *RunManager) RegisterProject(projectID, root string) error {
	root, err := filepath.Abs(root)
	if err != nil || projectID == "" {
		return errors.New("project id and root are required")
	}
	m.mu.Lock()
	m.projects[projectID] = root
	m.mu.Unlock()
	m.recoverProject(projectID, root)
	return nil
}

func (m *RunManager) recoverProject(projectID, root string) {
	rows, err := m.store.DB().Query(`SELECT id, request_json FROM ael_runs WHERE project_id=? AND status IN ('queued','running')`, projectID)
	if err != nil {
		return
	}
	type pendingRun struct {
		id      string
		request RunRequest
	}
	var pending []pendingRun
	for rows.Next() {
		var id string
		var payload []byte
		if rows.Scan(&id, &payload) != nil {
			continue
		}
		var request RunRequest
		if json.Unmarshal(payload, &request) != nil {
			continue
		}
		pending = append(pending, pendingRun{id: id, request: request})
	}
	_ = rows.Close()
	for _, item := range pending {
		m.mu.Lock()
		if m.cancels[item.id] != nil {
			m.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.cancels[item.id] = cancel
		m.mu.Unlock()
		_, _ = m.store.DB().Exec(`UPDATE ael_runs SET status='queued', error='resuming after daemon restart', updated_at=? WHERE id=?`, formatServiceTime(time.Now().UTC()), item.id)
		go m.execute(ctx, item.id, root, item.request)
	}
}

func (m *RunManager) Start(ctx context.Context, request RunRequest) (RunRecord, error) {
	m.mu.Lock()
	root := m.projects[request.ProjectID]
	m.mu.Unlock()
	if root == "" || request.ExperimentPath == "" || request.SystemPath == "" {
		return RunRecord{}, errors.New("registered project, experiment path, and system path are required")
	}
	now := time.Now().UTC()
	record := RunRecord{ID: uuid.NewString(), Request: request, Status: RunQueued, CreatedAt: now, UpdatedAt: now}
	payload, _ := json.Marshal(request)
	if _, err := m.store.DB().ExecContext(ctx, `INSERT INTO ael_runs(id, project_id, status, request_json, created_at, updated_at) VALUES(?,?,?,?,?,?)`, record.ID, request.ProjectID, record.Status, payload, formatServiceTime(now), formatServiceTime(now)); err != nil {
		return RunRecord{}, err
	}
	runContext, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[record.ID] = cancel
	m.mu.Unlock()
	go m.execute(runContext, record.ID, root, request)
	return record, nil
}

func (m *RunManager) execute(ctx context.Context, id, root string, request RunRequest) {
	m.update(id, RunRunning, nil, "")
	engine := Engine{Workspace: root, BackendExecutable: m.backendExecutable}
	bundle, evidencePath, err := engine.RunFiles(ctx, request.ExperimentPath, request.SystemPath, request.SourceRevision)
	status := RunCompleted
	errorMessage := ""
	if err != nil {
		status, errorMessage = RunFailed, err.Error()
		if errors.Is(err, context.Canceled) {
			status = RunCancelled
		}
	} else {
		for _, assertion := range bundle.Assertions {
			if !assertion.Passed {
				status, errorMessage = RunFailed, "one or more experiment assertions failed"
				break
			}
		}
	}
	m.update(id, status, &struct {
		EvidencePath string         `json:"evidence_path"`
		Bundle       EvidenceBundle `json:"bundle"`
	}{EvidencePath: evidencePath, Bundle: bundle}, errorMessage)
	m.mu.Lock()
	delete(m.cancels, id)
	m.mu.Unlock()
}

func (m *RunManager) update(id string, status RunStatus, result any, message string) {
	var payload []byte
	if result != nil {
		payload, _ = json.Marshal(result)
	}
	_, _ = m.store.DB().Exec(`UPDATE ael_runs SET status=?, result_json=?, error=NULLIF(?,''), updated_at=? WHERE id=?`, status, payload, message, formatServiceTime(time.Now().UTC()), id)
}

func (m *RunManager) Get(ctx context.Context, id string) (RunRecord, error) {
	var record RunRecord
	var requestPayload, resultPayload []byte
	var created, updated string
	record.ID = id
	if err := m.store.DB().QueryRowContext(ctx, `SELECT status, request_json, COALESCE(result_json,'{}'), COALESCE(error,''), created_at, updated_at FROM ael_runs WHERE id=?`, id).Scan(&record.Status, &requestPayload, &resultPayload, &record.Error, &created, &updated); err != nil {
		return RunRecord{}, err
	}
	if err := json.Unmarshal(requestPayload, &record.Request); err != nil {
		return RunRecord{}, err
	}
	var result struct {
		EvidencePath string          `json:"evidence_path"`
		Bundle       *EvidenceBundle `json:"bundle"`
	}
	if err := json.Unmarshal(resultPayload, &result); err != nil {
		return RunRecord{}, err
	}
	record.EvidencePath, record.Bundle = result.EvidencePath, result.Bundle
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return record, nil
}

func (m *RunManager) Cancel(id string) bool {
	m.mu.Lock()
	cancel, ok := m.cancels[id]
	m.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (m *RunManager) Replay(ctx context.Context, id string) (RunRecord, error) {
	record, err := m.Get(ctx, id)
	if err != nil {
		return RunRecord{}, err
	}
	if record.Bundle == nil {
		return RunRecord{}, errors.New("replay requires an existing evidence bundle")
	}
	m.mu.Lock()
	root := m.projects[record.Request.ProjectID]
	m.mu.Unlock()
	experiment, err := LoadExperiment(root, record.Request.ExperimentPath)
	if err != nil {
		return RunRecord{}, err
	}
	system, err := LoadSystem(root, record.Request.SystemPath)
	if err != nil {
		return RunRecord{}, err
	}
	if canonicalSHA(experiment) != canonicalSHA(record.Bundle.Experiment) || canonicalSHA(system) != canonicalSHA(record.Bundle.System) {
		return RunRecord{}, errors.New("replay input drift detected; experiment or system hash changed")
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return RunRecord{}, err
	}
	head := strings.TrimSpace(string(data))
	if record.Request.SourceRevision != "" && !strings.HasPrefix(head, record.Request.SourceRevision) && !strings.HasPrefix(record.Request.SourceRevision, head) {
		return RunRecord{}, errors.New("replay source revision drift detected")
	}
	return m.Start(ctx, record.Request)
}

func canonicalSHA(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (m *RunManager) Compare(ctx context.Context, leftID, rightID string) (Comparison, error) {
	left, err := m.Get(ctx, leftID)
	if err != nil {
		return Comparison{}, err
	}
	right, err := m.Get(ctx, rightID)
	if err != nil {
		return Comparison{}, err
	}
	if left.Bundle == nil || right.Bundle == nil {
		return Comparison{}, errors.New("both runs must have evidence bundles")
	}
	return Comparison{LeftRunID: leftID, RightRunID: rightID, TraceEqual: left.Bundle.TraceSHA256 == right.Bundle.TraceSHA256, AssertionsLeft: passedAssertions(left.Bundle.Assertions), AssertionsRight: passedAssertions(right.Bundle.Assertions), LeftFailed: left.Bundle.Failure != nil, RightFailed: right.Bundle.Failure != nil}, nil
}

func passedAssertions(assertions []AssertionResult) int {
	count := 0
	for _, assertion := range assertions {
		if assertion.Passed {
			count++
		}
	}
	return count
}

func formatServiceTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
