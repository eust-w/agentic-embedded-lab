package workers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

var (
	metricPattern = regexp.MustCompile(`^AEL_METRIC\s+([A-Za-z0-9_.-]+)=(.+)$`)
	eventPattern  = regexp.MustCompile(`^AEL_EVENT\s+([A-Za-z0-9_.-]+)(?:\s+(.+))?$`)
)

type Implementation interface {
	Backend() ael.Backend
	ExpectedVersion() string
	Commands() []string
	VersionArguments() []string
	Prepare(context.Context, *State) error
	Step(context.Context, *State, int64) (ael.StepResult, error)
}
type Checkpointable interface {
	Checkpoint() map[string]any
	RestoreCheckpoint(map[string]any) error
}

type VersionDetector interface {
	DetectVersion(context.Context, string, string) string
}

type State struct {
	Workspace     string
	Component     ael.Component
	Seed          int64
	Inputs        map[string]any
	VirtualTimeUS int64
	RuntimeDir    string
	ArtifactRoot  string
	Tool          string
	Version       string
}

type Server struct {
	implementation Implementation
	state          State
}

func NewServer(workspace string, implementation Implementation) (*Server, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	tool, version := resolveTool(implementation)
	artifactRoot := os.Getenv("AEL_RUNTIME_ROOT")
	if artifactRoot == "" {
		artifactRoot = filepath.Join(root, ".ael", "backend-runtime")
	}
	artifactRoot, err = filepath.Abs(artifactRoot)
	if err != nil {
		return nil, err
	}
	return &Server{implementation: implementation, state: State{Workspace: root, ArtifactRoot: artifactRoot, Inputs: make(map[string]any), Tool: tool, Version: version}}, nil
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request ael.BackendRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(ael.BackendResponse{APIVersion: ael.BackendProtocolVersion, ID: "invalid", OK: false, Error: "invalid request"})
			continue
		}
		response := s.handle(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
		if request.Operation == "shutdown" {
			return nil
		}
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, request ael.BackendRequest) (response ael.BackendResponse) {
	response = ael.BackendResponse{APIVersion: ael.BackendProtocolVersion, ID: request.ID, OK: true}
	defer func() {
		if recovered := recover(); recovered != nil {
			response.OK = false
			response.Error = fmt.Sprint(recovered)
		}
	}()
	if request.APIVersion != ael.BackendProtocolVersion {
		return failure(request.ID, "unsupported api version")
	}
	switch request.Operation {
	case "probe":
		response.Outputs = map[string]float64{"available": boolMetric(s.state.Tool != "" && s.state.Version == s.implementation.ExpectedVersion())}
		return response
	case "prepare":
		componentData, err := json.Marshal(request.Payload["component"])
		if err != nil {
			return failure(request.ID, err.Error())
		}
		if err := json.Unmarshal(componentData, &s.state.Component); err != nil {
			return failure(request.ID, err.Error())
		}
		seed, _ := numeric(request.Payload["seed"])
		s.state.Seed = int64(seed)
		runtimeRoot := s.state.ArtifactRoot
		if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
			return failure(request.ID, err.Error())
		}
		directory, err := os.MkdirTemp(runtimeRoot, "ael-"+string(s.implementation.Backend())+"-")
		if err != nil {
			return failure(request.ID, err.Error())
		}
		s.state.RuntimeDir = directory
		if s.state.Tool == "" || s.state.Version != s.implementation.ExpectedVersion() {
			return failure(request.ID, fmt.Sprintf("backend unavailable: expected %s, detected %s", s.implementation.ExpectedVersion(), s.state.Version))
		}
		if err := s.implementation.Prepare(ctx, &s.state); err != nil {
			return failure(request.ID, err.Error())
		}
	case "inject":
		target, _ := request.Payload["target"].(string)
		key := target
		if index := strings.LastIndex(target, "."); index >= 0 {
			key = target[index+1:]
		}
		s.state.Inputs[key] = request.Payload["value"]
		response.Events = []ael.Event{{Type: string(s.implementation.Backend()) + ".inject", Payload: map[string]any{"target": target, "value": request.Payload["value"]}, FidelityRef: string(s.implementation.Backend()) + ":tool-executed"}}
	case "step":
		step, _ := numeric(request.Payload["step_us"])
		s.state.VirtualTimeUS = request.VirtualTimeUS
		result, err := s.implementation.Step(ctx, &s.state, int64(step))
		if err != nil {
			return failure(request.ID, err.Error())
		}
		if err := finite(result); err != nil {
			return failure(request.ID, err.Error())
		}
		s.state.VirtualTimeUS += int64(step)
		response.Outputs, response.Metrics, response.Events, response.Artifacts = result.Outputs, result.Metrics, result.Events, result.Artifacts
	case "snapshot":
		path := filepath.Join(s.state.RuntimeDir, fmt.Sprintf("snapshot-%d.json", s.state.VirtualTimeUS))
		implementation := map[string]any{}
		if checkpointable, ok := s.implementation.(Checkpointable); ok {
			implementation = checkpointable.Checkpoint()
		}
		payload, _ := json.MarshalIndent(map[string]any{"state": s.state, "implementation": implementation}, "", "  ")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return failure(request.ID, err.Error())
		}
		response.Artifacts = map[string]string{"snapshot": relativeArtifact(&s.state, path)}
	case "restore":
		reference, _ := request.Payload["snapshot"].(string)
		if !strings.HasPrefix(reference, "ael-runtime://") {
			return failure(request.ID, "invalid snapshot reference")
		}
		path := filepath.Join(s.state.ArtifactRoot, filepath.FromSlash(strings.TrimPrefix(reference, "ael-runtime://")))
		relative, err := filepath.Rel(s.state.RuntimeDir, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return failure(request.ID, "snapshot escapes runtime")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return failure(request.ID, err.Error())
		}
		var snapshot struct {
			State          State          `json:"state"`
			Implementation map[string]any `json:"implementation"`
		}
		if json.Unmarshal(data, &snapshot) != nil {
			return failure(request.ID, "invalid snapshot")
		}
		if snapshot.State.Workspace != s.state.Workspace || snapshot.State.RuntimeDir != s.state.RuntimeDir {
			return failure(request.ID, "snapshot identity mismatch")
		}
		s.state = snapshot.State
		if checkpointable, ok := s.implementation.(Checkpointable); ok {
			if err := checkpointable.RestoreCheckpoint(snapshot.Implementation); err != nil {
				return failure(request.ID, err.Error())
			}
		}
	case "shutdown":
		s.state.Component = ael.Component{}
	default:
		return failure(request.ID, "unsupported operation")
	}
	return response
}

