package app

import (
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestPromptSubmitterRoutesThroughRuntimeUpdateAndDispatch(t *testing.T) {
	t.Parallel()

	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})

	result := runner.submitPrompt("  explain this repo  ")

	want := tui.TranscriptTurn{
		UserText:      "explain this repo",
		AssistantText: "Fake Aila response: explain this repo",
		RuntimeStatus: "idle",
		StatusSource:  "runtime.dispatch",
		StatusDetail:  "fake in-memory runtime loop",
		RuntimeActive: false,
		RuntimeResult: "Fake Aila response: explain this repo",
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("submit result = %+v, want %+v", result, want)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("dispatched effects = %#v, want one runtime effect batch", dispatched)
	}
	effect, ok := dispatched[0][0].(runtime.FakePromptEffect)
	if !ok {
		t.Fatalf("dispatched effect = %T, want runtime.FakePromptEffect", dispatched[0][0])
	}
	if effect.Prompt != "explain this repo" || effect.Metadata().Kind != runtime.OperationPrompt {
		t.Fatalf("prompt effect = %#v", effect)
	}
	wantTranscript := []runtime.TranscriptEntry{
		{Kind: "prompt", Text: "explain this repo"},
		{Kind: "result", Text: "Fake Aila response: explain this repo"},
	}
	if !reflect.DeepEqual(runner.model.Transcript, wantTranscript) {
		t.Fatalf("runtime transcript = %#v, want %#v", runner.model.Transcript, wantTranscript)
	}
	if runner.model.Status != runtime.StatusIdle || runner.model.NextOperation != 1 {
		t.Fatalf("runtime model = %#v, want idle after one operation", runner.model)
	}
}

func TestPromptSubmitWhileRuntimeActiveReturnsQueuedIntent(t *testing.T) {
	t.Parallel()

	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return nil
	})

	active := runner.submitPrompt("first prompt")
	queued := runner.submitPrompt("queued follow-up")

	if active.UserText != "first prompt" || active.AssistantText != "" || active.RuntimeStatus != "active" || !active.RuntimeActive {
		t.Fatalf("active submit result = %+v, want active prompt without assistant response", active)
	}
	if queued.UserText != "" || queued.AssistantText != "" {
		t.Fatalf("queued submit result = %+v, want no immediate transcript response", queued)
	}
	if queued.RuntimeStatus != "active" || !queued.RuntimeActive {
		t.Fatalf("queued runtime status = %+v, want active", queued)
	}
	if queued.QueuedCount != 1 || !reflect.DeepEqual(queued.QueuedText, []string{"queued follow-up"}) {
		t.Fatalf("queued handoff = count %d text %#v, want queued follow-up", queued.QueuedCount, queued.QueuedText)
	}
	if len(dispatched) != 2 || len(dispatched[0]) != 1 || len(dispatched[1]) != 0 {
		t.Fatalf("dispatched effects = %#v, want first prompt effect and queued no-op", dispatched)
	}
	if got := runner.model.Transcript; !reflect.DeepEqual(got, []runtime.TranscriptEntry{{Kind: "prompt", Text: "first prompt"}}) {
		t.Fatalf("runtime transcript = %#v, want only active prompt", got)
	}
	if got := runner.model.Queued; !reflect.DeepEqual(got, []runtime.QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}}) {
		t.Fatalf("runtime queue = %#v", got)
	}
}

func TestPromptSubmitHandoffDistinguishesQueuedAndNonQueuedPaths(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(runtime.Dispatch)

	result := runner.submitPrompt("answer now")

	if result.UserText != "answer now" || result.AssistantText != "Fake Aila response: answer now" {
		t.Fatalf("non-queued submit transcript = %+v", result)
	}
	if result.QueuedCount != 0 || len(result.QueuedText) != 0 {
		t.Fatalf("non-queued submit carried queue = count %d text %#v", result.QueuedCount, result.QueuedText)
	}
	if result.RuntimeStatus != "idle" || result.RuntimeActive {
		t.Fatalf("non-queued runtime state = %+v, want idle", result)
	}
}

func TestAppInputRunnerBoundaryStaysRuntimeAdapterOnly(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "prompt.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse input runner boundary: %v", err)
	}

	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
	for _, forbidden := range []string{
		"context",
		"io",
		"os",
		"os/exec",
		"net/http",
		"github.com/jgabor/aila/internal/agent",
		"github.com/jgabor/aila/internal/capability",
		"github.com/jgabor/aila/internal/permission",
		"github.com/jgabor/aila/internal/state",
		"github.com/jgabor/aila/internal/tools",
		"github.com/jgabor/aila/internal/workflow",
	} {
		if imports[forbidden] {
			t.Fatalf("input runner imports forbidden IO or future-scope package %q", forbidden)
		}
	}

	source, err := os.ReadFile("prompt.go")
	if err != nil {
		t.Fatalf("read prompt boundary: %v", err)
	}
	for _, forbidden := range []string{
		"type PromptHandler interface",
		"type Handler interface",
		"Provider",
		"Adapter",
		"Workflow",
		"Slash",
		"Transition",
		"transition",
		"capability",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("input runner contains future-scope abstraction %q", forbidden)
		}
	}
}

func TestStatusCommandRoutesThroughRuntimeOnly(t *testing.T) {
	t.Parallel()

	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})

	runner.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteStatus, Kind: policy.CommandInputSlash})

	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("dispatched effects = %#v, want one status command effect", dispatched)
	}
	effect, ok := dispatched[0][0].(runtime.FakeCommandEffect)
	if !ok {
		t.Fatalf("dispatched effect = %T, want runtime.FakeCommandEffect", dispatched[0][0])
	}
	if effect.Command != "status" || effect.Metadata().Kind != runtime.OperationCommand {
		t.Fatalf("status effect = %#v", effect)
	}
	if runner.model.LastCommand != "status" || runner.model.Status != runtime.StatusIdle || runner.model.NextOperation != 1 {
		t.Fatalf("runtime model after status = %#v", runner.model)
	}
	if got := runner.model.Transcript; !reflect.DeepEqual(got, []runtime.TranscriptEntry{
		{Kind: "command", Text: "status"},
		{Kind: "result", Text: "fake command result: status"},
	}) {
		t.Fatalf("status transcript = %#v", got)
	}
}

func TestOtherCommandRoutesStayBoundedOutsideRuntime(t *testing.T) {
	t.Parallel()

	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, effects)
		return runtime.Dispatch(effects)
	})

	runner.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteHelp, Kind: policy.CommandInputSlash})
	runner.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteQuit, Kind: policy.CommandInputShortcut})

	if len(dispatched) != 0 {
		t.Fatalf("non-status commands dispatched runtime effects: %#v", dispatched)
	}
	if runner.model.NextOperation != 0 || runner.model.LastCommand != "" || len(runner.model.Transcript) != 0 {
		t.Fatalf("non-status commands changed runtime model: %#v", runner.model)
	}
}
