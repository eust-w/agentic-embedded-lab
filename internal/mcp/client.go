package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

const ProtocolVersion = "2025-11-25"

type Transport string

const (
	TransportSTDIO Transport = "stdio"
	TransportHTTP  Transport = "streamable_http"
)

type Config struct {
	Name           string            `json:"name"`
	Transport      Transport         `json:"transport"`
	Command        string            `json:"command,omitempty"`
	Arguments      []string          `json:"arguments,omitempty"`
	Directory      string            `json:"directory,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	URL            string            `json:"url,omitempty"`
	Bearer         string            `json:"bearer,omitempty"`
	BearerEnv      string            `json:"bearer_env,omitempty"`
	Required       bool              `json:"required"`
	SandboxRoot    string            `json:"-"`
	NetworkAllowed bool              `json:"-"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Client struct {
	config       Config
	http         *http.Client
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	reader       *bufio.Reader
	nextID       atomic.Int64
	tokens       TokenProvider
	sessionID    string
	instructions string
}

type TokenProvider interface {
	Token(context.Context) (string, error)
}

func New(config Config) (*Client, error) {
	return NewWithToken(config, nil)
}

func NewWithToken(config Config, tokens TokenProvider) (*Client, error) {
	if config.Name == "" {
		return nil, errors.New("MCP server name is required")
	}
	if config.Transport == TransportSTDIO && config.Command == "" {
		return nil, errors.New("STDIO MCP requires a command")
	}
	if config.Transport == TransportHTTP && !strings.HasPrefix(config.URL, "https://") && !strings.HasPrefix(config.URL, "http://127.0.0.1") && !strings.HasPrefix(config.URL, "http://localhost") {
		return nil, errors.New("HTTP MCP requires HTTPS or a loopback URL")
	}
	if config.Bearer != "" {
		return nil, errors.New("inline MCP bearer tokens are forbidden; use a Secret-backed TokenProvider")
	}
	for key := range config.Env {
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "API_KEY") {
			return nil, errors.New("MCP secrets must use a Secret-backed provider, not inline environment values")
		}
	}
	if tokens == nil && config.BearerEnv != "" {
		tokens = EnvironmentToken(config.BearerEnv)
	}
	return &Client{config: config, http: &http.Client{}, tokens: tokens}, nil
}

type EnvironmentToken string

func (name EnvironmentToken) Token(context.Context) (string, error) {
	value := strings.TrimSpace(os.Getenv(string(name)))
	if value == "" {
		return "", errors.New("MCP bearer environment secret is unavailable")
	}
	return value, nil
}

func (c *Client) Connect(ctx context.Context) error {
	if c.config.Transport == TransportSTDIO {
		if err := c.startSTDIO(ctx); err != nil {
			return err
		}
	}
	var result map[string]any
	if err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "Aether Desktop", "version": "1.0.0-dev"},
	}, &result); err != nil {
		c.Close()
		return fmt.Errorf("initialize MCP %s: %w", c.config.Name, err)
	}
	if instructions, ok := result["instructions"].(string); ok {
		c.instructions = instructions
	}
	if err := c.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		c.Close()
		return err
	}
	return nil
}

func (c *Client) Instructions() string { return c.instructions }

func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var tools []Tool
	cursor := ""
	for {
		var result struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		for index := range result.Tools {
			result.Tools[index].Name = c.config.Name + "." + result.Tools[index].Name
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		cursor = result.NextCursor
		if len(tools) > 10000 {
			return nil, errors.New("MCP tools pagination exceeded safety limit")
		}
	}
}

func (c *Client) CallTool(ctx context.Context, namespacedName string, arguments map[string]any) (map[string]any, error) {
	prefix := c.config.Name + "."
	if !strings.HasPrefix(namespacedName, prefix) {
		return nil, errors.New("tool is outside MCP namespace")
	}
	var result map[string]any
	err := c.call(ctx, "tools/call", map[string]any{"name": strings.TrimPrefix(namespacedName, prefix), "arguments": arguments}, &result)
	return result, err
}

func (c *Client) Resources(ctx context.Context) ([]Resource, error) {
	var result struct {
		Resources []Resource `json:"resources"`
	}
	err := c.call(ctx, "resources/list", map[string]any{}, &result)
	return result.Resources, err
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	c.stdin, c.reader, c.cmd = nil, nil, nil
	return nil
}

