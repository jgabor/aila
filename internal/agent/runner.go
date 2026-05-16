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
	Prompt    string
	Provider  string
	Model     string
	RunID     string
	MaxSteps  int
	ToolNames []string
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
	Failure FailureMode
	Events  []Event
}

// DefaultFakeReadOnlyRunner returns a bounded read-only model/tool script.
func DefaultFakeReadOnlyRunner() FakeReadOnlyRunner {
	return FakeReadOnlyRunner{}
}

// Stream implements Runner without live provider IO.
func (runner FakeReadOnlyRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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
	runner   goagent.Runner
	provider string
	model    string
}

// NewGoAgentRunner constructs an adapter over the real go-agent Runner.
func NewGoAgentRunner(model goagent.Model, provider string, modelID string, tools ...goagent.Tool) (*GoAgentRunner, error) {
	wrapped, err := goagent.NewRunner(goagent.Agent{Model: model, Tools: append([]goagent.Tool(nil), tools...)})
	if err != nil {
		return nil, err
	}
	return &GoAgentRunner{runner: wrapped, provider: boundedEventText(provider), model: boundedEventText(modelID)}, nil
}

// Stream runs go-agent and maps its stream into Aila provider-style events.
func (runner *GoAgentRunner) Stream(ctx context.Context, request RunRequest) (<-chan Event, error) {
	if runner == nil || runner.runner == nil {
		return nil, fmt.Errorf("agent runner is nil")
	}
	stream, err := runner.runner.Stream(ctx, goagent.RunRequest{
		Input:     strings.TrimSpace(request.Prompt),
		RunID:     strings.TrimSpace(request.RunID),
		MaxSteps:  request.MaxSteps,
		ToolNames: append([]string(nil), request.ToolNames...),
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