func RunTool(ctx context.Context, state *State, arguments []string, timeout time.Duration, environment map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, state.Tool, arguments...)
	command.Dir = state.RuntimeDir
	command.Env = append(os.Environ(), "AEL_SEED="+strconv.FormatInt(state.Seed, 10))
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s exited: %w: %s", state.Component.Backend, err, diagnostic(output))
	}
	return output, nil
}

func RunToolObserved(ctx context.Context, state *State, arguments []string, timeout time.Duration, environment map[string]string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, state.Tool, arguments...)
	command.Dir = state.RuntimeDir
	command.Env = append(os.Environ(), "AEL_SEED="+strconv.FormatInt(state.Seed, 10))
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return output, -1, ctx.Err()
	}
	if err == nil {
		return output, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return output, exitError.ExitCode(), nil
	}
	return output, -1, err
}

func ParseOutput(state *State, output []byte, virtualTimeUS int64) (map[string]float64, []ael.Event) {
	metrics := make(map[string]float64)
	var events []ael.Event
	for _, raw := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(raw)
		if match := metricPattern.FindStringSubmatch(line); match != nil {
			if value, err := strconv.ParseFloat(match[2], 64); err == nil {
				metrics[match[1]] = value
			}
		}
		if match := eventPattern.FindStringSubmatch(line); match != nil {
			payload := make(map[string]any)
			if match[2] != "" {
				if err := json.Unmarshal([]byte(match[2]), &payload); err != nil {
					payload["message"] = match[2]
				}
			}
			events = append(events, ael.Event{VirtualTimeUS: virtualTimeUS, Source: state.Component.ID, Type: match[1], Payload: payload, FidelityRef: string(state.Component.Backend) + ":tool-executed"})
		}
	}
	return metrics, events
}

func WorkspacePath(state *State, value string, mustExist bool) (string, error) {
	target, err := filepath.Abs(filepath.Join(state.Workspace, value))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(state.Workspace, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("backend path escapes workspace")
	}
	if mustExist {
		if _, err := os.Stat(target); err != nil {
			return "", err
		}
	}
	return target, nil
}

func relativeArtifact(state *State, path string) string {
	for _, root := range []struct{ path, prefix string }{{state.Workspace, ""}, {state.ArtifactRoot, "ael-runtime://"}} {
		relative, err := filepath.Rel(root.path, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return root.prefix + filepath.ToSlash(relative)
		}
	}
	panic("backend artifact is outside approved roots")
}

func resolveTool(implementation Implementation) (string, string) {
	for _, command := range implementation.Commands() {
		path, err := exec.LookPath(command)
		if err != nil {
			continue
		}
		if detector, ok := implementation.(VersionDetector); ok {
			if version := detector.DetectVersion(context.Background(), path, ""); version != "" {
				return path, version
			}
			continue
		}
		for _, argument := range implementation.VersionArguments() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			output, _ := exec.CommandContext(ctx, path, argument).CombinedOutput()
			cancel()
			if strings.Contains(string(output), implementation.ExpectedVersion()) {
				return path, implementation.ExpectedVersion()
			}
		}
	}
	return "", ""
}

func finite(result ael.StepResult) error {
	for name, value := range result.Metrics {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("non-finite metric %s", name)
		}
	}
	for name, value := range result.Outputs {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("non-finite output %s", name)
		}
	}
	return nil
}

func numeric(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func boolMetric(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func failure(id, message string) ael.BackendResponse {
	return ael.BackendResponse{APIVersion: ael.BackendProtocolVersion, ID: id, OK: false, Error: message}
}

func diagnostic(output []byte) string {
	if len(output) <= 6000 {
		return string(output)
	}
	return string(output[:3000]) + "\n...truncated...\n" + string(output[len(output)-3000:])
}
