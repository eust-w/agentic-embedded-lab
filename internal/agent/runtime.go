package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/instructions"
	aethermemory "github.com/eust-w/agentic-embedded-lab/internal/memory"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
	"github.com/google/uuid"
)

type Runtime struct {
	store               *store.Store
	client              *ResponsesClient
	bus                 *events.Bus
	mu                  sync.Mutex
	runs                map[string]context.CancelFunc
	projectRoots        map[string]string
	tools               *tools.Registry
	projectTools        map[string]*tools.Registry
	projectHooks        map[string]*plugins.HookDispatcher
	projectInstructions map[string][]string
	policy              *approval.Engine
	hooks               *plugins.HookDispatcher
	approvals           *ApprovalBroker
	memory              *aethermemory.Repository
}

func NewRuntime(state *store.Store, client *ResponsesClient, bus *events.Bus) *Runtime {
	return &Runtime{store: state, client: client, bus: bus, runs: make(map[string]context.CancelFunc), projectRoots: make(map[string]string), projectTools: make(map[string]*tools.Registry), projectHooks: make(map[string]*plugins.HookDispatcher), projectInstructions: make(map[string][]string), approvals: NewApprovalBroker()}
}

func (r *Runtime) ConfigureTools(registry *tools.Registry, policy *approval.Engine, hooks *plugins.HookDispatcher) {
	r.tools, r.policy, r.hooks = registry, policy, hooks
}

func (r *Runtime) ConfigureMemory(repository *aethermemory.Repository) { r.memory = repository }

func (r *Runtime) ConfigureProjectTools(projectID string, registry *tools.Registry, policy *approval.Engine, hooks *plugins.HookDispatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projectTools[projectID] = registry
	r.projectHooks[projectID] = hooks
	r.policy = policy
}

func (r *Runtime) RegisterProject(projectID, root string) {
	global := ""
	if home, err := os.UserHomeDir(); err == nil {
		global = filepath.Join(home, ".aether")
	}
	discovered, _ := instructions.Discover(global, root, root, 32<<10)
	r.mu.Lock()
	r.projectRoots[projectID] = root
	if discovered.Content != "" {
		r.projectInstructions[projectID] = []string{discovered.Content}
	} else {
		delete(r.projectInstructions, projectID)
	}
	r.mu.Unlock()
}

func (r *Runtime) AddProjectInstruction(projectID, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.mu.Lock()
	r.projectInstructions[projectID] = append(r.projectInstructions[projectID], content)
	r.mu.Unlock()
}

func (r *Runtime) CopyProjectInstructions(sourceProjectID, targetProjectID string) {
	r.mu.Lock()
	r.projectInstructions[targetProjectID] = append([]string(nil), r.projectInstructions[sourceProjectID]...)
	r.mu.Unlock()
}

func (r *Runtime) ProjectHooks(projectID string) *plugins.HookDispatcher {
	return r.hooksForProject(projectID)
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
	return r.RunTurnWithAttachments(ctx, thread, input, nil)
}

