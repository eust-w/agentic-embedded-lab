package daemon

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/automation"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/computeruse"
	"github.com/eust-w/agentic-embedded-lab/internal/executor"
	"github.com/eust-w/agentic-embedded-lab/internal/mcp"
	aethermemory "github.com/eust-w/agentic-embedded-lab/internal/memory"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/terminal"
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
	Proof      string `json:"proof,omitempty"`
}

type Server struct {
	SocketPath     string
	Token          string
	Runtime        *agent.Runtime
	Agents         *multiagent.Manager
	AEL            *ael.RunManager
	Models         *modeling.Manager
	ChromeSessions *browser.ChromeSessionStore
	Browser        *browser.Controller
	Computer       *computeruse.Controller
	Terminals      *terminal.Manager
	Automations    *automation.Scheduler
	State          *store.Store
	PluginRegistry *plugins.Registry
	Memory         *aethermemory.Repository
	extensionMu    sync.Mutex
	mcpClients     map[string]*mcp.Client
	processPlugins map[string]*plugins.ProcessRuntime
}

func (s *Server) ConfigureProject(ctx context.Context, project store.ProjectRecord, persist bool) ([]protocol.ToolDefinition, error) {
	if s.Runtime == nil || project.ID == "" || !filepath.IsAbs(project.Root) {
		return nil, errors.New("runtime, project id, and absolute root are required")
	}
	root, err := filepath.EvalSymlinks(project.Root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("project root is not an accessible directory")
	}
	registry := tools.NewRegistry()
	_ = registry.Register(tools.FileTool{Workspace: root})
	_ = registry.Register(tools.SearchTool{Workspace: root, MaxResults: 200})
	_ = registry.Register(tools.GitReadTool{Workspace: root})
	_ = registry.Register(tools.GitWriteTool{Workspace: root})
	_ = registry.Register(tools.AELRouteTool{})
	_ = registry.Register(tools.CommandTool{Workspace: root, Executor: executor.New(), Profile: project.Permission})
	if s.Browser != nil {
		_ = registry.Register(tools.BrowserTool{Controller: s.Browser})
	}
	if s.Computer != nil {
		_ = registry.Register(tools.ComputerTool{Controller: s.Computer})
	}
	if previous := s.Runtime.ProjectHooks(project.ID); previous != nil {
		_, _ = previous.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookSessionEnd, ThreadID: project.ID})
	}
	s.Runtime.RegisterProject(project.ID, root)
	hooks := plugins.NewHookDispatcher()
	if err := s.configurePlugins(ctx, project.ID, registry, hooks); err != nil {
		return nil, err
	}
	if results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookSessionStart, ThreadID: project.ID, Data: map[string]any{"root": root}}); err != nil {
		return nil, err
	} else {
		for _, result := range results {
			if result.Block {
				return nil, fmt.Errorf("project session blocked by hook: %s", result.Reason)
			}
		}
	}
	s.Runtime.ConfigureProjectTools(project.ID, registry, approval.New(), hooks)
	if s.Agents != nil {
		if err := s.Agents.ConfigureProject(project.ID, root, filepath.Join(filepath.Dir(root), ".aether-worktrees")); err != nil {
			return nil, err
		}
	}
	if s.AEL != nil {
		if err := s.AEL.RegisterProject(project.ID, root); err != nil {
			return nil, err
		}
	}
	if s.Models != nil {
		s.Models.RegisterProject(project.ID, root)
	}
	if s.Terminals != nil {
		if err := s.Terminals.RegisterWorkspace(root); err != nil {
			return nil, err
		}
	}
	if persist && s.State != nil {
		project.Root = root
		if err := s.State.SaveProject(ctx, project); err != nil {
			return nil, err
		}
	}
	return registry.Definitions(), nil
}

