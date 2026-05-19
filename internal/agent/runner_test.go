package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/runtime"
	goagent "github.com/jgabor/go-agent"
)

func TestFakeReadOnlyRunnerStreamsBoundedToolTurn(t *testing.T) {
	t.Parallel()

	runner := DefaultFakeReadOnlyRunner()
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "explain", Provider: "fake", Model: "fake-readonly"})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 4 || events[1].Kind != EventToolRequest || events[1].ToolName != "read" {
		t.Fatalf("events = %#v", events)
	}
	messages := AdaptEvents(runtime.OperationMetadata{ID: "op-1", Kind: runtime.OperationPrompt}, events)
	if _, ok := messages[1].(runtime.AgentToolRequested); !ok {
		t.Fatalf("mapped messages = %#v, want tool request at index 1", messages)
	}
}

func TestFakeRunnersExposeConfiguredStepBudget(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		newRunner func(*int) Runner
	}{
		{
			name: "read-only",
			newRunner: func(observed *int) Runner {
				return FakeReadOnlyRunner{ObserveRequest: func(request RunRequest) {
					*observed = request.MaxSteps
				}}
			},
		},
		{
			name: "build",
			newRunner: func(observed *int) Runner {
				return FakeBuildRunner{ObserveRequest: func(request RunRequest) {
					*observed = request.MaxSteps
				}}
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var observed int
			stream, err := tc.newRunner(&observed).Stream(context.Background(), RunRequest{Prompt: "inspect", MaxSteps: 9})
			if err != nil {
				t.Fatal(err)
			}
			for range stream {
			}
			if observed != 9 {
				t.Fatalf("observed MaxSteps = %d, want 9", observed)
			}
		})
	}
}

func TestFakeBuildRunnerRequestsApprovalBoundWriteForFilePrompts(t *testing.T) {
	t.Parallel()

	runner := DefaultFakeBuildRunner()
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "create a note file from this request", Provider: "fake", Model: "fake-build", ToolNames: []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 4 || events[1].ToolName != "read" || events[3].ToolName != "write" {
		t.Fatalf("events = %#v, want read then write tool requests", events)
	}
	if events[3].ToolCallID != "call-write-1" || toolArgumentValue(events[3].Arguments, "path") != "docs/interactive-build-output.md" || !strings.Contains(toolArgumentValue(events[3].Arguments, "content"), "create a note file") {
		t.Fatalf("write event = %#v", events[3])
	}
}

func TestFakeBuildRunnerKeepsReadOnlyPromptReadOnly(t *testing.T) {
	t.Parallel()

	runner := DefaultFakeBuildRunner()
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "summarize the project", Provider: "fake", Model: "fake-build", ToolNames: []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	var toolNames []string
	for event := range stream {
		if event.Kind == EventToolRequest {
			toolNames = append(toolNames, event.ToolName)
		}
	}
	if !reflect.DeepEqual(toolNames, []string{"read"}) {
		t.Fatalf("tool names = %#v, want read-only turn", toolNames)
	}
}

