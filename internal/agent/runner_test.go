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

	stream, err := runner.Stream(context.Background(), RunRequest{Prompt: "inspect", RunID: "run-1", MaxSteps: 4})
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

type readInput struct {
	Path string `json:"path"`
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