func (s *Server) configurePlugins(ctx context.Context, projectID string, registry *tools.Registry, hooks *plugins.HookDispatcher) error {
	if s.PluginRegistry == nil {
		return nil
	}
	installed, err := s.PluginRegistry.List()
	if err != nil {
		return err
	}
	for _, plugin := range installed {
		if !plugin.Active || plugin.Revoked {
			continue
		}
		for _, relative := range plugin.Manifest.Skills {
			path := filepath.Join(plugin.Path, relative)
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			var skillPaths []string
			if info.IsDir() {
				metadata, err := plugins.DiscoverSkills([]string{path})
				if err != nil {
					return err
				}
				for _, item := range metadata {
					skillPaths = append(skillPaths, item.Path)
				}
			} else {
				skillPaths = []string{path}
			}
			for _, skillPath := range skillPaths {
				skill, err := plugins.LoadSkill(skillPath)
				if err != nil {
					return err
				}
				s.Runtime.AddProjectInstruction(projectID, "Plugin skill "+skill.Name+": "+skill.Description+"\n\n"+skill.Instructions)
			}
		}
		for _, relative := range plugin.Manifest.Hooks {
			config, err := plugins.LoadHookConfig(filepath.Join(plugin.Path, relative))
			if err != nil {
				return err
			}
			hooks.Register(config.Event, config.Handler())
		}
		if plugin.Manifest.Process != nil {
			if !hasPluginPermission(plugin.Manifest.Permissions, plugins.PermissionCommands) {
				return fmt.Errorf("plugin %s process requires commands permission", plugin.Manifest.ID)
			}
			process, err := plugins.StartProcess(ctx, plugin, "/private/tmp/aether-plugin-runtime")
			if err != nil {
				return err
			}
			processKey := projectID + ":" + plugin.Manifest.ID
			s.extensionMu.Lock()
			if previous := s.processPlugins[processKey]; previous != nil {
				_ = previous.Close()
			}
			if s.processPlugins == nil {
				s.processPlugins = make(map[string]*plugins.ProcessRuntime)
			}
			s.processPlugins[processKey] = process
			s.extensionMu.Unlock()
			for _, capability := range process.Capabilities() {
				if err := registry.Register(tools.PluginProcessTool{PluginID: plugin.Manifest.ID, Runtime: process, Capability: capability}); err != nil {
					return err
				}
			}
		}
		for _, relative := range plugin.Manifest.MCP {
			if err := s.configurePluginMCP(ctx, projectID, plugin, relative, registry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) configurePluginMCP(ctx context.Context, projectID string, plugin plugins.Installed, relative string, registry *tools.Registry) error {
	data, err := os.ReadFile(filepath.Join(plugin.Path, relative))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var config mcp.Config
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	config.Name = plugin.Manifest.ID + "." + config.Name
	if config.Transport == mcp.TransportSTDIO {
		if !hasPluginPermission(plugin.Manifest.Permissions, plugins.PermissionCommands) || filepath.IsAbs(config.Command) {
			return errors.New("plugin STDIO MCP requires commands permission and a package-relative executable")
		}
		command, err := filepath.Abs(filepath.Join(plugin.Path, config.Command))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(plugin.Path, command)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("plugin MCP command escapes package")
		}
		info, err := os.Lstat(command)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
			return errors.New("plugin MCP command must be a non-symlink executable")
		}
		config.Command = command
		config.Directory = plugin.Path
		config.SandboxRoot = plugin.Path
		config.NetworkAllowed = hasPluginPermission(plugin.Manifest.Permissions, plugins.PermissionNetwork)
	}
	if config.Transport == mcp.TransportHTTP && !hasPluginPermission(plugin.Manifest.Permissions, plugins.PermissionNetwork) {
		return errors.New("plugin HTTP MCP requires network permission")
	}
	client, err := mcp.New(config)
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		_ = client.Close()
		if config.Required {
			return err
		}
		return nil
	}
	remoteTools, err := client.Tools(connectCtx)
	if err != nil {
		_ = client.Close()
		return err
	}
	key := projectID + ":" + config.Name
	s.extensionMu.Lock()
	if s.mcpClients == nil {
		s.mcpClients = make(map[string]*mcp.Client)
	}
	if previous := s.mcpClients[key]; previous != nil {
		_ = previous.Close()
	}
	s.mcpClients[key] = client
	s.extensionMu.Unlock()
	for _, remote := range remoteTools {
		if err := registry.Register(tools.MCPProxyTool{Client: client, Remote: remote}); err != nil {
			return err
		}
	}
	if instructions := strings.TrimSpace(client.Instructions()); instructions != "" {
		s.Runtime.AddProjectInstruction(projectID, "MCP server instructions for "+config.Name+":\n\n"+instructions)
	}
	return nil
}

