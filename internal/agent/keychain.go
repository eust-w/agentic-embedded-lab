package agent

import (
	"context"
	"strings"

	"github.com/eust-w/agentic-embedded-lab/internal/secret"
)

const (
	KeychainService = "dev.aether.desktop"
	OpenAIAccount   = "openai-api-key"
)

type KeychainAPIKey struct{ Store secret.Store }

func (k KeychainAPIKey) APIKey(context.Context) (string, error) {
	value, err := k.Store.Get(KeychainService, OpenAIAccount)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}
