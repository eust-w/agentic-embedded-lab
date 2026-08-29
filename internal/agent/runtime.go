package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
	"github.com/google/uuid"
)

type Runtime struct {
	store        *store.Store
	client       *ResponsesClient
	bus          *events.Bus
	mu           sync.Mutex
	runs         map[string]context.CancelFunc
	projectRoots map[string]string
	tools        *tools.Registry
	projectTools map[string]*tools.Registry
	policy       *approval.Engine
	hooks        *plugins.HookDispatcher
	approvals    *ApprovalBroker
}

func NewRuntime(state *store.Store, client *ResponsesClient, bus *events.Bus) *Runtime {
	return &Runtime{store: state, client: client, bus: bus, runs: make(map[string]context.CancelFunc), projectRoots: make(map[string]string), projectTools: make(map[string]*tools.Registry), approvals: NewApprovalBroker()}
}

func (r *Runtime) ConfigureTools(registry *tools.Registry, policy *approval.Engine, hooks *plugins.HookDispatcher) {
	r.tools, r.policy, r.hooks = registry, policy, hooks
}

func (r *Runtime) ConfigureProjectTools(projectID string, registry *tools.Registry, policy *approval.Engine, hooks *plugins.HookDispatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projectTools[projectID] = registry
	r.policy, r.hooks = policy, hooks
}

func (r *Runtime) RegisterProject(projectID, root string) {
	r.mu.Lock()
	r.projectRoots[projectID] = root
	r.mu.Unlock()
}

func (r *Runtime) ResolveApproval(id string, allow bool) bool { return r.approvals.Resolve(id, allow) }

func (r *Runtime) CreateThread(ctx context.Context, projectID, title, model string, permission protocol.PermissionProfile) (protocol.Thread, error) {
	if projectID == "" || title == "" {
		return protocol.Thread{}, errors.New("projectID and title are required")
	}
	return r.store.CreateThread(ctx, projectID, title, model, permission)
}

func (r *Runtime) ListThreads(ctx context.Context, projectID string) ([]protocol.Thread, error) {
	return r.store.ListThreads(ctx, projectID)
}

func (r *Runtime) Items(ctx context.Context, threadID string, after int64, limit int) ([]protocol.Item, error) {
	return r.store.Items(ctx, threadID, after, limit)
}

func (r *Runtime) RunTurn(ctx context.Context, thread protocol.Thread, input string) (protocol.Turn, error) {
	turn, err := r.store.StartTurn(ctx, thread.ID, input)
	if err != nil {
		return protocol.Turn{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.runs[turn.ID] = cancel
	r.mu.Unlock()
	go r.execute(ctx, thread, turn)
	return turn, nil
}

func (r *Runtime) CancelTurn(turnID string) bool {
	r.mu.Lock()
	cancel, ok := r.runs[turnID]
	if ok {
		delete(r.runs, turnID)
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *Runtime) execute(ctx context.Context, thread protocol.Thread, turn protocol.Turn) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, turn.ID)
		r.mu.Unlock()
	}()
	_, _ = r.store.AppendItem(ctx, protocol.Item{ThreadID: thread.ID, TurnID: turn.ID, Type: protocol.ItemUserMessage, Payload: map[string]any{"text": turn.Input}})
	request := ResponseRequest{
		Model:     thread.Model,
		Input:     []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": turn.Input}}}},
		Reasoning: map[string]any{"effort": "high"},
		Metadata:  map[string]string{"thread_id": thread.ID, "turn_id": turn.ID},
	}
	registry := r.registryForProject(thread.ProjectID)
	if registry != nil {
		request.Tools = registry.Definitions()
	}
	for step := 0; step < 8; step++ {
		calls := make(map[string]*pendingFunctionCall)
		responseID := ""
		err := r.client.Stream(ctx, request, fmt.Sprintf("%s-%d", turn.ID, step), func(event ResponseEvent) error {
			itemType := protocol.ItemReasoning
			if strings.Contains(event.Type, "output_text") {
				itemType = protocol.ItemAgentMessage
			}
			item, err := r.store.AppendItem(ctx, protocol.Item{ThreadID: thread.ID, TurnID: turn.ID, Type: itemType, Payload: map[string]any{"event_type": event.Type, "delta": event.Delta, "raw": event.Payload}})
			if err == nil {
				r.bus.Publish(events.Event{Topic: "thread.item", Data: item})
			}
			collectFunctionEvent(event, calls, &responseID)
			return err
		})
		if err != nil {
			r.failTurn(thread.ID, turn.ID, err)
			return
		}
		if len(calls) == 0 {
			if err := r.store.FinishTurn(context.Background(), thread.ID, turn.ID, protocol.ThreadCompleted, ""); err != nil {
				r.failTurn(thread.ID, turn.ID, err)
				return
			}
			r.bus.Publish(events.Event{Topic: "turn.completed", Data: map[string]any{"turn_id": turn.ID}})
			return
		}
		outputs, err := r.executeFunctionCalls(ctx, thread, turn, calls, registry)
		if err != nil {
			r.failTurn(thread.ID, turn.ID, err)
			return
		}
		request.Input = outputs
		request.PreviousID = responseID
	}
	r.failTurn(thread.ID, turn.ID, errors.New("tool loop exceeded 8 steps"))
}

