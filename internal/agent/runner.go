package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	goagent "github.com/jgabor/go-agent"
)

// Runner executes one bounded Aila-owned agent request and returns provider-style events.
type Runner interface {
	Stream(context.Context, RunRequest) (<-chan Event, error)
}

// RunRequest is the Aila-owned request shape passed behind the agent adapter.
type RunRequest struct {
	Prompt       string
	Instructions string
	Provider     string
	Model        string
	SessionID    string
	Context      []ContextMessage
	RunID        string
	MaxSteps     int
	ToolNames    []string
	// DispatchToolsThroughHost makes go-agent expose tool definitions but prevents
	// inline tool IO; the host must dispatch mapped EventToolRequest values.
	DispatchToolsThroughHost bool
}

// ContextMessage is Aila's in-process conversation context for a new agent run.
type ContextMessage struct {
	Role    string
	Content string
}

// FailureMode selects deterministic fake provider failures for tests and PTY smoke.
type FailureMode string

const (
	FailureNone             FailureMode = ""
	FailureProviderAuth     FailureMode = "provider_auth_failed"
	FailureProviderTimeout  FailureMode = "provider_timeout"
	FailureRateLimited      FailureMode = "rate_limited"
	FailureModelUnavailable FailureMode = "model_unavailable"
	FailureStreamError      FailureMode = "stream_error"
)

// FakeReadOnlyRunner is the default deterministic runner used by app tests and smoke tests.
type FakeReadOnlyRunner struct {
	Failure        FailureMode
	Events         []Event
	ObserveRequest func(RunRequest)
}

// DefaultFakeReadOnlyRunner returns a bounded read-only model/tool script.
func DefaultFakeReadOnlyRunner() FakeReadOnlyRunner {
	return FakeReadOnlyRunner{}
}

// FakeBuildRunner is the deterministic interactive build runner used by the live TUI loop.
type FakeBuildRunner struct {
	Failure        FailureMode
	Events         []Event
	ObserveRequest func(RunRequest)
}

// UnavailableRunner reports a bounded provider setup failure without silently falling back to fake output.
type UnavailableRunner struct {
	Provider string
	Model    string
	Failure  ProviderError
}

// DefaultFakeBuildRunner returns a bounded read/write model/tool script.
func DefaultFakeBuildRunner() FakeBuildRunner {
	return FakeBuildRunner{}
}

// Stream implements Runner without live provider IO.
func (runner FakeReadOnlyRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner.ObserveRequest != nil {
		runner.ObserveRequest(cloneRunRequest(request))
	}
	events := runner.Events
	if len(events) == 0 {
		events = defaultFakeReadOnlyEvents(request, runner.Failure)
	}
	out := make(chan Event, len(events))
	go func() {
		defer close(out)
		for _, event := range events {
			select {
			case <-ctx.Done():
				out <- Event{Kind: EventError, Provider: requestProvider(request), Model: requestModel(request), Error: ProviderError{Code: string(FailureStreamError), Message: ctx.Err().Error(), Retryable: false}}
				return
			case out <- withRequestIdentity(event, request):
			}
		}
	}()
	return out, nil
}

func defaultFakeReadOnlyEvents(request RunRequest, failure FailureMode) []Event {
	provider := requestProvider(request)
	model := requestModel(request)
	if failure != FailureNone {
		return []Event{{Kind: EventError, Provider: provider, Model: model, Sequence: 1, Error: providerFailure(failure)}}
	}
	return []Event{
		{Kind: EventAssistantDelta, Provider: provider, Model: model, Sequence: 1, Text: "I will inspect README.md before answering. "},
		{Kind: EventToolRequest, Provider: provider, Model: model, Sequence: 2, ToolCallID: "call-read-1", ToolName: "read", Arguments: []ToolArgument{{Name: "path", Value: "README.md"}, {Name: "line_limit", Value: "6"}}},
		{Kind: EventAssistantDelta, Provider: provider, Model: model, Sequence: 3, Text: "Read-only inspection completed. Aila is a terminal coding agent."},
		{Kind: EventCompleted, Provider: provider, Model: model, Sequence: 4, FinishReason: "complete"},
	}
}

