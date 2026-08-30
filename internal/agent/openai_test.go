package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestResponsesClientStreamsEventsWithoutPersisting(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing authorization header")
		}
		payload, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(payload), `"stream":true`) || !strings.Contains(string(payload), `"store":true`) {
			t.Fatalf("Responses continuation flags missing: %s", payload)
		}
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n" +
			"data: {\"type\":\"response.completed\"}\n" + "data: [DONE]\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	client := NewResponsesClient(StaticAPIKey("test-key"))
	client.HTTPClient = &http.Client{Transport: transport}
	var events []ResponseEvent
	err := client.Stream(context.Background(), ResponseRequest{
		Input: []map[string]any{{"role": "user", "content": "hello"}},
	}, "turn-1", func(event ResponseEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Delta != "hello" || events[1].Type != "response.completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestResponsesClientClassifiesAuthenticationFailure(t *testing.T) {
	client := NewResponsesClient(StaticAPIKey("test-key"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("invalid key")), Header: make(http.Header)}, nil
	})}
	err := client.Stream(context.Background(), ResponseRequest{}, "", func(ResponseEvent) error { return nil })
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Category != "authentication" {
		t.Fatalf("expected authentication error, got %T %v", err, err)
	}
}