type pendingFunctionCall struct {
	ItemID    string
	CallID    string
	Name      string
	Arguments strings.Builder
}

func collectFunctionEvent(event ResponseEvent, calls map[string]*pendingFunctionCall, responseID *string) {
	if response, ok := event.Payload["response"].(map[string]any); ok {
		if id, ok := response["id"].(string); ok {
			*responseID = id
		}
	}
	if item, ok := event.Payload["item"].(map[string]any); ok {
		if itemType, _ := item["type"].(string); itemType == "function_call" {
			itemID, _ := item["id"].(string)
			call := calls[itemID]
			if call == nil {
				call = &pendingFunctionCall{ItemID: itemID}
				calls[itemID] = call
			}
			call.CallID, _ = item["call_id"].(string)
			call.Name, _ = item["name"].(string)
			if arguments, ok := item["arguments"].(string); ok && arguments != "" {
				call.Arguments.Reset()
				call.Arguments.WriteString(arguments)
			}
		}
	}
	itemID, _ := event.Payload["item_id"].(string)
	if call := calls[itemID]; call != nil && event.Delta != "" {
		call.Arguments.WriteString(event.Delta)
	}
}

func (r *Runtime) executeFunctionCalls(ctx context.Context, thread protocol.Thread, turn protocol.Turn, pending map[string]*pendingFunctionCall, registry *tools.Registry) ([]map[string]any, error) {
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	outputs := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		call := pending[id]
		if call.CallID == "" || call.Name == "" {
			return nil, errors.New("model returned an incomplete function call")
		}
		var arguments map[string]any
		if err := json.Unmarshal([]byte(call.Arguments.String()), &arguments); err != nil {
			return nil, fmt.Errorf("decode arguments for %s: %w", call.Name, err)
		}
		if registry == nil {
			return nil, errors.New("project has no configured tool registry")
		}
		tool, ok := registry.Get(call.Name)
		if !ok {
			return nil, fmt.Errorf("tool %s is not registered", call.Name)
		}
		operation := tool.Operation(arguments)
		operation.ThreadID, operation.ProjectID = thread.ID, thread.ProjectID
		if r.hooks != nil {
			results, err := r.hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPreToolUse, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"tool": call.Name, "arguments": arguments}})
			if err != nil {
				return nil, err
			}
			for _, result := range results {
				if result.Block {
					return nil, fmt.Errorf("tool blocked by hook: %s", result.Reason)
				}
			}
		}
		decision := approval.Result{Decision: approval.DecisionAllow}
		if r.policy != nil {
			r.mu.Lock()
			workspace := r.projectRoots[thread.ProjectID]
			r.mu.Unlock()
			decision = r.policy.Evaluate(thread.Permission, workspace, operation)
		}
		if decision.Decision == approval.DecisionDeny {
			outputs = append(outputs, functionOutput(call.CallID, map[string]any{"success": false, "error": decision.Reason}))
			continue
		}
		if decision.Decision == approval.DecisionAsk {
			request := protocol.ApprovalRequest{APIVersion: protocol.APIVersion, ID: uuid.NewString(), ThreadID: thread.ID, TurnID: turn.ID, Tool: call.Name, Reason: decision.Reason, Risk: operation.Risk, Scope: protocol.ApprovalOnce, Resource: operation.Resource, Metadata: map[string]any{"arguments": arguments}, CreatedAt: time.Now().UTC()}
			r.approvals.Prepare(request.ID)
			item, _ := r.store.AppendItem(ctx, protocol.Item{ThreadID: thread.ID, TurnID: turn.ID, Type: protocol.ItemApproval, Payload: map[string]any{"request": request}})
			r.bus.Publish(events.Event{Topic: "approval.requested", Data: item})
			allowed, err := r.approvals.Wait(ctx, request.ID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				outputs = append(outputs, functionOutput(call.CallID, map[string]any{"success": false, "error": "user denied approval"}))
				continue
			}
		}
		protocolCall := protocol.ToolCall{ID: call.ItemID, ThreadID: thread.ID, TurnID: turn.ID, Name: call.Name, Arguments: arguments, IdempotencyKey: thread.ID + ":" + turn.ID + ":" + call.CallID}
		execute, cached, err := r.store.BeginToolCall(ctx, protocolCall)
		if err != nil {
			return nil, err
		}
		result := cached
		if execute {
			started := time.Now()
			toolResult, toolErr := tool.Execute(ctx, arguments)
			result = protocol.ToolResult{CallID: call.CallID, Success: toolErr == nil, Output: toolResult.Output, ArtifactIDs: toolResult.ArtifactIDs, DurationMS: time.Since(started).Milliseconds()}
			if toolErr != nil {
				result.Error = toolErr.Error()
			}
			if err := r.store.FinishToolCall(ctx, protocolCall.IdempotencyKey, result); err != nil {
				return nil, err
			}
		}
		_, _ = r.store.AppendItem(ctx, protocol.Item{ThreadID: thread.ID, TurnID: turn.ID, Type: protocol.ItemToolResult, Payload: map[string]any{"tool": call.Name, "result": result}})
		outputs = append(outputs, functionOutput(call.CallID, result))
	}
	return outputs, nil
}

