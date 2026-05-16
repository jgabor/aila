package runtime

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
)

func TestUpdateHandlesPromptDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle}
	firstModel, firstEffects := Update(model, PromptSubmitted{Text: "explain status"})
	secondModel, secondEffects := Update(model, PromptSubmitted{Text: "explain status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for prompt model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for prompt effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}

	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.NextOperation != 1 {
		t.Fatalf("NextOperation = %d, want 1", firstModel.NextOperation)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-1",
		Kind:    OperationPrompt,
		Subject: "explain status",
		Source:  "user",
	})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "prompt", Text: "explain status"}}) {
		t.Fatalf("Transcript = %#v", got)
	}

	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakePromptEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakePromptEffect", firstEffects[0])
	}
	if effect.Prompt != "explain status" {
		t.Fatalf("Prompt = %q", effect.Prompt)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-1",
		Kind:    OperationPrompt,
		Subject: "explain status",
		Source:  "user",
	})
}

func TestUpdateHandlesCommandDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle, NextOperation: 7}
	firstModel, firstEffects := Update(model, CommandSelected{Name: "status"})
	secondModel, secondEffects := Update(model, CommandSelected{Name: "status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for command model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for command effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.LastCommand != "status" {
		t.Fatalf("LastCommand = %q, want status", firstModel.LastCommand)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-8",
		Kind:    OperationCommand,
		Subject: "status",
		Source:  "user",
	})
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakeCommandEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakeCommandEffect", firstEffects[0])
	}
	if effect.Command != "status" {
		t.Fatalf("Command = %q", effect.Command)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-8",
		Kind:    OperationCommand,
		Subject: "status",
		Source:  "user",
	})
}

func TestUpdateHandlesReadToolProposalDeterministically(t *testing.T) {
	t.Parallel()

	request := ReadToolRequest{Path: "docs/notes.md", StartLine: 3, LineLimit: 2, MaxPreviewBytes: 256, Source: ReadSourceMetadata{Caller: "test", RequestID: "read-1"}}
	model := Model{Status: StatusIdle, NextOperation: 4}
	firstModel, firstEffects := Update(model, ReadToolProposed{Request: request})
	secondModel, secondEffects := Update(model, ReadToolProposed{Request: request})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for read proposal model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for read proposal effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-5",
		Kind:    OperationRead,
		Subject: "docs/notes.md",
		Source:  "user",
	})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "tool", Text: "read docs/notes.md"}}) {
		t.Fatalf("Transcript = %#v", got)
	}
	if !reflect.DeepEqual(firstModel.ActiveRead, request) {
		t.Fatalf("ActiveRead = %#v, want %#v", firstModel.ActiveRead, request)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(ReadToolEffect)
	if !ok {
		t.Fatalf("effect type = %T, want ReadToolEffect", firstEffects[0])
	}
	if !reflect.DeepEqual(effect.Request, request) {
		t.Fatalf("read request = %#v, want %#v", effect.Request, request)
	}
	assertOperationMetadata(t, effect.Metadata(), firstModel.ActiveOperation)
}

func TestUpdateHandlesFakeResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "hello", Source: "user"}
	model := Model{
		Status:        StatusActive,
		NextOperation: 3,
		Transcript:    []TranscriptEntry{{Kind: "prompt", Text: "hello"}},
	}

	completed, effects := Update(model, FakeEffectCompleted{Operation: operation, Result: "fake answer"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", completed.Status, StatusIdle)
	}
	if completed.Result != "fake answer" {
		t.Fatalf("Result = %q, want fake answer", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "fake answer"}) {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := FailureMetadata{Code: "fake_failed", Message: "fake failure", Retryable: true}
	failed, effects := Update(model, FakeEffectFailed{Operation: operation, Failure: failure})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", failed.Status, StatusIdle)
	}
	if failed.Result != "fake failure" {
		t.Fatalf("Result = %q, want fake failure", failed.Result)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got != (TranscriptEntry{Kind: "failure", Text: "fake failure"}) {
		t.Fatalf("last transcript = %#v", got)
	}
}

func TestUpdateHandlesReadToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationRead, Subject: "notes.txt", Source: "user"}
	model := Model{
		Status:        StatusActive,
		NextOperation: 3,
		Transcript:    []TranscriptEntry{{Kind: "tool", Text: "read notes.txt"}},
	}
	result := ReadToolResult{
		ToolName:              "read",
		RequestedPath:         "notes.txt",
		WorkspaceRelativePath: "notes.txt",
		EffectiveRange:        ReadLineRange{StartLine: 2, EndLine: 3, Limit: 2},
		PreviewText:           "2: beta\n3: gamma\n",
		Error:                 ReadToolError{Kind: ReadToolErrorNone},
	}

	completed, effects := Update(model, ReadToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", completed.Status, StatusIdle)
	}
	if completed.LastRead != result {
		t.Fatalf("LastRead = %#v, want %#v", completed.LastRead, result)
	}
	if completed.ActiveRead != (ReadToolRequest{}) {
		t.Fatalf("ActiveRead = %#v, want cleared after read completion", completed.ActiveRead)
	}
	if !strings.Contains(completed.Result, "read notes.txt:2-3") || !strings.Contains(completed.Result, "2: beta") {
		t.Fatalf("Result = %q, want bounded read summary with line refs", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got.Kind != "result" || got.Text != completed.Result {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := result
	failure.Error = ReadToolError{Kind: ReadToolErrorMissingFile, Message: "file does not exist"}
	failure.PreviewText = ""
	failed, effects := Update(model, ReadToolCompleted{Operation: operation, Result: failure})
	if len(effects) != 0 {
		t.Fatalf("failed len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("failed Status = %q, want %q", failed.Status, StatusIdle)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got.Kind != "failure" || !strings.Contains(got.Text, "missing_file") {
		t.Fatalf("failure transcript = %#v", got)
	}
}

func TestUpdateQueuesPromptWhileFakeWorkIsActive(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, PromptSubmitted{Text: "queued follow-up"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if updated.NextOperation != model.NextOperation {
		t.Fatalf("NextOperation = %d, want %d", updated.NextOperation, model.NextOperation)
	}
	if got, want := updated.Transcript, model.Transcript; !reflect.DeepEqual(got, want) {
		t.Fatalf("Transcript = %#v, want active transcript unchanged %#v", got, want)
	}
	if got, want := updated.ActiveOperation, operation; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveOperation = %#v, want %#v", got, want)
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
}

func TestUpdateRecordsInterruptingForActiveFakeWork(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Queued:          []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "user pressed interrupt"})
	if updated.Status != StatusCanceling {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusCanceling)
	}
	if !reflect.DeepEqual(updated.Queued, model.Queued) {
		t.Fatalf("Queued = %#v, want %#v", updated.Queued, model.Queued)
	}
	if got := updated.Transcript[len(updated.Transcript)-1]; got != (TranscriptEntry{Kind: "interrupting", Text: "user pressed interrupt"}) {
		t.Fatalf("last transcript = %#v", got)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(FakeInterruptEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakeInterruptEffect", effects[0])
	}
	wantCancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	wantOperation := operation
	wantOperation.Cancel = wantCancel
	assertOperationMetadata(t, updated.ActiveOperation, wantOperation)
	assertOperationMetadata(t, effect.Metadata(), wantOperation)
	if effect.Cancel != wantCancel {
		t.Fatalf("Cancel = %#v, want %#v", effect.Cancel, wantCancel)
	}
}

func TestUpdateDoesNotFakeCancelActiveReadWork(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationRead, Subject: "notes.txt", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "tool", Text: "read notes.txt"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "ctrl-c"})

	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want no fake interrupt effect for read work", len(effects))
	}
	if !reflect.DeepEqual(updated, model) {
		t.Fatalf("updated model = %#v, want active read unchanged %#v", updated, model)
	}
}

func TestUpdateQueuesReadProposalWhileWorkIsActive(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationRead, Subject: "active.txt", Source: "user"}
	model := Model{
		Status:          StatusActive,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "tool", Text: "read active.txt"}},
	}

	updated, effects := Update(model, ReadToolProposed{Request: ReadToolRequest{Path: "queued.txt"}})

	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want queued read no-op", len(effects))
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "read", Text: "queued.txt"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(updated.Transcript, model.Transcript) || updated.ActiveOperation != operation {
		t.Fatalf("active read mutated unexpectedly: %#v", updated)
	}
}