func (c *Client) startSTDIO(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		return nil
	}
	executable, err := exec.LookPath(c.config.Command)
	if err != nil {
		return err
	}
	commandName := executable
	arguments := append([]string(nil), c.config.Arguments...)
	if runtime.GOOS == "darwin" && c.config.SandboxRoot != "" {
		arguments = append([]string{"-p", mcpSeatbeltProfile(c.config.SandboxRoot, c.config.NetworkAllowed), "--", executable}, arguments...)
		commandName = "/usr/bin/sandbox-exec"
	}
	command := exec.CommandContext(ctx, commandName, arguments...)
	command.Dir = c.config.Directory
	command.Env = mcpEnvironment(c.config.Env)
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	c.cmd, c.stdin, c.reader = command, stdin, bufio.NewReader(stdout)
	return nil
}

func mcpEnvironment(values map[string]string) []string {
	result := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin", "HOME=" + os.Getenv("HOME"), "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TMPDIR=" + os.TempDir()}
	for key, value := range values {
		if key == "PATH" || key == "HOME" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			continue
		}
		result = append(result, key+"="+value)
	}
	return result
}

func mcpSeatbeltProfile(root string, network bool) string {
	root, _ = filepath.Abs(root)
	root = strings.ReplaceAll(root, "\"", "\\\"")
	lines := []string{"(version 1)", "(deny default)", "(allow process*)", "(allow sysctl-read)", "(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/bin\") (subpath \"/opt/homebrew\"))", fmt.Sprintf("(allow file-read* (subpath \"%s\"))", root), "(allow file-write* (literal \"/dev/null\"))"}
	if network {
		lines = append(lines, "(allow network-outbound)")
	}
	return strings.Join(lines, "\n")
}

func (c *Client) call(ctx context.Context, method string, params any, target any) error {
	id := c.nextID.Add(1)
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	var response rpcResponse
	var err error
	switch c.config.Transport {
	case TransportSTDIO:
		err = c.callSTDIO(ctx, request, &response)
	case TransportHTTP:
		err = c.callHTTP(ctx, request, &response)
	default:
		return errors.New("unsupported MCP transport")
	}
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}
	if target == nil || len(response.Result) == 0 {
		return nil
	}
	return json.Unmarshal(response.Result, target)
}

func (c *Client) callSTDIO(ctx context.Context, request rpcRequest, response *rpcResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil || c.reader == nil {
		return errors.New("MCP STDIO server is not connected")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(payload, '\n')); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		line, err := c.reader.ReadBytes('\n')
		if err == nil {
			err = json.Unmarshal(line, response)
		}
		result <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		return err
	}
}

func (c *Client) callHTTP(ctx context.Context, request rpcRequest, response *rpcResponse) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json, text/event-stream")
	if c.tokens != nil {
		token, err := c.tokens.Token(ctx)
		if err != nil {
			return err
		}
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	if c.sessionID != "" {
		httpRequest.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()
	if session := httpResponse.Header.Get("Mcp-Session-Id"); session != "" {
		c.sessionID = session
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 16<<10))
		return fmt.Errorf("MCP HTTP %d: %s", httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.Contains(httpResponse.Header.Get("Content-Type"), "text/event-stream") {
		return decodeHTTPEventStream(httpResponse.Body, response)
	}
	return json.NewDecoder(httpResponse.Body).Decode(response)
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	switch c.config.Transport {
	case TransportSTDIO:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.stdin == nil {
			return errors.New("MCP STDIO server is not connected")
		}
		_, err = c.stdin.Write(append(payload, '\n'))
		return err
	case TransportHTTP:
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if c.tokens != nil {
			token, err := c.tokens.Token(ctx)
			if err != nil {
				return err
			}
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if c.sessionID != "" {
			request.Header.Set("Mcp-Session-Id", c.sessionID)
		}
		response, err := c.http.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if session := response.Header.Get("Mcp-Session-Id"); session != "" {
			c.sessionID = session
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("MCP notification HTTP %d", response.StatusCode)
		}
		return nil
	default:
		return errors.New("unsupported MCP transport")
	}
}

func decodeHTTPEventStream(reader io.Reader, response *rpcResponse) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if err := json.Unmarshal([]byte(data), response); err != nil {
			return err
		}
		if response.ID != 0 {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("MCP event stream ended without a response")
}