func (s *Server) CloseExtensions() {
	if s.State != nil && s.Runtime != nil {
		if projects, err := s.State.ListProjects(context.Background()); err == nil {
			for _, project := range projects {
				if hooks := s.Runtime.ProjectHooks(project.ID); hooks != nil {
					_, _ = hooks.Dispatch(context.Background(), plugins.HookPayload{Event: plugins.HookSessionEnd, ThreadID: project.ID})
				}
			}
		}
	}
	s.extensionMu.Lock()
	defer s.extensionMu.Unlock()
	for _, client := range s.mcpClients {
		_ = client.Close()
	}
	for _, process := range s.processPlugins {
		_ = process.Close()
	}
	s.mcpClients = nil
	s.processPlugins = nil
}

func (s *Server) reloadProjectExtensions(ctx context.Context) error {
	if s.State == nil {
		return nil
	}
	projects, err := s.State.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if _, err := s.ConfigureProject(ctx, project, false); err != nil {
			return err
		}
	}
	return nil
}

func hasPluginPermission(values []plugins.Permission, expected plugins.Permission) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	encoder := json.NewEncoder(connection)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(Response{APIVersion: protocol.APIVersion, Error: "invalid request"})
			continue
		}
		response := s.dispatch(ctx, request)
		response.Proof = responseProof(s.Token, response)
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
	case "attachment.put":
		if s.State == nil {
			response.Error = "artifact store is unavailable"
			break
		}
		var params struct {
			Name       string `json:"name"`
			MimeType   string `json:"mime_type"`
			DataBase64 string `json:"data_base64"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || !strings.HasPrefix(params.MimeType, "image/") {
			response.Error = "valid image attachment is required"
			break
		}
		data, err := base64.StdEncoding.DecodeString(params.DataBase64)
		if err != nil || len(data) == 0 || len(data) > 10*1024*1024 {
			response.Error = "image attachment must be between 1 byte and 10 MiB"
			break
		}
		digest, _, err := s.State.PutArtifact(bytes.NewReader(data))
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.Result = protocol.AttachmentRef{SHA256: digest, Name: filepath.Base(params.Name), MimeType: params.MimeType, Bytes: int64(len(data))}
	case "terminal.start":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		var params struct {
			Workspace string `json:"workspace"`
			Columns   uint16 `json:"columns"`
			Rows      uint16 `json:"rows"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid terminal params"
			break
		}
		info, err := s.Terminals.Start(params.Workspace, params.Columns, params.Rows)
		response.Result, response.Error = resultOrError(info, err)
	case "terminal.list":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		response.Result = s.Terminals.List()
	case "terminal.read":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		var params struct {
			ID    string `json:"id"`
			After int64  `json:"after"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid terminal params"
			break
		}
		snapshot, err := s.Terminals.Read(params.ID, params.After, params.Limit)
		response.Result, response.Error = resultOrError(snapshot, err)
	case "terminal.write":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		var params struct {
			ID         string `json:"id"`
			DataBase64 string `json:"data_base64"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid terminal params"
			break
		}
		data, err := base64.StdEncoding.DecodeString(params.DataBase64)
		if err == nil {
			err = s.Terminals.Write(params.ID, data)
		}
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"written": true}
		}
	case "terminal.resize":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		var params struct {
			ID      string `json:"id"`
			Columns uint16 `json:"columns"`
			Rows    uint16 `json:"rows"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid terminal params"
			break
		}
		if err := s.Terminals.Resize(params.ID, params.Columns, params.Rows); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"resized": true}
		}
	case "terminal.stop":
		if s.Terminals == nil {
			response.Error = "terminal runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid terminal params"
			break
		}
		if err := s.Terminals.Stop(params.ID); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"stopped": true}
		}
	case "automation.save":
		if s.Automations == nil {
			response.Error = "automation scheduler is unavailable"
			break
		}
		var spec protocol.AutomationSpec
		if err := json.Unmarshal(request.Params, &spec); err != nil || spec.APIVersion != protocol.APIVersion {
			response.Error = "invalid automation spec"
			break
		}
		if err := s.Automations.Save(ctx, spec); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = spec
		}
	case "automation.list":
		if s.Automations == nil {
			response.Error = "automation scheduler is unavailable"
			break
		}
		values, err := s.Automations.List(ctx)
		response.Result, response.Error = resultOrError(values, err)
	case "automation.run":
		if s.Automations == nil {
			response.Error = "automation scheduler is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid automation params"
			break
		}
		jobID, err := s.Automations.RunNow(ctx, params.ID)
		response.Result, response.Error = resultOrError(map[string]string{"job_id": jobID}, err)
	case "automation.cancel":
		if s.Automations == nil {
			response.Error = "automation scheduler is unavailable"
			break
		}
		var params struct {
			JobID string `json:"job_id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = map[string]bool{"cancelled": s.Automations.Cancel(params.JobID)}
	case "plugin.list":
		if s.PluginRegistry == nil {
			response.Error = "plugin registry is unavailable"
			break
		}
		values, err := s.PluginRegistry.List()
		response.Result, response.Error = resultOrError(values, err)
	case "plugin.install":
		if s.PluginRegistry == nil {
			response.Error = "plugin registry is unavailable"
			break
		}
		var params struct {
			Source             string `json:"source"`
			ApprovePermissions bool   `json:"approve_permissions"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || !filepath.IsAbs(params.Source) {
			response.Error = "absolute plugin source is required"
			break
		}
		installed, err := s.PluginRegistry.Install(params.Source, params.ApprovePermissions)
		if err != nil {
			response.Error = err.Error()
			break
		}
		if err := s.reloadProjectExtensions(ctx); err != nil {
			_ = s.PluginRegistry.Revoke(installed.Manifest.ID, "activation failed: "+err.Error())
			response.Error = err.Error()
			break
		}
		response.Result = installed
	case "plugin.revoke":
		if s.PluginRegistry == nil {
			response.Error = "plugin registry is unavailable"
			break
		}
		var params struct{ ID, Reason string }
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid plugin params"
			break
		}
		if err := s.PluginRegistry.Revoke(params.ID, params.Reason); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"revoked": true}
		}
	case "memory.status":
		if s.Memory == nil {
			response.Error = "memory repository is unavailable"
			break
		}
		var params struct {
			ProjectID string `json:"project_id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		global, globalErr := s.Memory.Enabled(ctx, aethermemory.ScopeGlobal, "")
		project, projectErr := s.Memory.Enabled(ctx, aethermemory.ScopeProject, params.ProjectID)
		if globalErr != nil || projectErr != nil {
			response.Error = errors.Join(globalErr, projectErr).Error()
		} else {
			response.Result = map[string]bool{"global": global, "project": project}
		}
	case "memory.enable":
		if s.Memory == nil {
			response.Error = "memory repository is unavailable"
			break
		}
		var params struct {
			Scope     aethermemory.Scope `json:"scope"`
			ProjectID string             `json:"project_id"`
			Enabled   bool               `json:"enabled"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid memory params"
			break
		}
		if err := s.Memory.SetEnabled(ctx, params.Scope, params.ProjectID, params.Enabled); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"updated": true}
		}
	case "memory.list":
		if s.Memory == nil {
			response.Error = "memory repository is unavailable"
			break
		}
		var params struct {
			Scope     aethermemory.Scope `json:"scope"`
			ProjectID string             `json:"project_id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		values, err := s.Memory.Search(ctx, params.Scope, params.ProjectID, "", 100)
		response.Result, response.Error = resultOrError(values, err)
	case "memory.save":
		if s.Memory == nil {
			response.Error = "memory repository is unavailable"
			break
		}
		var value aethermemory.Memory
		if err := json.Unmarshal(request.Params, &value); err != nil {
			response.Error = "invalid memory"
			break
		}
		result, err := s.Memory.Save(ctx, value)
		response.Result, response.Error = resultOrError(result, err)
	case "memory.delete":
		if s.Memory == nil {
			response.Error = "memory repository is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		if err := s.Memory.Delete(ctx, params.ID); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"deleted": true}
		}
	case "browser.start":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		if err := s.Browser.Start(ctx); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = s.Browser.Status()
		}
	case "browser.stop":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		s.Browser.Stop()
		response.Result = map[string]bool{"stopped": true}
	case "browser.status":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		response.Result = s.Browser.Status()
	case "browser.site_permission":
		if s.Browser == nil || s.Browser.Permissions == nil {
			response.Error = "browser permission store is unavailable"
			break
		}
		var params struct {
			URL      string `json:"url"`
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid browser permission params"
			break
		}
		parsed, err := url.Parse(params.URL)
		if err != nil || parsed.Hostname() == "" {
			response.Error = "valid browser URL is required"
			break
		}
		host := strings.ToLower(parsed.Hostname())
		if params.Decision == "revoke" {
			err = s.Browser.Permissions.Revoke(ctx, "site", host)
		} else {
			decision := browser.Decision(params.Decision)
			err = s.Browser.Permissions.Set(ctx, "site", host, decision, "persistent")
		}
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"updated": true}
		}
	case "browser.navigate":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		var params struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid browser params"
			break
		}
		if err := s.Browser.Navigate(ctx, params.URL); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = s.Browser.Status()
		}
	case "browser.dom":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		value, err := s.Browser.DOM(ctx)
		response.Result, response.Error = resultOrError(value, err)
	case "browser.screenshot":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		value, err := s.Browser.Screenshot(ctx)
		response.Result, response.Error = resultOrError(value, err)
	case "browser.console":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		var params struct {
			After int `json:"after"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = s.Browser.Console(params.After)
	case "browser.network":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		var params struct {
			After int `json:"after"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = s.Browser.Network(params.After)
	case "browser.click":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		var params struct {
			Selector  string `json:"selector"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid browser params"
			break
		}
		if sensitiveBrowserAction(params.Selector) && !params.Confirmed {
			response.Error = "sensitive browser action requires explicit confirmation"
			break
		}
		if err := s.Browser.Click(ctx, params.Selector); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"clicked": true}
		}
	case "browser.type":
		if s.Browser == nil {
			response.Error = "controlled browser is unavailable"
			break
		}
		var params struct{ Selector, Text string }
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid browser params"
			break
		}
		if err := s.Browser.Type(ctx, params.Selector, params.Text); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"typed": true}
		}
	case "computer.status":
		if s.Computer == nil {
			response.Error = "Computer Use is unavailable"
			break
		}
		var params struct {
			Prompt bool `json:"prompt"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = s.Computer.Status(params.Prompt)
	case "computer.decision":
		if s.Computer == nil {
			response.Error = "Computer Use is unavailable"
			break
		}
		var params struct {
			BundleID string `json:"bundle_id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = map[string]any{"bundle_id": params.BundleID, "decision": s.Computer.ApplicationDecision(ctx, params.BundleID)}
	case "computer.permission":
		if s.Computer == nil {
			response.Error = "Computer Use is unavailable"
			break
		}
		var params struct {
			BundleID string `json:"bundle_id"`
			Decision string `json:"decision"`
			Scope    string `json:"scope"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid Computer Use params"
			break
		}
		if err := s.Computer.SetApplicationPermission(ctx, params.BundleID, computeruse.Decision(params.Decision), params.Scope); err != nil {
			response.Error = err.Error()
		} else {
			response.Result = map[string]bool{"updated": true}
		}
	case "browser.chrome_ingest":
		if s.ChromeSessions == nil {
			response.Error = "Chrome native messaging bridge is unavailable"
			break
		}
		var message browser.NativeMessage
		if err := json.Unmarshal(request.Params, &message); err != nil {
			response.Error = "invalid Chrome snapshot"
			break
		}
		snapshot, err := s.ChromeSessions.Ingest(message)
		response.Result, response.Error = resultOrError(snapshot, err)
	case "browser.chrome_latest":
		if s.ChromeSessions == nil {
			response.Error = "Chrome native messaging bridge is unavailable"
			break
		}
		snapshot, ok := s.ChromeSessions.Latest()
		response.Result = map[string]any{"available": ok, "snapshot": snapshot}
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
		definitions, err := s.ConfigureProject(ctx, store.ProjectRecord{ID: params.ProjectID, Root: params.Root, Permission: params.Permission}, true)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.Result = map[string]any{"project_id": params.ProjectID, "root": params.Root, "tools": definitions}
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
			Thread      protocol.Thread          `json:"thread"`
			Input       string                   `json:"input"`
			Attachments []protocol.AttachmentRef `json:"attachments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		turn, err := s.Runtime.RunTurnWithAttachments(ctx, params.Thread, params.Input, params.Attachments)
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
	case "ael.start":
		if s.AEL == nil {
			response.Error = "AEL runtime is unavailable"
			break
		}
		var params ael.RunRequest
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		record, err := s.AEL.Start(ctx, params)
		response.Result, response.Error = resultOrError(record, err)
	case "ael.get":
		if s.AEL == nil {
			response.Error = "AEL runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		record, err := s.AEL.Get(ctx, params.ID)
		response.Result, response.Error = resultOrError(record, err)
	case "ael.cancel":
		if s.AEL == nil {
			response.Error = "AEL runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		response.Result = map[string]bool{"cancelled": s.AEL.Cancel(params.ID)}
	case "ael.replay":
		if s.AEL == nil {
			response.Error = "AEL runtime is unavailable"
			break
		}
		var params struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Params, &params)
		record, err := s.AEL.Replay(ctx, params.ID)
		response.Result, response.Error = resultOrError(record, err)
	case "ael.compare":
		if s.AEL == nil {
			response.Error = "AEL runtime is unavailable"
			break
		}
		var params struct{ Left, Right string }
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		comparison, err := s.AEL.Compare(ctx, params.Left, params.Right)
		response.Result, response.Error = resultOrError(comparison, err)
	case "model.generate":
		if s.Models == nil {
			response.Error = "model runtime is unavailable"
			break
		}
		var params struct {
			ProjectID string                     `json:"project_id"`
			Request   modeling.GenerationRequest `json:"request"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		packageValue, err := s.Models.Generate(ctx, params.ProjectID, params.Request)
		response.Result, response.Error = resultOrError(packageValue, err)
	case "model.generate_grounded":
		if s.Models == nil {
			response.Error = "model runtime is unavailable"
			break
		}
		var params struct {
			ProjectID string                   `json:"project_id"`
			Request   modeling.GroundedRequest `json:"request"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		packageValue, err := s.Models.GenerateGrounded(ctx, params.ProjectID, params.Request)
		response.Result, response.Error = resultOrError(packageValue, err)
	case "model.promote":
		if s.Models == nil {
			response.Error = "model runtime is unavailable"
			break
		}
		var params struct {
			ProjectID string                        `json:"project_id"`
			ID        string                        `json:"id"`
			Version   string                        `json:"version"`
			Target    modeling.ModelState           `json:"target"`
			Actor     string                        `json:"actor"`
			Evidence  *modeling.ConformanceEvidence `json:"evidence"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = "invalid params"
			break
		}
		packageValue, err := s.Models.Promote(params.ProjectID, params.ID, params.Version, params.Target, params.Actor, params.Evidence)
		response.Result, response.Error = resultOrError(packageValue, err)
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
	proof := response.Proof
	response.Proof = ""
	if proof == "" || !hmac.Equal([]byte(proof), []byte(responseProof(c.Token, response))) {
		return errors.New("daemon response authentication failed")
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

func responseProof(token string, response Response) string {
	response.Proof = ""
	payload, _ := json.Marshal(response)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func sensitiveBrowserAction(selector string) bool {
	value := strings.ToLower(selector)
	for _, marker := range []string{"submit", "purchase", "buy", "delete", "remove", "upload", "付款", "购买", "删除", "提交", "上传"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