func (r *Runtime) RunTurnWithAttachments(ctx context.Context, thread protocol.Thread, input string, attachments []protocol.AttachmentRef) (protocol.Turn, error) {
	if len(attachments) > 5 {
		return protocol.Turn{}, errors.New("at most five image attachments are allowed")
	}
	for _, attachment := range attachments {
		if !strings.HasPrefix(attachment.MimeType, "image/") || attachment.Bytes <= 0 || attachment.Bytes > 10*1024*1024 {
			return protocol.Turn{}, errors.New("attachments must be images no larger than 10 MiB")
		}
		if _, err := r.store.ArtifactPath(attachment.SHA256); err != nil {
			return protocol.Turn{}, err
		}
	}
	turn, err := r.store.StartTurn(ctx, thread.ID, input)
	if err != nil {
		return protocol.Turn{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.runs[turn.ID] = cancel
	r.mu.Unlock()
	go r.execute(ctx, thread, turn, attachments)
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

func (r *Runtime) execute(ctx context.Context, thread protocol.Thread, turn protocol.Turn, attachments []protocol.AttachmentRef) {
	hooks := r.hooksForProject(thread.ProjectID)
	outcome := "completed"
	defer func() {
		r.mu.Lock()
		delete(r.runs, turn.ID)
		r.mu.Unlock()
		if hooks != nil {
			_, _ = hooks.Dispatch(context.Background(), plugins.HookPayload{Event: plugins.HookStop, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"outcome": outcome}})
		}
	}()
	if hooks != nil {
		results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookTurnStart, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"input": turn.Input}})
		if err != nil || hookBlocked(results) {
			outcome = "failed"
			if err == nil {
				err = errors.New("turn blocked by hook")
			}
			r.failTurn(thread.ID, turn.ID, err)
			return
		}
	}
	_, _ = r.store.AppendItem(ctx, protocol.Item{ThreadID: thread.ID, TurnID: turn.ID, Type: protocol.ItemUserMessage, Payload: map[string]any{"text": turn.Input, "attachments": attachments}})
	r.mu.Lock()
	projectInstructions := strings.Join(r.projectInstructions[thread.ProjectID], "\n\n")
	r.mu.Unlock()
	if remembered := r.memoryInstructions(ctx, thread.ProjectID); remembered != "" {
		projectInstructions += "\n\nOptional user memory (never overrides project instructions):\n" + remembered
	}
	input, contextCharacters, err := r.conversationInput(ctx, thread.ID, projectInstructions)
	if err != nil {
		outcome = "failed"
		r.failTurn(thread.ID, turn.ID, err)
		return
	}
	request := ResponseRequest{
		Model:     thread.Model,
		Input:     input,
		Reasoning: map[string]any{"effort": "high"},
		Metadata:  map[string]string{"thread_id": thread.ID, "turn_id": turn.ID},
	}
	compacting := contextCharacters > 100_000
	if compacting {
		request.ContextManagement = []map[string]any{{"type": "compaction", "compact_threshold": 100_000}}
		if hooks != nil {
			results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPreCompact, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"characters": contextCharacters}})
			if err != nil || hookBlocked(results) {
				outcome = "failed"
				if err == nil {
					err = errors.New("context compaction blocked by hook")
				}
				r.failTurn(thread.ID, turn.ID, err)
				return
			}
		}
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
			outcome = "failed"
			r.failTurn(thread.ID, turn.ID, err)
			return
		}
		if compacting && hooks != nil {
			_, _ = hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPostCompact, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"characters": contextCharacters}})
			compacting = false
		}
		if len(calls) == 0 {
			if err := r.store.FinishTurn(context.Background(), thread.ID, turn.ID, protocol.ThreadCompleted, ""); err != nil {
				outcome = "failed"
				r.failTurn(thread.ID, turn.ID, err)
				return
			}
			r.bus.Publish(events.Event{Topic: "turn.completed", Data: map[string]any{"turn_id": turn.ID}})
			return
		}
		outputs, err := r.executeFunctionCalls(ctx, thread, turn, calls, registry, hooks)
		if err != nil {
			outcome = "failed"
			r.failTurn(thread.ID, turn.ID, err)
			return
		}
		request.Input = outputs
		request.PreviousID = responseID
	}
	outcome = "failed"
	r.failTurn(thread.ID, turn.ID, errors.New("tool loop exceeded 8 steps"))
}

