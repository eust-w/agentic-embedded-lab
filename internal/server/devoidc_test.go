package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testRoundTrip func(*http.Request) (*http.Response, error)

func (fn testRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestDevOIDCTokensValidateAgainstJWKS(t *testing.T) {
	provider, err := NewDevOIDC("https://placeholder.invalid", "ael-dev")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://issuer.invalid/token", strings.NewReader(`{"subject":"developer"}`))
	response := httptest.NewRecorder()
	provider.token(response, request)
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	jwks, _ := json.Marshal(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": provider.Kid, "n": base64.RawURLEncoding.EncodeToString(provider.key.PublicKey.N.Bytes()), "e": encodeExponent(provider.key.PublicKey.E)}}})
	validator := OIDCValidator{Issuer: provider.Issuer, Audience: "ael-dev", JWKSURL: provider.Issuer + "/.well-known/jwks.json", HTTP: &http.Client{Timeout: time.Second, Transport: testRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(jwks))}, nil
	})}}
	claims, err := validator.Validate(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "developer" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestWorkerMiddlewareRejectsUntrustedFingerprint(t *testing.T) {
	control := &ControlPlane{config: Config{WorkerFingerprints: map[string]bool{"trusted": true}}}
	handler := control.worker(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("untrusted worker got %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("X-Client-Cert-SHA256", "trusted")
	request.Header.Set("X-Aether-Ingress", "user")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("user ingress spoofed a worker certificate: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("X-Client-Cert-SHA256", "trusted")
	request.Header.Set("X-Aether-Ingress", "worker")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("trusted worker got %d", response.Code)
	}
}
