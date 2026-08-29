package mcp

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type secretMemory struct{ values map[string][]byte }

func (s *secretMemory) Get(service, account string) ([]byte, error) {
	return s.values[service+":"+account], nil
}
func (s *secretMemory) Set(service, account string, value []byte) error {
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[service+":"+account] = append([]byte{}, value...)
	return nil
}
func TestOAuthPKCEStateAndSecretBackedRefresh(t *testing.T) {
	store := &secretMemory{}
	provider := &OAuthProvider{Config: OAuthConfig{AuthorizationURL: "https://auth.example/authorize", TokenURL: "https://auth.example/token", ClientID: "client", Scopes: []string{"tools"}}, Store: store, HTTP: &http.Client{Transport: transport(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		values, _ := url.ParseQuery(string(body))
		if values.Get("code_verifier") == "" {
			t.Fatal("PKCE verifier missing")
		}
		return jsonResponse(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`), nil
	})}}
	authorization, err := provider.Authorization("http://127.0.0.1/callback")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorization)
	state := parsed.Query().Get("state")
	if state == "" || parsed.Query().Get("code_challenge") == "" {
		t.Fatal("PKCE authorization parameters missing")
	}
	if err := provider.Exchange(context.Background(), "code", state, "http://127.0.0.1/callback"); err != nil {
		t.Fatal(err)
	}
	token, err := provider.Token(context.Background())
	if err != nil || token != "access" {
		t.Fatalf("unexpected token %q %v", token, err)
	}
	if !strings.Contains(string(store.values[oauthKeychainService+":client"]), "refresh") {
		t.Fatal("refresh token was not stored in secret store")
	}
}
