package modeling

import (
	"context"
	"errors"
	"sync"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
)

type Manager struct {
	client   *agent.ResponsesClient
	mu       sync.RWMutex
	projects map[string]string
}

func NewManager(client *agent.ResponsesClient) *Manager {
	return &Manager{client: client, projects: make(map[string]string)}
}

func (m *Manager) RegisterProject(projectID, root string) {
	m.mu.Lock()
	m.projects[projectID] = root
	m.mu.Unlock()
}

func (m *Manager) registry(projectID string) (Registry, error) {
	m.mu.RLock()
	root := m.projects[projectID]
	m.mu.RUnlock()
	if root == "" {
		return Registry{}, errors.New("model project is not registered")
	}
	return Registry{Workspace: root}, nil
}

func (m *Manager) Generate(ctx context.Context, projectID string, request GenerationRequest) (Package, error) {
	registry, err := m.registry(projectID)
	if err != nil {
		return Package{}, err
	}
	return registry.Generate(request)
}
func (m *Manager) GenerateGrounded(ctx context.Context, projectID string, request GroundedRequest) (Package, error) {
	registry, err := m.registry(projectID)
	if err != nil {
		return Package{}, err
	}
	return registry.GenerateGrounded(ctx, request, m.client)
}
func (m *Manager) Promote(projectID, id, version string, target ModelState, actor string, evidence *ConformanceEvidence) (Package, error) {
	registry, err := m.registry(projectID)
	if err != nil {
		return Package{}, err
	}
	return registry.Promote(id, version, target, actor, evidence)
}
