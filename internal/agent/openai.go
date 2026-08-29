package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

const DefaultModel = "gpt-5.6"

type APIKeyProvider interface {
	APIKey(context.Context) (string, error)
}

type StaticAPIKey string

func (s StaticAPIKey) APIKey(context.Context) (string, error) {
	if strings.TrimSpace(string(s)) == "" {
		return "", errors.New("OpenAI API key is not configured")
	}
	return string(s), nil
}

type ToolDefinition struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ImageInput struct {
	MimeType string `json:"mime_type"`
	DataURL  string `json:"data_url"`
}

type ResponseRequest struct {
	Model      string            `json:"model"`
	Input      []map[string]any  `json:"input"`
	Tools      []ToolDefinition  `json:"tools,omitempty"`
	Stream     bool              `json:"stream"`
	Store      bool              `json:"store"`
	PreviousID string            `json:"previous_response_id,omitempty"`
	Reasoning  map[string]any    `json:"reasoning,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ResponseEvent struct {
	Type        string         `json:"type"`
	Delta       string         `json:"delta,omitempty"`
	ItemID      string         `json:"item_id,omitempty"`
	OutputIndex int            `json:"output_index,omitempty"`
	Payload     map[string]any `json:"-"`
}

type APIError struct {
	StatusCode int
	Category   string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("OpenAI %s error (%d): %s", e.Category, e.StatusCode, e.Body)
}

type ResponsesClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Keys       APIKeyProvider
	MaxRetries int
}

func NewResponsesClient(keys APIKeyProvider) *ResponsesClient {
	return &ResponsesClient{
		BaseURL:    "https://api.openai.com/v1",
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
		Keys:       keys,
		MaxRetries: 3,
	}
}

func (c *ResponsesClient) Stream(
	ctx context.Context,
	request ResponseRequest,
	idempotencyKey string,
	emit func(ResponseEvent) error,
) error {
	if request.Model == "" {
		request.Model = DefaultModel
	}
	request.Stream = true
	request.Store = false
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode response request: %w", err)
	}
	key, err := c.Keys.APIKey(ctx)
	if err != nil {
		return err
	}
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		err = c.streamOnce(ctx, payload, key, idempotencyKey, emit)
		if err == nil {
			return nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusTooManyRequests && apiErr.StatusCode < 500) {
			return err
		}
		if attempt == c.MaxRetries {
			return err
		}
		delay := time.Duration(250*(1<<attempt)+rand.IntN(150)) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func (c *ResponsesClient) streamOnce(
	ctx context.Context,
	payload []byte,
	key, idempotencyKey string,
	emit func(ResponseEvent) error,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/responses", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("request OpenAI Responses API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 32<<10))
		return &APIError{
			StatusCode: response.StatusCode,
			Category:   classifyStatus(response.StatusCode),
			Body:       strings.TrimSpace(string(body)),
		}
	}
	return parseSSE(response.Body, emit)
}

func parseSSE(reader io.Reader, emit func(ResponseEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			return fmt.Errorf("decode OpenAI event: %w", err)
		}
		event := ResponseEvent{Payload: raw}
		if value, ok := raw["type"].(string); ok {
			event.Type = value
		}
		if value, ok := raw["delta"].(string); ok {
			event.Delta = value
		}
		if value, ok := raw["item_id"].(string); ok {
			event.ItemID = value
		}
		if value, ok := raw["output_index"].(float64); ok {
			event.OutputIndex = int(value)
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func classifyStatus(code int) string {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authentication"
	case http.StatusPaymentRequired:
		return "billing"
	case http.StatusTooManyRequests:
		return "rate_limit"
	default:
		if code >= 500 {
			return "server"
		}
		return "request"
	}
}
