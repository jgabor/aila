package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/runtime"
)

func TestAdaptEventsMapsFakeProviderStreamInOrder(t *testing.T) {
	t.Parallel()

	operation := runtime.OperationMetadata{ID: "op-agent", Kind: runtime.OperationPrompt, Subject: "explain", Source: "test"}
	events := FakeStream(
		Event{Kind: EventAssistantDelta, Provider: "fake", Model: "fake-model", Sequence: 1, Text: "Hello "},
		Event{Kind: EventAssistantDelta, Provider: "fake", Model: "fake-model", Sequence: 2, Text: "world"},
		Event{Kind: EventToolRequest, Provider: "fake", Model: "fake-model", Sequence: 3, ToolCallID: "call-1", ToolName: "read", Arguments: []ToolArgument{{Name: "path", Value: "README.md"}}},
		Event{Kind: EventCompleted, Provider: "fake", Model: "fake-model", Sequence: 4, FinishReason: "stop"},
	)

	messages := AdaptEvents(operation, events)
	want := []runtime.Message{
		runtime.AgentAssistantDelta{Operation: operation, Provider: "fake", Model: "fake-model", Sequence: 1, Text: "Hello"},
		runtime.AgentAssistantDelta{Operation: operation, Provider: "fake", Model: "fake-model", Sequence: 2, Text: "world"},
		runtime.AgentToolRequested{Operation: operation, Request: runtime.AgentToolRequest{ID: "call-1", Name: "read", Arguments: []runtime.AgentToolArgument{{Name: "path", Value: "README.md"}}, Provider: "fake", Model: "fake-model", Sequence: 3}},
		runtime.AgentTurnCompleted{Operation: operation, Provider: "fake", Model: "fake-model", FinishReason: "stop"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
}

func TestAdaptEventsMapsProviderErrorAndBoundsUnsafeText(t *testing.T) {
	t.Parallel()

	operation := runtime.OperationMetadata{ID: "op-agent", Kind: runtime.OperationPrompt, Subject: "explain", Source: "test"}
	messages := AdaptEvents(operation, FakeStream(
		Event{Kind: EventAssistantDelta, Provider: "fake", Model: "fake-model", Sequence: 1, Text: "token=secret \x1b[31m /home/jgabor/.config/aila/config.toml .aila/state"},
		Event{Kind: EventError, Provider: "fake", Model: "fake-model", Sequence: 2, Error: ProviderError{Code: "rate_limited", Message: "password=secret retry later", Retryable: true}},
	))

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	delta := messages[0].(runtime.AgentAssistantDelta)
	if strings.Contains(delta.Text, "secret") || strings.Contains(delta.Text, "/home") || strings.Contains(delta.Text, ".aila") || strings.Contains(delta.Text, "\x1b") || !strings.Contains(delta.Text, "[redacted]") {
		t.Fatalf("delta text was not bounded/redacted: %q", delta.Text)
	}
	failure := messages[1].(runtime.AgentTurnFailed)
	if failure.Failure.Code != "rate_limited" || !failure.Failure.Retryable || strings.Contains(failure.Failure.Message, "secret") || !strings.Contains(failure.Failure.Message, "[redacted]") {
		t.Fatalf("failure = %+v", failure.Failure)
	}
}

func TestAdaptEventsMapsStepLimitPauseAsResumableState(t *testing.T) {
	t.Parallel()

	operation := runtime.OperationMetadata{ID: "op-agent", Kind: runtime.OperationPrompt, Subject: "explain", Source: "test"}
	messages := AdaptEvents(operation, FakeStream(Event{Kind: EventPaused, Provider: "fake", Model: "fake-model", Sequence: 3, FinishReason: "step_limit"}))

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	paused, ok := messages[0].(runtime.AgentTurnPaused)
	if !ok {
		t.Fatalf("message = %T, want runtime.AgentTurnPaused", messages[0])
	}
	if paused.Pause.Reason != "step_limit" || !paused.Pause.Resumable || !strings.Contains(paused.Pause.Message, "paused at the step budget") {
		t.Fatalf("pause = %+v", paused.Pause)
	}
}
