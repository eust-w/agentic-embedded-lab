package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type WorkerConfig struct {
	ControlPlane, Cert, Key, CA, ID, Workspace, BackendExecutable string
	Capabilities                                                  []string
}
type Worker struct {
	config WorkerConfig
	client *http.Client
}

func NewWorker(config WorkerConfig) (*Worker, error) {
	if config.ControlPlane == "" || config.Cert == "" || config.Key == "" || config.CA == "" || config.ID == "" {
		return nil, errors.New("control plane, mTLS files, and worker id are required")
	}
	certificate, err := tls.LoadX509KeyPair(config.Cert, config.Key)
	if err != nil {
		return nil, err
	}
	caData, err := os.ReadFile(config.CA)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, errors.New("worker CA is invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: pool}}
	return &Worker{config: config, client: &http.Client{Transport: transport, Timeout: 3 * time.Minute}}, nil
}
func (w *Worker) Run(ctx context.Context) error {
	if err := w.post(ctx, "/v1/workers/register", map[string]any{"id": w.config.ID, "capabilities": w.config.Capabilities}, nil); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var response struct {
				Task *Task `json:"task"`
			}
			if err := w.post(ctx, "/v1/workers/"+w.config.ID+"/lease", map[string]any{}, &response); err != nil {
				return err
			}
			if response.Task != nil {
				w.execute(ctx, *response.Task)
			}
		}
	}
}
func (w *Worker) execute(parent context.Context, task Task) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	go w.heartbeat(ctx, task.ID, done)
	result, err := w.handle(ctx, task)
	close(done)
	path := "/v1/workers/" + w.config.ID + "/tasks/" + task.ID + "/complete"
	payload := map[string]any{"result": result}
	if err != nil {
		payload = map[string]any{"result": map[string]any{"error": err.Error(), "status": "failed"}}
	}
	_ = w.post(parent, path, payload, nil)
}
func (w *Worker) heartbeat(ctx context.Context, taskID string, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			_ = w.post(ctx, "/v1/workers/"+w.config.ID+"/heartbeat", map[string]string{"task_id": taskID}, nil)
		}
	}
}
func (w *Worker) handle(ctx context.Context, task Task) (map[string]any, error) {
	kind, _ := task.Payload["kind"].(string)
	switch kind {
	case "health_probe":
		return map[string]any{"status": "passed", "worker": w.config.ID, "attempt": task.Attempt, "echo": task.Payload["input"]}, nil
	case "ael_experiment":
		experiment, _ := task.Payload["experiment_path"].(string)
		system, _ := task.Payload["system_path"].(string)
		revision, _ := task.Payload["source_revision"].(string)
		if experiment == "" || system == "" {
			return nil, errors.New("AEL task requires experiment_path and system_path")
		}
		bundle, evidence, err := (ael.Engine{Workspace: w.config.Workspace, BackendExecutable: w.config.BackendExecutable}).RunFiles(ctx, experiment, system, revision)
		return map[string]any{"status": statusFor(err), "evidence_path": evidence, "bundle": bundle}, err
	default:
		return nil, fmt.Errorf("unsupported worker task kind %q", kind)
	}
}
func statusFor(err error) string {
	if err != nil {
		return "failed"
	}
	return "passed"
}
func (w *Worker) post(ctx context.Context, path string, input, target any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(w.config.ControlPlane, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		return fmt.Errorf("control plane HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if target != nil {
		return json.NewDecoder(response.Body).Decode(target)
	}
	return nil
}
