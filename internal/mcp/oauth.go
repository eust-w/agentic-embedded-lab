package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type OAuthSecretStore interface {
	Get(service, account string) ([]byte, error)
	Set(service, account string, value []byte) error
}
type OAuthConfig struct {
	AuthorizationURL string
	TokenURL         string
	ClientID         string
	Scopes           []string
	Account          string
}
type OAuthProvider struct {
	Config          OAuthConfig
	Store           OAuthSecretStore
	HTTP            *http.Client
	mu              sync.Mutex
	access          string
	expires         time.Time
	pendingVerifier string
	pendingState    string
}
type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

const oauthKeychainService = "dev.aether.desktop.mcp.oauth"

func (p *OAuthProvider) Authorization(redirectURI string) (string, error) {
	if p.Config.AuthorizationURL == "" || p.Config.TokenURL == "" || p.Config.ClientID == "" || p.Store == nil {
		return "", errors.New("OAuth provider is incomplete")
	}
	verifier, err := randomURLSafe(48)
	if err != nil {
		return "", err
	}
	state, err := randomURLSafe(24)
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	p.mu.Lock()
	p.pendingVerifier, p.pendingState = verifier, state
	p.mu.Unlock()
	parsed, err := url.Parse(p.Config.AuthorizationURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", p.Config.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("state", state)
	if len(p.Config.Scopes) > 0 {
		query.Set("scope", strings.Join(p.Config.Scopes, " "))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
func (p *OAuthProvider) Exchange(ctx context.Context, code, state, redirectURI string) error {
	p.mu.Lock()
	verifier, expected := p.pendingVerifier, p.pendingState
	p.pendingVerifier, p.pendingState = "", ""
	p.mu.Unlock()
	if verifier == "" || state == "" || state != expected {
		return errors.New("OAuth state verification failed")
	}
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {p.Config.ClientID}, "redirect_uri": {redirectURI}, "code_verifier": {verifier}}
	token, err := p.exchange(ctx, values)
	if err != nil {
		return err
	}
	return p.accept(token)
}
func (p *OAuthProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.access != "" && time.Until(p.expires) > time.Minute {
		token := p.access
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()
	account := p.Config.Account
	if account == "" {
		account = p.Config.ClientID
	}
	refresh, err := p.Store.Get(oauthKeychainService, account)
	if err != nil || len(refresh) == 0 {
		return "", errors.New("MCP OAuth authorization is required")
	}
	token, err := p.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {string(refresh)}, "client_id": {p.Config.ClientID}})
	if err != nil {
		return "", err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = string(refresh)
	}
	if err := p.accept(token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
func (p *OAuthProvider) exchange(ctx context.Context, values url.Values) (oauthToken, error) {
	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Config.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthToken{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return oauthToken{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return oauthToken{}, errors.New("MCP OAuth token endpoint rejected the request")
	}
	var token oauthToken
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&token); err != nil {
		return oauthToken{}, err
	}
	if token.AccessToken == "" || token.TokenType != "Bearer" {
		return oauthToken{}, errors.New("MCP OAuth token response is invalid")
	}
	return token, nil
}
func (p *OAuthProvider) accept(token oauthToken) error {
	account := p.Config.Account
	if account == "" {
		account = p.Config.ClientID
	}
	if token.RefreshToken != "" {
		if err := p.Store.Set(oauthKeychainService, account, []byte(token.RefreshToken)); err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.access = token.AccessToken
	p.expires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	p.mu.Unlock()
	return nil
}
func randomURLSafe(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
