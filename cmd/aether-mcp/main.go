package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	capability "github.com/eust-w/agentic-embedded-lab/internal/acceptance"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	workspace := flag.String("workspace", ".", "AEL workspace")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	if err != nil {
		panic(err)
	}
	revision := gitRevision(root)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request request
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		if request.ID == nil {
			continue
		}
		result, err := dispatch(root, revision, request)
		reply := response{JSONRPC: "2.0", ID: request.ID, Result: result}
		if err != nil {
			reply.Result = nil
			reply.Error = &rpcError{Code: -32000, Message: err.Error()}
		}
		_ = encoder.Encode(reply)
	}
}

func dispatch(root, revision string, request request) (any, error) {
	switch request.Method {
	case "initialize":
		return map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "aether-acceptance", "version": "1.0.0-dev"}}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if json.Unmarshal(request.Params, &params) != nil {
			return nil, fmt.Errorf("invalid tool params")
		}
		value, err := call(root, revision, params.Name, params.Arguments)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(value, "", "  ")
		return map[string]any{"content": []map[string]any{{"type": "text", "text": string(data)}}}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP method %s", request.Method)
	}
}
func toolDefinitions() []map[string]any {
	result := []map[string]any{}
	for _, name := range []string{"inspect_capabilities", "get_acceptance_status", "get_acceptance_evidence"} {
		required := []string{}
		if name != "inspect_capabilities" {
			required = []string{"id"}
		}
		result = append(result, map[string]any{"name": name, "description": "Read immutable Aether/AEL acceptance state", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": required}})
	}
	return result
}
func call(root, revision, name string, arguments map[string]any) (any, error) {
	id, _ := arguments["id"].(string)
	switch name {
	case "inspect_capabilities":
		if id == "" {
			return capability.List(root, revision)
		}
		return capability.Inspect(root, revision, id)
	case "get_acceptance_status":
		return capability.Inspect(root, revision, id)
	case "get_acceptance_evidence":
		path, err := capability.EvidencePath(id)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		var value any
		if json.Unmarshal(data, &value) != nil {
			return nil, fmt.Errorf("invalid evidence JSON")
		}
		return map[string]any{"path": path, "evidence": value}, nil
	default:
		return nil, fmt.Errorf("unknown tool %s", name)
	}
}
func gitRevision(root string) string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	data, _ := command.Output()
	return strings.TrimSpace(string(data))
}