// Stream implements Runner without live provider IO.
func (runner FakeBuildRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner.ObserveRequest != nil {
		runner.ObserveRequest(cloneRunRequest(request))
	}
	events := runner.Events
	if len(events) == 0 {
		events = defaultFakeBuildEvents(request, runner.Failure)
	}
	out := make(chan Event, len(events))
	go func() {
		defer close(out)
		for _, event := range events {
			select {
			case <-ctx.Done():
				out <- Event{Kind: EventError, Provider: requestProvider(request), Model: requestModel(request), Error: ProviderError{Code: string(FailureStreamError), Message: ctx.Err().Error(), Retryable: false}}
				return
			case out <- withRequestIdentity(event, request):
			}
		}
	}()
	return out, nil
}

// Stream implements Runner by returning one provider setup failure event.
func (runner UnavailableRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	failure := runner.Failure
	if failure.Code == "" {
		failure.Code = string(FailureModelUnavailable)
	}
	if failure.Message == "" {
		failure.Message = "agent runner unavailable"
	}
	provider := boundedEventText(runner.Provider)
	if provider == "" {
		provider = requestProvider(request)
	}
	model := boundedEventText(runner.Model)
	if model == "" {
		model = requestModel(request)
	}
	out := make(chan Event, 1)
	go func() {
		defer close(out)
		out <- Event{Kind: EventError, Provider: provider, Model: model, Sequence: 1, Error: failure}
	}()
	return out, nil
}

func cloneRunRequest(request RunRequest) RunRequest {
	request.Context = append([]ContextMessage(nil), request.Context...)
	request.ToolNames = append([]string(nil), request.ToolNames...)
	return request
}

func defaultFakeBuildEvents(request RunRequest, failure FailureMode) []Event {
	provider := requestProvider(request)
	model := requestModel(request)
	if failure != FailureNone {
		return []Event{{Kind: EventError, Provider: provider, Model: model, Sequence: 1, Error: providerFailure(failure)}}
	}
	if !requestHasTool(request, "write") || !promptRequestsWorkspaceWrite(request.Prompt) {
		return defaultFakeReadOnlyEvents(request, FailureNone)
	}
	path := "docs/interactive-build-output.md"
	content := "Interactive build output for prompt: " + boundedPromptLine(request.Prompt)
	return []Event{
		{Kind: EventAssistantDelta, Provider: provider, Model: model, Sequence: 1, Text: "I will inspect README.md before proposing a guarded workspace write. "},
		{Kind: EventToolRequest, Provider: provider, Model: model, Sequence: 2, ToolCallID: "call-read-1", ToolName: "read", Arguments: []ToolArgument{{Name: "path", Value: "README.md"}, {Name: "line_limit", Value: "6"}}},
		{Kind: EventAssistantDelta, Provider: provider, Model: model, Sequence: 3, Text: "Read context is ready. I need approval before creating docs/interactive-build-output.md. "},
		{Kind: EventToolRequest, Provider: provider, Model: model, Sequence: 4, ToolCallID: "call-write-1", ToolName: "write", Arguments: []ToolArgument{{Name: "path", Value: path}, {Name: "target_version", Value: "missing"}, {Name: "content", Value: content}, {Name: "expected_effect", Value: "create interactive build output file"}}},
	}
}

func requestHasTool(request RunRequest, name string) bool {
	for _, tool := range request.ToolNames {
		if strings.EqualFold(strings.TrimSpace(tool), name) {
			return true
		}
	}
	return false
}

func promptRequestsWorkspaceWrite(prompt string) bool {
	lower := strings.ToLower(prompt)
	writeVerbs := []string{"create", "write", "add", "edit", "change", "update", "modify"}
	writeNouns := []string{"file", "note", "doc", "document", "output"}
	for _, verb := range writeVerbs {
		if !strings.Contains(lower, verb) {
			continue
		}
		for _, noun := range writeNouns {
			if strings.Contains(lower, noun) {
				return true
			}
		}
	}
	return false
}

func boundedPromptLine(prompt string) string {
	fields := strings.Fields(prompt)
	if len(fields) == 0 {
		return "interactive build request"
	}
	line := strings.Join(fields, " ")
	if len([]rune(line)) <= 120 {
		return line
	}
	runes := []rune(line)
	return string(runes[:120])
}

func providerFailure(failure FailureMode) ProviderError {
	switch failure {
	case FailureProviderAuth:
		return ProviderError{Code: string(failure), Message: "provider authentication failed", Retryable: false}
	case FailureProviderTimeout:
		return ProviderError{Code: string(failure), Message: "provider request timed out", Retryable: true}
	case FailureRateLimited:
		return ProviderError{Code: string(failure), Message: "provider rate limit reached", Retryable: true}
	case FailureModelUnavailable:
		return ProviderError{Code: string(failure), Message: "model unavailable", Retryable: false}
	default:
		return ProviderError{Code: string(FailureStreamError), Message: "provider stream failed", Retryable: true}
	}
}

