package ael

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const BackendProtocolVersion = "ael.backend/v2"

type BackendRequest struct {
	APIVersion    string         `json:"api_version"`
	ID            string         `json:"id"`
	Operation     string         `json:"operation"`
	VirtualTimeUS int64          `json:"virtual_time_us,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
}

type BackendResponse struct {
	APIVersion string             `json:"api_version"`
	ID         string             `json:"id"`
	OK         bool               `json:"ok"`
	Error      string             `json:"error,omitempty"`
	Outputs    map[string]float64 `json:"outputs,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Events     []Event            `json:"events,omitempty"`
	Artifacts  map[string]string  `json:"artifacts,omitempty"`
}

type ProcessConfig struct {
	Executable string
	Arguments  []string
	Directory  string
	Timeout    time.Duration
}

type ProcessAdapter struct {
	config  ProcessConfig
	process *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *ringBuffer
	mu      sync.Mutex
	nextID  int64
}

func NewProcessAdapter(config ProcessConfig) (*ProcessAdapter, error) {
	if config.Executable == "" || config.Directory == "" {
		return nil, errors.New("backend executable and directory are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	return &ProcessAdapter{config: config, stderr: newRingBuffer(64 << 10)}, nil
}

func (p *ProcessAdapter) Prepare(ctx context.Context, component Component, seed int64) error {
	if err := p.start(ctx); err != nil {
		return err
	}
	_, err := p.call(ctx, "prepare", 0, map[string]any{"component": component, "seed": seed})
	return err
}

func (p *ProcessAdapter) Inject(ctx context.Context, target string, value any, virtualTimeUS int64) ([]Event, error) {
	response, err := p.call(ctx, "inject", virtualTimeUS, map[string]any{"target": target, "value": value})
	return response.Events, err
}

func (p *ProcessAdapter) Step(ctx context.Context, virtualTimeUS, stepUS int64) (StepResult, error) {
	response, err := p.call(ctx, "step", virtualTimeUS, map[string]any{"step_us": stepUS})
	return StepResult{Outputs: response.Outputs, Metrics: response.Metrics, Events: response.Events, Artifacts: response.Artifacts}, err
}

func (p *ProcessAdapter) Snapshot(ctx context.Context, virtualTimeUS int64) (string, error) {
	response, err := p.call(ctx, "snapshot", virtualTimeUS, nil)
	if err != nil {
		return "", err
	}
	return response.Artifacts["snapshot"], nil
}
func (p *ProcessAdapter) Restore(ctx context.Context, snapshot string) error {
	_, err := p.call(ctx, "restore", 0, map[string]any{"snapshot": snapshot})
	return err
}

func (p *ProcessAdapter) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil {
		return nil
	}
	_ = p.writeRequest(BackendRequest{APIVersion: BackendProtocolVersion, ID: "shutdown", Operation: "shutdown"})
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	done := make(chan error, 1)
	go func() { done <- p.process.Wait() }()
	select {
	case <-ctx.Done():
		_ = p.process.Process.Kill()
	case <-time.After(3 * time.Second):
		_ = p.process.Process.Kill()
	case <-done:
	}
	p.process, p.stdin, p.stdout = nil, nil, nil
	return nil
}

func (p *ProcessAdapter) start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process != nil {
		return nil
	}
	executable, err := exec.LookPath(p.config.Executable)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, executable, p.config.Arguments...)
	command.Dir = p.config.Directory
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = p.stderr
	if err := command.Start(); err != nil {
		return err
	}
	p.process, p.stdin, p.stdout = command, stdin, bufio.NewReader(stdout)
	return nil
}

func (p *ProcessAdapter) call(ctx context.Context, operation string, virtualTimeUS int64, payload map[string]any) (BackendResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil || p.stdin == nil || p.stdout == nil {
		return BackendResponse{}, errors.New("backend process is not running")
	}
	p.nextID++
	request := BackendRequest{APIVersion: BackendProtocolVersion, ID: fmt.Sprintf("request-%d", p.nextID), Operation: operation, VirtualTimeUS: virtualTimeUS, Payload: payload}
	if err := p.writeRequest(request); err != nil {
		return BackendResponse{}, err
	}
	type result struct {
		response BackendResponse
		err      error
	}
	channel := make(chan result, 1)
	go func() {
		line, err := p.stdout.ReadBytes('\n')
		var response BackendResponse
		if err == nil {
			err = json.Unmarshal(line, &response)
		}
		channel <- result{response: response, err: err}
	}()
	timer := time.NewTimer(p.config.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return BackendResponse{}, ctx.Err()
	case <-timer.C:
		return BackendResponse{}, fmt.Errorf("backend timeout; stderr=%s", p.stderr.String())
	case outcome := <-channel:
		if outcome.err != nil {
			return BackendResponse{}, fmt.Errorf("read backend response: %w; stderr=%s", outcome.err, p.stderr.String())
		}
		if outcome.response.APIVersion != BackendProtocolVersion || outcome.response.ID != request.ID {
			return BackendResponse{}, errors.New("backend response does not match request")
		}
		if !outcome.response.OK {
			return BackendResponse{}, errors.New(outcome.response.Error)
		}
		return outcome.response, nil
	}
}

func (p *ProcessAdapter) writeRequest(request BackendRequest) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = p.stdin.Write(append(payload, '\n'))
	return err
}

type ringBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func newRingBuffer(limit int) *ringBuffer { return &ringBuffer{limit: limit} }

func (r *ringBuffer) Write(payload []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, payload...)
	if len(r.data) > r.limit {
		r.data = append([]byte(nil), r.data[len(r.data)-r.limit:]...)
	}
	return len(payload), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(append([]byte(nil), r.data...))
}
