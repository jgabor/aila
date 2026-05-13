package runtime

import "strings"

// Status describes whether the runtime is waiting for user input or a fake
// operation result.
type Status string

const (
	StatusIdle   Status = "idle"
	StatusActive Status = "active"
)

// Message is an input to the deterministic runtime update function.
type Message interface {
	runtimeMessage()
}

// PromptSubmitted records user prompt text submitted through a presentation
// adapter.
type PromptSubmitted struct {
	Text string
}

func (PromptSubmitted) runtimeMessage() {}

// CommandSelected records an inert command selected through a presentation
// adapter.
type CommandSelected struct {
	Name string
}

func (CommandSelected) runtimeMessage() {}

// FakeEffectCompleted reports a deterministic in-memory fake effect result.
type FakeEffectCompleted struct {
	Operation OperationMetadata
	Result    string
}

func (FakeEffectCompleted) runtimeMessage() {}

// FakeEffectFailed reports deterministic failure metadata for a fake effect.
type FakeEffectFailed struct {
	Operation OperationMetadata
	Failure   FailureMetadata
}

func (FakeEffectFailed) runtimeMessage() {}

// Model is runtime-owned state for the current fake interaction surface.
type Model struct {
	Status        Status
	Transcript    []TranscriptEntry
	Result        string
	LastCommand   string
	NextOperation int
}

// TranscriptEntry is a deterministic surface entry for prompt, command, result,
// and failure messages.
type TranscriptEntry struct {
	Kind string
	Text string
}

// Effect is a typed operation request returned by Update for later execution.
type Effect interface {
	runtimeEffect()
	Metadata() OperationMetadata
}

// FakePromptEffect requests fake in-memory handling for a prompt.
type FakePromptEffect struct {
	Operation OperationMetadata
	Prompt    string
}

func (FakePromptEffect) runtimeEffect() {}

func (effect FakePromptEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// FakeCommandEffect requests fake in-memory handling for a command.
type FakeCommandEffect struct {
	Operation OperationMetadata
	Command   string
}

func (FakeCommandEffect) runtimeEffect() {}

func (effect FakeCommandEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// Dispatch interprets fake in-memory effects synchronously and returns result
// messages in the same order as the input effects.
func Dispatch(effects []Effect) []Message {
	if len(effects) == 0 {
		return nil
	}

	messages := make([]Message, 0, len(effects))
	for _, effect := range effects {
		switch typed := effect.(type) {
		case FakePromptEffect:
			messages = append(messages, FakeEffectCompleted{
				Operation: typed.Operation,
				Result:    "Fake Aila response: " + typed.Prompt,
			})
		case FakeCommandEffect:
			messages = append(messages, FakeEffectCompleted{
				Operation: typed.Operation,
				Result:    "fake command result: " + typed.Command,
			})
		}
	}

	return messages
}

// OperationKind classifies a requested operation without performing it.
type OperationKind string

const (
	OperationPrompt  OperationKind = "prompt"
	OperationCommand OperationKind = "command"
)

// OperationMetadata is inert typed data for future permission and dispatch
// decisions.
type OperationMetadata struct {
	ID      string
	Kind    OperationKind
	Subject string
	Source  string
	Failure FailureMetadata
	Cancel  CancelMetadata
}

// FailureMetadata is inert typed data describing a fake failure.
type FailureMetadata struct {
	Code      string
	Message   string
	Retryable bool
}

// CancelMetadata is inert typed data reserved for future cancellation surfaces.
type CancelMetadata struct {
	Requested bool
	Reason    string
}

// Update applies one runtime message and returns the next model plus typed
// effects for an external interpreter.
func Update(model Model, message Message) (Model, []Effect) {
	next := model
	next.Transcript = append([]TranscriptEntry(nil), model.Transcript...)

	switch msg := message.(type) {
	case PromptSubmitted:
		text := strings.TrimSpace(msg.Text)
		operation := nextOperation(&next, OperationPrompt, text)
		next.Status = StatusActive
		next.Result = ""
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "prompt", Text: text})
		return next, []Effect{FakePromptEffect{Operation: operation, Prompt: text}}
	case CommandSelected:
		operation := nextOperation(&next, OperationCommand, msg.Name)
		next.Status = StatusActive
		next.Result = ""
		next.LastCommand = msg.Name
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "command", Text: msg.Name})
		return next, []Effect{FakeCommandEffect{Operation: operation, Command: msg.Name}}
	case FakeEffectCompleted:
		next.Status = StatusIdle
		next.Result = msg.Result
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "result", Text: msg.Result})
		return next, nil
	case FakeEffectFailed:
		next.Status = StatusIdle
		next.Result = msg.Failure.Message
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "failure", Text: msg.Failure.Message})
		return next, nil
	default:
		return next, nil
	}
}

func nextOperation(model *Model, kind OperationKind, subject string) OperationMetadata {
	model.NextOperation++
	return OperationMetadata{
		ID:      operationID(model.NextOperation),
		Kind:    kind,
		Subject: subject,
		Source:  "user",
	}
}

func operationID(number int) string {
	const prefix = "op-"
	if number <= 0 {
		return prefix + "0"
	}

	digits := make([]byte, 0, 8)
	for number > 0 {
		digits = append(digits, byte('0'+number%10))
		number /= 10
	}

	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}

	return prefix + string(digits)
}