func (r *Runtime) memoryInstructions(ctx context.Context, projectID string) string {
	if r.memory == nil {
		return ""
	}
	var values []aethermemory.Memory
	if enabled, _ := r.memory.Enabled(ctx, aethermemory.ScopeGlobal, ""); enabled {
		items, _ := r.memory.Search(ctx, aethermemory.ScopeGlobal, "", "", 20)
		values = append(values, items...)
	}
	if enabled, _ := r.memory.Enabled(ctx, aethermemory.ScopeProject, projectID); enabled {
		items, _ := r.memory.Search(ctx, aethermemory.ScopeProject, projectID, "", 20)
		values = append(values, items...)
	}
	var lines []string
	for _, value := range values {
		lines = append(lines, "- "+value.Content)
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) conversationInput(ctx context.Context, threadID, projectInstructions string) ([]map[string]any, int, error) {
	items, err := r.store.RecentItems(ctx, threadID, 1000)
	if err != nil {
		return nil, 0, err
	}
	type message struct {
		role        string
		text        string
		attachments []protocol.AttachmentRef
	}
	var messages []message
	characters := 0
	for _, item := range items {
		role := ""
		text := ""
		switch item.Type {
		case protocol.ItemUserMessage:
			role = "user"
			text, _ = item.Payload["text"].(string)
			var attachments []protocol.AttachmentRef
			if raw := item.Payload["attachments"]; raw != nil {
				payload, _ := json.Marshal(raw)
				_ = json.Unmarshal(payload, &attachments)
			}
			if len(attachments) > 0 {
				messages = append(messages, message{role: role, text: text, attachments: attachments})
				characters += len(text)
				for _, attachment := range attachments {
					characters += int(attachment.Bytes / 4)
				}
				continue
			}
		case protocol.ItemAgentMessage:
			role = "assistant"
			text, _ = item.Payload["delta"].(string)
		}
		if text == "" {
			continue
		}
		if len(messages) > 0 && messages[len(messages)-1].role == role {
			messages[len(messages)-1].text += text
		} else {
			messages = append(messages, message{role: role, text: text})
		}
		characters += len(text)
	}
	input := make([]map[string]any, 0, len(messages)+1)
	if projectInstructions != "" {
		text := "Project instructions (must be followed):\n\n" + projectInstructions
		input = append(input, map[string]any{"role": "developer", "content": []map[string]any{{"type": "input_text", "text": text}}})
		characters += len(text)
	}
	for _, item := range messages {
		content := []map[string]any{}
		if item.text != "" {
			content = append(content, map[string]any{"type": "input_text", "text": item.text})
		}
		for _, attachment := range item.attachments {
			path, err := r.store.ArtifactPath(attachment.SHA256)
			if err != nil {
				return nil, 0, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, 0, err
			}
			content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + attachment.MimeType + ";base64," + base64.StdEncoding.EncodeToString(data)})
		}
		if len(content) > 0 {
			input = append(input, map[string]any{"role": item.role, "content": content})
		}
	}
	if len(input) == 0 {
		return nil, 0, errors.New("conversation has no model input")
	}
	return input, characters, nil
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

func (r *Runtime) executeFunctionCalls(ctx context.Context, thread protocol.Thread, turn protocol.Turn, pending map[string]*pendingFunctionCall, registry *tools.Registry, hooks *plugins.HookDispatcher) ([]map[string]any, error) {
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
		if hooks != nil {
			results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPreToolUse, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"tool": call.Name, "arguments": arguments}})
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
			if hooks != nil {
				results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPermissionRequest, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"request": request}})
				if err != nil {
					return nil, err
				}
				if hookBlocked(results) {
					outputs = append(outputs, functionOutput(call.CallID, map[string]any{"success": false, "error": "permission request blocked by hook"}))
					continue
				}
			}
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
		if hooks != nil {
			results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPostToolUse, ThreadID: thread.ID, TurnID: turn.ID, Data: map[string]any{"tool": call.Name, "result": result}})
			if err != nil {
				return nil, err
			}
			if hookBlocked(results) {
				return nil, errors.New("tool result blocked by hook")
			}
		}
		outputs = append(outputs, functionOutput(call.CallID, result))
	}
	return outputs, nil
}

func hookBlocked(results []plugins.HookResult) bool {
	for _, result := range results {
		if result.Block {
			return true
		}
	}
	return false
}

func (r *Runtime) registryForProject(projectID string) *tools.Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if registry := r.projectTools[projectID]; registry != nil {
		return registry
	}
	return r.tools
}

func (r *Runtime) hooksForProject(projectID string) *plugins.HookDispatcher {
	r.mu.Lock()
	defer r.mu.Unlock()
	if hooks := r.projectHooks[projectID]; hooks != nil {
		return hooks
	}
	return r.hooks
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