func withRequestIdentity(event Event, request RunRequest) Event {
	if event.Provider == "" {
		event.Provider = requestProvider(request)
	}
	if event.Model == "" {
		event.Model = requestModel(request)
	}
	return event
}

// GoAgentRunner adapts github.com/jgabor/go-agent behind Aila's event contract.
type GoAgentRunner struct {
	runner       goagent.Runner
	provider     string
	model        string
	instructions string
}

// NewGoAgentRunner constructs an adapter over the real go-agent Runner.
func NewGoAgentRunner(model goagent.Model, provider string, modelID string, tools ...goagent.Tool) (*GoAgentRunner, error) {
	return NewGoAgentRunnerWithInstructions(model, provider, modelID, "", tools...)
}

// NewGoAgentRunnerWithInstructions constructs an adapter with host-owned system instructions.
func NewGoAgentRunnerWithInstructions(model goagent.Model, provider string, modelID string, instructions string, tools ...goagent.Tool) (*GoAgentRunner, error) {
	instructions = normalizeInstructions(instructions)
	wrapped, err := goagent.NewRunner(goagent.Agent{Instructions: instructions, Model: model, Tools: append([]goagent.Tool(nil), tools...), Policy: hostDispatchToolPolicy()})
	if err != nil {
		return nil, err
	}
	return &GoAgentRunner{runner: wrapped, provider: boundedEventText(provider), model: boundedEventText(modelID), instructions: instructions}, nil
}

const hostDispatchToolsMetadataKey = "aila.dispatch_tools_through_host"

type hostDispatchToolsContextKey struct{}

func hostDispatchToolPolicy() goagent.Policy {
	return goagent.PolicyFunc(func(ctx context.Context, decision goagent.Decision) (goagent.PolicyDecision, error) {
		if decision.Kind == goagent.DecisionToolCall && hostDispatchToolsEnabled(ctx, decision.Request) {
			return goagent.PolicyDecision{
				Allowed: false,
				Reason:  "Aila capability tool requests are dispatched through host effects.",
				ToolResult: &goagent.ToolResult{
					CallID:   decision.ToolCall.ID,
					Name:     decision.ToolCall.Name,
					Content:  "Tool request recorded for Aila host effect dispatch; no go-agent tool IO was executed inline.",
					Metadata: map[string]string{"aila_dispatch": "host_effect"},
				},
			}, nil
		}
		return goagent.PolicyDecision{Allowed: true}, nil
	})
}

func hostDispatchToolsEnabled(ctx context.Context, request goagent.RunRequest) bool {
	return request.Metadata[hostDispatchToolsMetadataKey] == "true" || ctx.Value(hostDispatchToolsContextKey{}) == true
}

// Stream runs go-agent and maps its stream into Aila provider-style events.
func (runner *GoAgentRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if runner == nil || runner.runner == nil {
		return nil, fmt.Errorf("agent runner is nil")
	}
	instructions := strings.TrimSpace(request.Instructions)
	if instructions == "" {
		instructions = runner.instructions
	}
	instructions = normalizeInstructions(instructions)
	metadata := map[string]string(nil)
	streamCtx := ctx
	if request.DispatchToolsThroughHost {
		metadata = map[string]string{hostDispatchToolsMetadataKey: "true"}
		streamCtx = context.WithValue(ctx, hostDispatchToolsContextKey{}, true)
	}
	stream, err := runner.runner.Stream(streamCtx, goagent.RunRequest{
		Input:        strings.TrimSpace(request.Prompt),
		SessionID:    strings.TrimSpace(request.SessionID),
		Session:      goagent.Session{ID: strings.TrimSpace(request.SessionID), Messages: goAgentContextMessages(request.Context)},
		Instructions: instructions,
		RunID:        strings.TrimSpace(request.RunID),
		MaxSteps:     request.MaxSteps,
		ToolNames:    append([]string(nil), request.ToolNames...),
		Metadata:     metadata,
	})
	if err != nil {
		return nil, err
	}
	out := make(chan Event)
	go func() {
		defer close(out)
		for event := range stream {
			for _, mapped := range mapGoAgentEvent(event, runner.provider, runner.model) {
				out <- withRequestIdentity(mapped, request)
			}
		}
	}()
	return out, nil
}