func TestFakeReadOnlyRunnerStreamsTypedProviderFailures(t *testing.T) {
	t.Parallel()

	for _, failure := range []FailureMode{FailureProviderAuth, FailureProviderTimeout, FailureRateLimited, FailureModelUnavailable} {
		failure := failure
		t.Run(string(failure), func(t *testing.T) {
			t.Parallel()

			stream, err := (FakeReadOnlyRunner{Failure: failure}).Stream(context.Background(), RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			var events []Event
			for event := range stream {
				events = append(events, event)
			}
			if len(events) != 1 || events[0].Kind != EventError || events[0].Error.Code != string(failure) {
				t.Fatalf("events = %#v", events)
			}
		})
	}
}

func toolArgumentValue(arguments []ToolArgument, name string) string {
	for _, argument := range arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func TestGoAgentRunnerMapsRealGoAgentEvents(t *testing.T) {
	t.Parallel()

	readTool, err := goagent.NewTool("read", "Read a workspace file.", func(ctx context.Context, input readInput) (string, error) {
		if input.Path != "README.md" {
			t.Fatalf("tool path = %q, want README.md", input.Path)
		}
		return "Aila is a terminal coding agent.", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []goagent.TurnResult{
		{ToolCalls: []goagent.ToolCall{{ID: "call-read-1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}},
		{Message: goagent.Message{Role: goagent.RoleAssistant, Content: "Read README.md."}, StopReason: goagent.StopComplete},
	}}
	runner, err := NewGoAgentRunner(goagent.ModelFromSimple(model), "goagent", "fake-model", readTool)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect", RunID: "run-1", MaxSteps: 5})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	wantKinds := []EventKind{EventToolRequest, EventAssistantDelta, EventCompleted}
	var gotKinds []EventKind
	for _, event := range events {
		gotKinds = append(gotKinds, event.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("mapped kinds = %#v, want %#v; events=%#v", gotKinds, wantKinds, events)
	}
	if events[0].Arguments[0].Name != "path" || events[0].Arguments[0].Value != "README.md" {
		t.Fatalf("tool args = %#v", events[0].Arguments)
	}
}

func TestGoAgentRunnerHostDispatchModeDoesNotExecuteToolFunctions(t *testing.T) {
	t.Parallel()

	called := 0
	readTool, err := goagent.NewTool("read", "Read a workspace file.", func(context.Context, readInput) (string, error) {
		called++
		return "direct tool IO must not run for capability dispatch", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []goagent.TurnResult{
		{ToolCalls: []goagent.ToolCall{{ID: "call-read-1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}},
		{Message: goagent.Message{Role: goagent.RoleAssistant, Content: "Tool request recorded."}, StopReason: goagent.StopComplete},
	}}
	runner, err := NewGoAgentRunner(goagent.ModelFromSimple(model), "goagent", "fake-model", readTool)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect", RunID: "capability-run", MaxSteps: 5, DispatchToolsThroughHost: true})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if called != 0 {
		t.Fatalf("tool function called %d times, want 0", called)
	}
	if len(events) < 1 || events[0].Kind != EventToolRequest || events[0].ToolName != "read" || toolArgumentValue(events[0].Arguments, "path") != "README.md" {
		t.Fatalf("events = %#v, want mapped tool request without inline execution", events)
	}
	if events[len(events)-1].Kind != EventCompleted {
		t.Fatalf("events = %#v, want capability run to continue after synthetic host-dispatch result", events)
	}
}

func TestGoAgentRunnerPreservesConfiguredStepBudget(t *testing.T) {
	t.Parallel()

	goRunner := &capturingGoAgentRunner{}
	runner := &GoAgentRunner{runner: goRunner, provider: "goagent", model: "fake-model"}

	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect", RunID: "run-budget", MaxSteps: 11})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}

	if goRunner.request.MaxSteps != 11 {
		t.Fatalf("go-agent MaxSteps = %d, want 11", goRunner.request.MaxSteps)
	}
}

func TestGoAgentRunnerPassesSessionContext(t *testing.T) {
	t.Parallel()

	goRunner := &capturingGoAgentRunner{}
	runner := &GoAgentRunner{runner: goRunner, provider: "goagent", model: "fake-model"}
	contextMessages := []ContextMessage{
		{Role: "user", Content: "previous prompt"},
		{Role: "tool", Content: "tool request read call-1"},
		{Role: "assistant", Content: "previous answer"},
	}

	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "continue", SessionID: "interactive-agent", Context: contextMessages})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}

	if goRunner.request.SessionID != "interactive-agent" || goRunner.request.Session.ID != "interactive-agent" {
		t.Fatalf("session ids = request %q session %q", goRunner.request.SessionID, goRunner.request.Session.ID)
	}
	want := []goagent.Message{
		{Role: goagent.RoleUser, Content: "previous prompt"},
		{Role: goagent.RoleAssistant, Content: "Tool evidence: tool request read call-1"},
		{Role: goagent.RoleAssistant, Content: "previous answer"},
	}
	if !reflect.DeepEqual(goRunner.request.Session.Messages, want) {
		t.Fatalf("session messages = %#v, want %#v", goRunner.request.Session.Messages, want)
	}
}

func TestGoAgentRunnerMapsProviderFailures(t *testing.T) {
	t.Parallel()

	runner, err := NewGoAgentRunner(goagent.ModelFromSimple(goagent.SimpleModelFunc(func(context.Context, goagent.TurnRequest) (goagent.TurnResult, error) {
		return goagent.TurnResult{}, errors.New("provider API key missing")
	})), "goagent", "fake-model")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "fail"})
	if err != nil {
		t.Fatal(err)
	}
	var failures []Event
	for event := range stream {
		if event.Kind == EventError {
			failures = append(failures, event)
		}
	}
	if len(failures) == 0 || failures[0].Error.Code != string(FailureProviderAuth) || failures[0].Error.Retryable {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestGoAgentRunnerMapsStepLimitStopAsPauseNotFailure(t *testing.T) {
	t.Parallel()

	events := mapGoAgentEvent(goagent.Event{Kind: goagent.EventStop, StopReason: goagent.StopStepLimit, Sequence: 7}, "goagent", "fake-model")
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Kind != EventPaused || events[0].FinishReason != string(goagent.StopStepLimit) || events[0].Error.Code != "" {
		t.Fatalf("step limit event = %#v, want pause without error", events[0])
	}
}

func TestGoAgentRunnerMapsNonStepLimitStopAsFailure(t *testing.T) {
	t.Parallel()

	events := mapGoAgentEvent(goagent.Event{Kind: goagent.EventStop, StopReason: goagent.StopToolError, Sequence: 7}, "goagent", "fake-model")
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Kind != EventError || events[0].Error.Code != string(goagent.StopToolError) {
		t.Fatalf("tool error event = %#v, want failure", events[0])
	}
}

func TestGoAgentRunnerSendsConfiguredInstructions(t *testing.T) {
	t.Parallel()

	model := &capturingModel{result: goagent.TurnResult{Message: goagent.Message{Role: goagent.RoleAssistant, Content: "ready"}, StopReason: goagent.StopComplete}}
	runner, err := NewGoAgentRunnerWithInstructions(goagent.ModelFromSimple(model), "goagent", "fake-model", "configured system prompt")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if model.instructions != "configured system prompt" {
		t.Fatalf("instructions = %q, want configured system prompt", model.instructions)
	}
}

func TestGoAgentRunnerSendsMinimalInstructionsWhenEmpty(t *testing.T) {
	t.Parallel()

	model := &capturingModel{result: goagent.TurnResult{Message: goagent.Message{Role: goagent.RoleAssistant, Content: "ready"}, StopReason: goagent.StopComplete}}
	runner, err := NewGoAgentRunner(goagent.ModelFromSimple(model), "goagent", "fake-model")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect", Instructions: "   "})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !strings.Contains(model.instructions, "You are Aila") || !strings.Contains(model.instructions, "fixed built-in tools") {
		t.Fatalf("minimal instructions = %q", model.instructions)
	}
}

type readInput struct {
	Path string `json:"path"`
}

type capturingModel struct {
	result       goagent.TurnResult
	instructions string
}

func (model *capturingModel) Turn(_ context.Context, request goagent.TurnRequest) (goagent.TurnResult, error) {
	model.instructions = request.Instructions
	return model.result, nil
}

type capturingGoAgentRunner struct {
	request goagent.RunRequest
}

func (runner *capturingGoAgentRunner) Run(ctx context.Context, request goagent.RunRequest) (goagent.RunResult, error) {
	runner.request = request
	return goagent.RunResult{}, ctx.Err()
}

func (runner *capturingGoAgentRunner) Stream(ctx context.Context, request goagent.RunRequest) (<-chan goagent.Event, error) {
	runner.request = request
	out := make(chan goagent.Event, 1)
	go func() {
		defer close(out)
		if err := ctx.Err(); err != nil {
			out <- goagent.Event{Kind: goagent.EventError, Err: err}
			return
		}
		out <- goagent.Event{Kind: goagent.EventStop, StopReason: goagent.StopComplete}
	}()
	return out, nil
}

type scriptedModel struct {
	turns []goagent.TurnResult
	index int
}

func (model *scriptedModel) Turn(_ context.Context, _ goagent.TurnRequest) (goagent.TurnResult, error) {
	if model.index >= len(model.turns) {
		return goagent.TurnResult{}, errors.New("no scripted turn")
	}
	turn := model.turns[model.index]
	model.index++
	return turn, nil
}
