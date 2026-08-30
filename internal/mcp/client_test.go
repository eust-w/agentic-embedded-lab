package mcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (fn transport) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestHTTPToolsAreNamespacedAndToolCallsStripNamespace(t *testing.T) {
	client, err := New(Config{Name: "lab", Transport: TransportHTTP, URL: "https://mcp.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: transport(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), `"method":"initialize"`) {
			return jsonResponse(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}`), nil
		}
		if strings.Contains(string(body), `"method":"notifications/initialized"`) {
			return &http.Response{StatusCode: http.StatusAccepted, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if strings.Contains(string(body), `"method":"tools/list"`) {
			return jsonResponse(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"run","description":"Run","inputSchema":{"type":"object"}}]}}`), nil
		}
		if !strings.Contains(string(body), `"name":"run"`) {
			t.Fatalf("tool namespace was not stripped: %s", body)
		}
		return jsonResponse(`{"jsonrpc":"2.0","id":3,"result":{"ok":true}}`), nil
	})}
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "lab.run" {
		t.Fatalf("unexpected tools: %#v %v", tools, err)
	}
	result, err := client.CallTool(context.Background(), "lab.run", map[string]any{})
	if err != nil || result["ok"] != true {
		t.Fatalf("unexpected result: %#v %v", result, err)
	}
}

func TestRejectsInsecureRemoteHTTP(t *testing.T) {
	if _, err := New(Config{Name: "bad", Transport: TransportHTTP, URL: "http://example.com"}); err == nil {
		t.Fatal("expected insecure remote URL to fail")
	}
}

func TestRejectsInlineSecretEnvironmentAndBuildsNetworkClosedSandbox(t *testing.T) {
	if _, err := New(Config{Name: "secret-env", Transport: TransportSTDIO, Command: "tool", Env: map[string]string{"API_KEY": "plaintext"}}); err == nil {
		t.Fatal("inline secret environment was accepted")
	}
	profile := mcpSeatbeltProfile(t.TempDir(), false)
	if strings.Contains(profile, "(allow network-outbound)") {
		t.Fatalf("network-closed MCP sandbox is too broad: %s", profile)
	}
}

func TestRejectsInlineBearerAndDecodesStreamableHTTPSSE(t *testing.T) {
	if _, err := New(Config{Name: "secret", Transport: TransportHTTP, URL: "https://mcp.example.test", Bearer: "plaintext"}); err == nil {
		t.Fatal("inline bearer was accepted")
	}
	client, err := New(Config{Name: "stream", Transport: TransportHTTP, URL: "https://mcp.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: transport(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), `"method":"initialize"`) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}, "Mcp-Session-Id": {"session-1"}}, Body: io.NopCloser(strings.NewReader("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-11-25\",\"instructions\":\"Use evidence\"}}\n\n"))}, nil
		}
		if strings.Contains(string(body), "notifications/initialized") {
			if request.Header.Get("Mcp-Session-Id") != "session-1" {
				t.Fatal("MCP session id was not continued")
			}
			return &http.Response{StatusCode: 202, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return jsonResponse(`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`), nil
	})}
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.Instructions() != "Use evidence" {
		t.Fatalf("instructions missing: %q", client.Instructions())
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