func TestUpdateRecordsCanceledOutcomeFromFakeInterruptResolution(t *testing.T) {
	t.Parallel()

	cancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user", Cancel: cancel}
	model := Model{
		Status:          StatusCanceling,
		ActiveOperation: operation,
		Queued:          []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript: []TranscriptEntry{
			{Kind: "prompt", Text: "active work"},
			{Kind: "interrupting", Text: "user pressed interrupt"},
		},
	}

	updated, effects := Update(model, FakeInterruptResolved{Operation: operation, Cancel: cancel})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusCanceled {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusCanceled)
	}
	if updated.Result != "fake work canceled" {
		t.Fatalf("Result = %q, want fake work canceled", updated.Result)
	}
	if !reflect.DeepEqual(updated.Queued, model.Queued) {
		t.Fatalf("Queued = %#v, want %#v", updated.Queued, model.Queued)
	}
	if got := updated.Transcript[len(updated.Transcript)-1]; got != (TranscriptEntry{Kind: "canceled", Text: "fake work canceled"}) {
		t.Fatalf("last transcript = %#v", got)
	}
	assertOperationMetadata(t, updated.ActiveOperation, operation)
}

func TestUpdateIgnoresInterruptWhenNoFakeWorkIsActive(t *testing.T) {
	t.Parallel()

	model := Model{
		Status:     StatusIdle,
		Result:     "previous result",
		Queued:     []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript: []TranscriptEntry{{Kind: "result", Text: "previous result"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "user pressed interrupt"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if !reflect.DeepEqual(updated, model) {
		t.Fatalf("updated model = %#v, want unchanged %#v", updated, model)
	}
}

func TestUpdateQueuesOrdinaryPromptInsteadOfTreatingItAsInterrupt(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, PromptSubmitted{Text: "please stop after this"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "prompt", Text: "please stop after this"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(updated.Transcript, model.Transcript) {
		t.Fatalf("Transcript = %#v, want unchanged %#v", updated.Transcript, model.Transcript)
	}
	assertOperationMetadata(t, updated.ActiveOperation, operation)
}

func TestUpdatePreservesQueuedPromptSubmissionOrder(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusActive}
	model, effects := Update(model, PromptSubmitted{Text: "first queued"})
	if len(effects) != 0 {
		t.Fatalf("first len(effects) = %d, want 0", len(effects))
	}
	model, effects = Update(model, PromptSubmitted{Text: "second queued"})
	if len(effects) != 0 {
		t.Fatalf("second len(effects) = %d, want 0", len(effects))
	}
	model, effects = Update(model, PromptSubmitted{Text: "third queued"})
	if len(effects) != 0 {
		t.Fatalf("third len(effects) = %d, want 0", len(effects))
	}

	want := []QueuedEntry{
		{Kind: "prompt", Text: "first queued"},
		{Kind: "prompt", Text: "second queued"},
		{Kind: "prompt", Text: "third queued"},
	}
	if !reflect.DeepEqual(model.Queued, want) {
		t.Fatalf("Queued = %#v, want %#v", model.Queued, want)
	}
}

func TestUpdateKeepsQueuedPromptsVisibleAfterFakeWorkCompletes(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status: StatusActive,
		Queued: []QueuedEntry{
			{Kind: "prompt", Text: "first queued"},
			{Kind: "prompt", Text: "second queued"},
		},
		Transcript: []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	completed, effects := Update(model, FakeEffectCompleted{Operation: operation, Result: "done"})
	if len(effects) != 0 {
		t.Fatalf("completed len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("completed Status = %q, want %q", completed.Status, StatusIdle)
	}
	if !reflect.DeepEqual(completed.Queued, model.Queued) {
		t.Fatalf("completed Queued = %#v, want %#v", completed.Queued, model.Queued)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "done"}) {
		t.Fatalf("completed last transcript = %#v", got)
	}

	failure := FailureMetadata{Code: "fake_failed", Message: "failed", Retryable: true}
	failed, effects := Update(model, FakeEffectFailed{Operation: operation, Failure: failure})
	if len(effects) != 0 {
		t.Fatalf("failed len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("failed Status = %q, want %q", failed.Status, StatusIdle)
	}
	if !reflect.DeepEqual(failed.Queued, model.Queued) {
		t.Fatalf("failed Queued = %#v, want %#v", failed.Queued, model.Queued)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got != (TranscriptEntry{Kind: "failure", Text: "failed"}) {
		t.Fatalf("failed last transcript = %#v", got)
	}
}

func TestDispatchHandlesPromptEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "explain status", Source: "user"}
	messages := Dispatch([]Effect{FakePromptEffect{Operation: operation, Prompt: "explain status"}})

	if got, want := messages, []Message{FakeEffectCompleted{Operation: operation, Result: "Fake Aila response: explain status"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchHandlesCommandEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-2", Kind: OperationCommand, Subject: "status", Source: "user"}
	messages := Dispatch([]Effect{FakeCommandEffect{Operation: operation, Command: "status"}})

	if got, want := messages, []Message{FakeEffectCompleted{Operation: operation, Result: "fake command result: status"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchHandlesInterruptEffect(t *testing.T) {
	t.Parallel()

	cancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	operation := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "active work", Source: "user", Cancel: cancel}
	messages := Dispatch([]Effect{FakeInterruptEffect{Operation: operation, Cancel: cancel}})

	if got, want := messages, []Message{FakeInterruptResolved{Operation: operation, Cancel: cancel}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchReturnsMixedResultsInInputOrder(t *testing.T) {
	t.Parallel()

	prompt := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "hello", Source: "user"}
	command := OperationMetadata{ID: "op-4", Kind: OperationCommand, Subject: "status", Source: "user"}
	messages := Dispatch([]Effect{
		FakePromptEffect{Operation: prompt, Prompt: "hello"},
		FakeCommandEffect{Operation: command, Command: "status"},
		FakePromptEffect{Operation: prompt, Prompt: "again"},
	})

	want := []Message{
		FakeEffectCompleted{Operation: prompt, Result: "Fake Aila response: hello"},
		FakeEffectCompleted{Operation: command, Result: "fake command result: status"},
		FakeEffectCompleted{Operation: prompt, Result: "Fake Aila response: again"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", messages, want)
	}
}

func TestDispatchIgnoresUnsupportedEffects(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-5", Kind: OperationPrompt, Subject: "ignored", Source: "user"}
	messages := Dispatch([]Effect{
		unsupportedEffect{operation: operation},
		FakePromptEffect{Operation: operation, Prompt: "kept"},
	})

	want := []Message{FakeEffectCompleted{Operation: operation, Result: "Fake Aila response: kept"}}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", messages, want)
	}
}

func TestDispatchIsDeterministic(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-6", Kind: OperationCommand, Subject: "status", Source: "user"}
	effects := []Effect{FakeCommandEffect{Operation: operation, Command: "status"}}
	first := Dispatch(effects)
	second := Dispatch(effects)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Dispatch is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestDispatchHandlesNoEffects(t *testing.T) {
	t.Parallel()

	if messages := Dispatch(nil); len(messages) != 0 {
		t.Fatalf("len(messages) = %d, want 0", len(messages))
	}
}

func TestDispatchContextRecordsCancellationDiagnosticMessage(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operation := OperationMetadata{ID: "op-7", Kind: OperationPrompt, Subject: "canceled", Source: "user"}

	messages := DispatchContext(ctx, []Effect{FakePromptEffect{Operation: operation, Prompt: "canceled"}})

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	message, ok := messages[0].(RuntimeDiagnostic)
	if !ok {
		t.Fatalf("message = %T, want RuntimeDiagnostic", messages[0])
	}
	if message.Diagnostic.Category != diagnostic.CategoryCancellation || message.Diagnostic.Source != diagnostic.SourceEffect {
		t.Fatalf("diagnostic identity = %#v", message.Diagnostic)
	}
	if !strings.Contains(message.Diagnostic.BoundedMessage, "context canceled") {
		t.Fatalf("diagnostic message = %q, want context cancellation", message.Diagnostic.BoundedMessage)
	}

	model, effects := Update(Model{Status: StatusActive}, message)
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusActive {
		t.Fatalf("status = %q, want active unchanged", model.Status)
	}
	if len(model.Diagnostics) != 1 || model.Diagnostics[0].Category != diagnostic.CategoryCancellation {
		t.Fatalf("model diagnostics = %#v", model.Diagnostics)
	}
}

func TestDispatchRecoversEffectPanicAsDiagnosticMessage(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-8", Kind: OperationPrompt, Subject: "panic", Source: "user"}
	messages := Dispatch([]Effect{panicEffect{operation: operation}})

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	message, ok := messages[0].(RuntimeDiagnostic)
	if !ok {
		t.Fatalf("message = %T, want RuntimeDiagnostic", messages[0])
	}
	if message.Diagnostic.Category != diagnostic.CategoryEffect || message.Diagnostic.Source != diagnostic.SourceEffect {
		t.Fatalf("diagnostic identity = %#v", message.Diagnostic)
	}
	if !strings.Contains(message.Diagnostic.BoundedMessage, "supervised effect panic recovered") {
		t.Fatalf("diagnostic message = %q, want recovered panic", message.Diagnostic.BoundedMessage)
	}

	model, effects := Update(Model{Status: StatusActive}, message)
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusActive {
		t.Fatalf("status = %q, want active unchanged", model.Status)
	}
	if len(model.Diagnostics) != 1 || model.Diagnostics[0].RecoveryAction != diagnostic.RecoveryInspect {
		t.Fatalf("model diagnostics = %#v", model.Diagnostics)
	}
}

func TestUpdateDoesNotMutateInputModel(t *testing.T) {
	t.Parallel()

	model := Model{
		Status:        StatusIdle,
		NextOperation: 2,
		Transcript:    []TranscriptEntry{{Kind: "result", Text: "previous"}},
	}
	original := Model{
		Status:        model.Status,
		NextOperation: model.NextOperation,
		Transcript:    append([]TranscriptEntry(nil), model.Transcript...),
	}

	updated, _ := Update(model, PromptSubmitted{Text: "next"})
	updated.Transcript[0].Text = "mutated copy"

	if !reflect.DeepEqual(model, original) {
		t.Fatalf("input model mutated:\ngot:  %#v\nwant: %#v", model, original)
	}
}

func TestFailureAndCancelMetadataAreInert(t *testing.T) {
	t.Parallel()

	metadata := OperationMetadata{
		ID:      "op-9",
		Kind:    OperationPrompt,
		Subject: "danger?",
		Source:  "user",
		Failure: FailureMetadata{Code: "bounded", Message: "bounded failure", Retryable: false},
		Cancel:  CancelMetadata{Requested: true, Reason: "user requested stop"},
	}

	model, effects := Update(Model{Status: StatusIdle}, FakeEffectFailed{
		Operation: metadata,
		Failure:   metadata.Failure,
	})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", model.Status, StatusIdle)
	}
	if model.Result != metadata.Failure.Message {
		t.Fatalf("Result = %q, want %q", model.Result, metadata.Failure.Message)
	}
}

func TestRuntimeProductionFilesHaveNoForbiddenImportsOrTokens(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]bool{
		"io":            true,
		"net/http":      true,
		"os":            true,
		"os/exec":       true,
		"path/filepath": true,
		"sync":          true,
	}
	forbiddenTokens := []string{
		"go ",
		"http.",
		"os.",
		"exec.",
		"Open(",
		"ReadFile(",
		"WriteFile(",
		"Mkdir",
		"Remove(",
		"Chdir(",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(string(content), token) {
				t.Fatalf("%s contains forbidden token %q", file, token)
			}
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if forbiddenImports[path] {
				t.Fatalf("%s imports forbidden package %q", file, path)
			}
		}
	}
}

func assertOperationMetadata(t *testing.T, got OperationMetadata, want OperationMetadata) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
}

type unsupportedEffect struct {
	operation OperationMetadata
}

func (unsupportedEffect) runtimeEffect() {}

func (effect unsupportedEffect) Metadata() OperationMetadata {
	return effect.operation
}

type panicEffect struct {
	operation OperationMetadata
}

func (panicEffect) runtimeEffect() {}

func (effect panicEffect) Metadata() OperationMetadata {
	return effect.operation
}

func (panicEffect) dispatchPanic() {
	panic("fake effect worker panic")
}