func goAgentContextMessages(context []ContextMessage) []goagent.Message {
	if len(context) == 0 {
		return nil
	}
	messages := make([]goagent.Message, 0, len(context))
	for _, item := range context {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		role := goAgentRole(item.Role)
		if role == goagent.RoleTool {
			role = goagent.RoleAssistant
			content = "Tool evidence: " + content
		}
		messages = append(messages, goagent.Message{Role: role, Content: content})
	}
	return messages
}

func goAgentRole(role string) goagent.Role {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(goagent.RoleUser):
		return goagent.RoleUser
	case string(goagent.RoleTool):
		return goagent.RoleTool
	default:
		return goagent.RoleAssistant
	}
}

func normalizeInstructions(instructions string) string {
	instructions = strings.TrimSpace(instructions)
	if instructions != "" {
		return instructions
	}
	return "You are Aila, a terminal coding agent. Inspect before answering, respect the configured autonomy boundary, and use only the fixed built-in tools exposed for this run."
}

func mapGoAgentEvent(event goagent.Event, provider string, model string) []Event {
	sequence := int(event.Sequence)
	switch event.Kind {
	case goagent.EventTextDelta:
		return []Event{{Kind: EventAssistantDelta, Provider: provider, Model: model, Sequence: sequence, Text: event.Text}}
	case goagent.EventToolCall:
		return []Event{{Kind: EventToolRequest, Provider: provider, Model: model, Sequence: sequence, ToolCallID: event.ToolCall.ID, ToolName: event.ToolCall.Name, Arguments: toolArgumentsFromJSON(event.ToolCall.Input)}}
	case goagent.EventError:
		return []Event{{Kind: EventError, Provider: provider, Model: model, Sequence: sequence, Error: classifyGoAgentError(event)}}
	case goagent.EventStop:
		if event.StopReason == goagent.StopComplete || event.StopReason == "" {
			return []Event{{Kind: EventCompleted, Provider: provider, Model: model, Sequence: sequence, FinishReason: string(goagent.StopComplete)}}
		}
		if event.StopReason == goagent.StopStepLimit {
			return []Event{{Kind: EventPaused, Provider: provider, Model: model, Sequence: sequence, FinishReason: string(goagent.StopStepLimit)}}
		}
		return []Event{{Kind: EventError, Provider: provider, Model: model, Sequence: sequence, Error: ProviderError{Code: string(event.StopReason), Message: "agent stopped: " + string(event.StopReason), Retryable: retryableStop(event.StopReason)}}}
	default:
		return nil
	}
}

func classifyGoAgentError(event goagent.Event) ProviderError {
	message := "provider stream failed"
	if event.Err != nil {
		message = event.Err.Error()
	}
	code := string(FailureStreamError)
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "api key") || strings.Contains(lower, "auth") || strings.Contains(lower, "credential"):
		code = string(FailureProviderAuth)
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline"):
		code = string(FailureProviderTimeout)
	case strings.Contains(lower, "rate") || strings.Contains(lower, "429"):
		code = string(FailureRateLimited)
	case strings.Contains(lower, "model") && (strings.Contains(lower, "unavailable") || strings.Contains(lower, "not found")):
		code = string(FailureModelUnavailable)
	}
	return ProviderError{Code: code, Message: message, Retryable: code == string(FailureProviderTimeout) || code == string(FailureRateLimited) || code == string(FailureStreamError)}
}

func retryableStop(reason goagent.StopReason) bool {
	switch reason {
	case goagent.StopModelError, goagent.StopDurationLimit, goagent.StopRetryExhausted:
		return true
	default:
		return false
	}
}

func toolArgumentsFromJSON(data json.RawMessage) []ToolArgument {
	if len(data) == 0 {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return []ToolArgument{{Name: "input", Value: string(data)}}
	}
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	arguments := make([]ToolArgument, 0, len(names))
	for _, name := range names {
		arguments = append(arguments, ToolArgument{Name: name, Value: fmt.Sprint(object[name])})
	}
	return arguments
}

func requestProvider(request RunRequest) string {
	if request.Provider != "" {
		return request.Provider
	}
	return "fake"
}

func requestModel(request RunRequest) string {
	if request.Model != "" {
		return request.Model
	}
	return "fake-readonly"
}
