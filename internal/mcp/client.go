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
	"os/exec"
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
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Arguments []string          `json:"arguments,omitempty"`
	Directory string            `json:"directory,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Bearer    string            `json:"bearer,omitempty"`
	Required  bool              `json:"required"`
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
	config Config
	http   *http.Client
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID atomic.Int64
}

func New(config Config) (*Client, error) {
	if config.Name == "" {
		return nil, errors.New("MCP server name is required")
	}
	if config.Transport == TransportSTDIO && config.Command == "" {
		return nil, errors.New("STDIO MCP requires a command")
	}
	if config.Transport == TransportHTTP && !strings.HasPrefix(config.URL, "https://") && !strings.HasPrefix(config.URL, "http://127.0.0.1") && !strings.HasPrefix(config.URL, "http://localhost") {
		return nil, errors.New("HTTP MCP requires HTTPS or a loopback URL")
	}
	return &Client{config: config, http: &http.Client{}}, nil
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
	return nil
}

func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var result struct {
		Tools []Tool `json:"tools"`
	}
	err := c.call(ctx, "tools/list", map[string]any{}, &result)
	for index := range result.Tools {
		result.Tools[index].Name = c.config.Name + "." + result.Tools[index].Name
	}
	return result.Tools, err
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
	command := exec.CommandContext(ctx, executable, c.config.Arguments...)
	command.Dir = c.config.Directory
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
	if c.config.Bearer != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.config.Bearer)
	}
	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 16<<10))
		return fmt.Errorf("MCP HTTP %d: %s", httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(httpResponse.Body).Decode(response)
}
