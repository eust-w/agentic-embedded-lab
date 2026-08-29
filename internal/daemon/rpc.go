package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/executor"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
)

type Request struct {
	APIVersion string          `json:"api_version"`
	ID         string          `json:"id"`
	Token      string          `json:"token"`
	Method     string          `json:"method"`
	Params     json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	APIVersion string `json:"api_version"`
	ID         string `json:"id"`
	Result     any    `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Server struct {
	SocketPath string
	Token      string
	Runtime    *agent.Runtime
	Agents     *multiagent.Manager
}

func (s *Server) Listen(ctx context.Context) error {
	if s.SocketPath == "" || s.Token == "" || s.Runtime == nil {
		return errors.New("socket path, token, and runtime are required")
	}
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(s.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing to replace non-socket path %s", s.SocketPath)
		}
		if err := os.Remove(s.SocketPath); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(s.SocketPath)
	if err := os.Chmod(s.SocketPath, 0o600); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.serveConnection(ctx, connection)
	}
}

func (s *Server) serveConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	scanner := bufio.NewScanner(connection)
	encoder := json.NewEncoder(connection)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(Response{APIVersion: protocol.APIVersion, Error: "invalid request"})
			continue
		}
		response := s.dispatch(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, request Request) Response {
	response := Response{APIVersion: protocol.APIVersion, ID: request.ID}
	if request.APIVersion != protocol.APIVersion {
		response.Error = "unsupported api_version"
		return response
	}
	if request.Token != s.Token {
		response.Error = "unauthorized"
		return response
	}
	switch request.Method {
	case "health":
		response.Result = map[string]any{"status": "ready", "time": time.Now().UTC()}
	case "project.open":
		var params struct {
			ProjectID  string                     `json:"project_id"`
			Root       string                     `json:"root"`
			Permission protocol.PermissionProfile `json:"permission"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.ProjectID == "" || !filepath.IsAbs(params.Root) {
			response.Error = "project id and absolute root are required"
			break
		}
		info, err := os.Stat(params.Root)
		if err != nil || !info.IsDir() {
			response.Error = "project root is not an accessible directory"
			break
		}
		registry := tools.NewRegistry()
		_ = registry.Register(tools.FileTool{Workspace: params.Root})
		_ = registry.Register(tools.SearchTool{Workspace: params.Root, MaxResults: 200})
		_ = registry.Register(tools.CommandTool{Workspace: params.Root, Executor: executor.New(), Profile: params.Permission})
		s.Runtime.RegisterProject(params.ProjectID, params.Root)
		s.Runtime.ConfigureProjectTools(params.ProjectID, registry, approval.New(), plugins.NewHookDispatcher())
		if s.Agents != nil {
			_ = s.Agents.ConfigureProject(params.ProjectID, params.Root, filepath.Join(filepath.Dir(params.Root), ".aether-worktrees"))
		}
		response.Result = map[string]any{"project_id": params.ProjectID, "root": params.Root, "tools": registry.Definitions()}
	case "thread.create":
		var params struct {
			ProjectID  string                     `json:"project_id"`
			Title      string                     `json:"title"`
			Model      string                     `json:"model"`
			Permission protocol.PermissionProfile `json:"permission"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		thread, err := s.Runtime.CreateThread(ctx, params.ProjectID, params.Title, params.Model, params.Permission)
		response.Result, response.Error = resultOrError(thread, err)
	case "thread.list":
		var params struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		threads, err := s.Runtime.ListThreads(ctx, params.ProjectID)
		response.Result, response.Error = resultOrError(threads, err)
	case "thread.items":
		var params struct {
			ThreadID string `json:"thread_id"`
			After    int64  `json:"after"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		items, err := s.Runtime.Items(ctx, params.ThreadID, params.After, params.Limit)
		response.Result, response.Error = resultOrError(items, err)
	case "turn.run":
		var params struct {
			Thread protocol.Thread `json:"thread"`
			Input  string          `json:"input"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		turn, err := s.Runtime.RunTurn(ctx, params.Thread, params.Input)
		response.Result, response.Error = resultOrError(turn, err)
	case "turn.cancel":
		var params struct {
			TurnID string `json:"turn_id"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		response.Result = map[string]bool{"cancelled": s.Runtime.CancelTurn(params.TurnID)}
	case "approval.resolve":
		var params struct {
			ApprovalID string `json:"approval_id"`
			Allow      bool   `json:"allow"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		response.Result = map[string]bool{"resolved": s.Runtime.ResolveApproval(params.ApprovalID, params.Allow)}
	case "agent.spawn":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		var params struct {
			Parent protocol.Thread    `json:"parent"`
			Prompt string             `json:"prompt"`
			Spec   protocol.AgentSpec `json:"spec"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		handle, err := s.Agents.Spawn(ctx, params.Parent, params.Parent.ProjectID, params.Prompt, params.Spec)
		response.Result, response.Error = resultOrError(handle, err)
	case "agent.list":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		response.Result = s.Agents.List()
	case "agent.message":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		var params struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		turn, err := s.Agents.Message(ctx, params.ID, params.Message)
		response.Result, response.Error = resultOrError(turn, err)
	case "agent.interrupt":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = map[string]bool{"interrupted": s.Agents.Interrupt(params.ID)}
	case "agent.result":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		result, err := s.Agents.Result(ctx, params.ID)
		response.Result, response.Error = resultOrError(result, err)
	case "agent.close":
		if s.Agents == nil {
			response.Error = "multi-agent runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		err := s.Agents.CloseAgent(ctx, params.ID)
		response.Result, response.Error = resultOrError(map[string]bool{"closed": err == nil}, err)
	default:
		response.Error = "unknown method"
	}
	return response
}

func resultOrError[T any](value T, err error) (any, string) {
	if err != nil {
		return nil, err.Error()
	}
	return value, ""
}

type Client struct {
	SocketPath string
	Token      string
}

func (c *Client) Call(ctx context.Context, request Request, result any) error {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return err
	}
	defer connection.Close()
	request.APIVersion = protocol.APIVersion
	request.Token = c.Token
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}
	var response Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	if result == nil {
		return nil
	}
	payload, err := json.Marshal(response.Result)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}
