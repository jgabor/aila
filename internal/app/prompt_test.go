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
	if result != want {
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
