package agent

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jgabor/aila/internal/runtime"
)

// EventKind names one provider-style stream event accepted by the adapter.
type EventKind string

const (
	EventAssistantDelta EventKind = "assistant_delta"
	EventToolRequest    EventKind = "tool_request"
	EventError          EventKind = "error"
	EventPaused         EventKind = "paused"
	EventCompleted      EventKind = "completed"
)

// Event is a bounded fake provider event for adapter tests.
type Event struct {
	Kind         EventKind
	Provider     string
	Model        string
	Sequence     int
	Text         string
	ToolCallID   string
	ToolName     string
	Arguments    []ToolArgument
	Error        ProviderError
	FinishReason string
}

// ToolArgument records a provider-requested tool argument without executing it.
type ToolArgument struct {
	Name  string
	Value string
}

// ProviderError records a bounded provider-style failure.
type ProviderError struct {
	Code      string
	Message   string
	Retryable bool
}

// FakeStream copies events so tests can model provider streaming deterministically.
func FakeStream(events ...Event) []Event {
	return append([]Event(nil), events...)
}

// AdaptEvents maps fake provider events into runtime messages without provider IO.
func AdaptEvents(operation runtime.OperationMetadata, events []Event) []runtime.Message {
	messages := make([]runtime.Message, 0, len(events))
	for _, event := range events {
		provider := boundedEventText(event.Provider)
		model := boundedEventText(event.Model)
		switch event.Kind {
		case EventAssistantDelta:
			messages = append(messages, runtime.AgentAssistantDelta{Operation: operation, Provider: provider, Model: model, Sequence: event.Sequence, Text: boundedEventText(event.Text)})
		case EventToolRequest:
			messages = append(messages, runtime.AgentToolRequested{Operation: operation, Request: runtime.AgentToolRequest{ID: boundedEventText(event.ToolCallID), Name: boundedEventText(event.ToolName), Arguments: runtimeToolArguments(event.Arguments), Provider: provider, Model: model, Sequence: event.Sequence}})
		case EventError:
			messages = append(messages, runtime.AgentTurnFailed{Operation: operation, Provider: provider, Model: model, Failure: runtime.FailureMetadata{Code: boundedEventText(event.Error.Code), Message: boundedEventText(event.Error.Message), Retryable: event.Error.Retryable}})
		case EventPaused:
			messages = append(messages, runtime.AgentTurnPaused{Operation: operation, Provider: provider, Model: model, Pause: runtime.AgentPauseMetadata{Reason: boundedEventText(event.FinishReason), Message: stepLimitPauseMessage(), Resumable: true, Suggestion: "continue"}})
		case EventCompleted:
			messages = append(messages, runtime.AgentTurnCompleted{Operation: operation, Provider: provider, Model: model, FinishReason: boundedEventText(event.FinishReason)})
		}
	}
	return messages
}

func stepLimitPauseMessage() string {
	return "Agent paused at the step budget. Send a continuation prompt to continue."
}

func runtimeToolArguments(arguments []ToolArgument) []runtime.AgentToolArgument {
	if len(arguments) == 0 {
		return nil
	}
	items := make([]runtime.AgentToolArgument, 0, len(arguments))
	for _, argument := range arguments {
		items = append(items, runtime.AgentToolArgument{Name: boundedEventText(argument.Name), Value: boundedEventText(argument.Value)})
	}
	return items
}

func boundedEventText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	fields := strings.Fields(value)
	for i, field := range fields {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "token=") || strings.Contains(lower, "password=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "api_key=") || strings.HasPrefix(field, "/") || strings.Contains(field, ".aila") || strings.Contains(field, ".agentera") || strings.Contains(field, ".config") {
			fields[i] = "[redacted]"
		}
	}
	value = strings.Join(fields, " ")
	const maxRunes = 240
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes]) + "..."
}
