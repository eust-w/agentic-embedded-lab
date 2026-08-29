package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type DevOIDC struct {
	Issuer, Audience, Kid string
	key                   *rsa.PrivateKey
}

func NewDevOIDC(issuer, audience string) (*DevOIDC, error) {
	if issuer == "" || audience == "" {
		return nil, errors.New("development OIDC issuer and audience are required")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(key.PublicKey.N.Bytes())
	return &DevOIDC{Issuer: strings.TrimRight(issuer, "/"), Audience: audience, Kid: hex.EncodeToString(digest[:8]), key: key}, nil
}
func (d *DevOIDC) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"issuer": d.Issuer, "jwks_uri": d.Issuer + "/.well-known/jwks.json", "token_endpoint": d.Issuer + "/token", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": d.Kid, "n": base64.RawURLEncoding.EncodeToString(d.key.PublicKey.N.Bytes()), "e": encodeExponent(d.key.PublicKey.E)}}})
	})
	mux.HandleFunc("POST /token", d.token)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ready"}) })
	return mux
}
func (d *DevOIDC) token(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Subject string `json:"subject"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	if request.Subject == "" {
		request.Subject = "aether-developer"
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{"iss": d.Issuer, "aud": d.Audience, "sub": request.Subject, "iat": now.Unix(), "exp": now.Add(15 * time.Minute).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = d.Kid
	signed, err := token.SignedString(d.key)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"access_token": signed, "token_type": "Bearer", "expires_in": 900})
}
func encodeExponent(value int) string {
	bytes := big.NewInt(int64(value)).Bytes()
	return base64.RawURLEncoding.EncodeToString(bytes)
}