func (r *Runtime) registryForProject(projectID string) *tools.Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if registry := r.projectTools[projectID]; registry != nil {
		return registry
	}
	return r.tools
}

func functionOutput(callID string, output any) map[string]any {
	payload, _ := json.Marshal(output)
	return map[string]any{"type": "function_call_output", "call_id": callID, "output": string(payload)}
}

func (r *Runtime) failTurn(threadID, turnID string, err error) {
	status := protocol.ThreadFailed
	if errors.Is(err, context.Canceled) {
		status = protocol.ThreadCancelled
	}
	_ = r.store.FinishTurn(context.Background(), threadID, turnID, status, err.Error())
	_, _ = r.store.AppendItem(context.Background(), protocol.Item{ThreadID: threadID, TurnID: turnID, Type: protocol.ItemAgentMessage, Payload: map[string]any{"error": err.Error(), "at": time.Now().UTC()}})
	topic := "turn.failed"
	if status == protocol.ThreadCancelled {
		topic = "turn.cancelled"
	}
	r.bus.Publish(events.Event{Topic: topic, Data: map[string]any{"turn_id": turnID, "error": err.Error()}})
}

type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func NewApprovalBroker() *ApprovalBroker { return &ApprovalBroker{pending: make(map[string]chan bool)} }

func (b *ApprovalBroker) Prepare(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending[id] == nil {
		b.pending[id] = make(chan bool, 1)
	}
}

func (b *ApprovalBroker) Wait(ctx context.Context, id string) (bool, error) {
	b.mu.Lock()
	channel := b.pending[id]
	if channel == nil {
		channel = make(chan bool, 1)
		b.pending[id] = channel
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case allowed := <-channel:
		return allowed, nil
	}
}

func (b *ApprovalBroker) Resolve(id string, allow bool) bool {
	b.mu.Lock()
	channel, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case channel <- allow:
		return true
	default:
		return false
	}
}
