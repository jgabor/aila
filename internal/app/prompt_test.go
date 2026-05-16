package app

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
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

func TestInterruptRequestRoutesTypedRuntimeMessageWhileFakeWorkActive(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		reason string
	}{
		{name: "ctrl-c", reason: "ctrl-c"},
		{name: "ctrl+x c", reason: "ctrl+x c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var dispatched [][]runtime.Effect
			runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
				dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
				return nil
			})

			active := runner.submitPrompt("active fake work")
			interrupted := runner.requestInterrupt(tc.reason)

			if active.RuntimeStatus != "active" || !active.RuntimeActive {
				t.Fatalf("active handoff = %+v, want active fake work", active)
			}
			if interrupted.RuntimeStatus != "canceling" || !interrupted.RuntimeActive {
				t.Fatalf("interrupt handoff = %+v, want canceling active state", interrupted)
			}
			if interrupted.RuntimeResult != "" || interrupted.QueuedCount != 0 || len(interrupted.QueuedText) != 0 {
				t.Fatalf("interrupt handoff carried unexpected result or queue: %+v", interrupted)
			}
			if len(dispatched) != 2 || len(dispatched[0]) != 1 || len(dispatched[1]) != 1 {
				t.Fatalf("dispatched effects = %#v, want prompt then interrupt effect", dispatched)
			}
			interrupt, ok := dispatched[1][0].(runtime.FakeInterruptEffect)
			if !ok {
				t.Fatalf("second effect = %T, want runtime.FakeInterruptEffect", dispatched[1][0])
			}
			if interrupt.Cancel != (runtime.CancelMetadata{Requested: true, Reason: tc.reason}) {
				t.Fatalf("interrupt cancel metadata = %#v", interrupt.Cancel)
			}
			if runner.model.Status != runtime.StatusCanceling {
				t.Fatalf("runtime status = %q, want canceling", runner.model.Status)
			}
			if got := runner.model.Transcript[len(runner.model.Transcript)-1]; got != (runtime.TranscriptEntry{Kind: "interrupting", Text: tc.reason}) {
				t.Fatalf("last transcript = %#v", got)
			}
		})
	}
}

func TestInterruptRequestHandoffReportsCanceledFromRuntimeState(t *testing.T) {
	t.Parallel()

	var calls int
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		calls++
		if calls == 1 {
			return nil
		}
		return runtime.Dispatch(effects)
	})

	runner.submitPrompt("active fake work")
	canceled := runner.requestInterrupt("ctrl-c")

	if canceled.RuntimeStatus != "canceled" || canceled.RuntimeActive {
		t.Fatalf("canceled handoff = %+v, want canceled inactive state", canceled)
	}
	if canceled.RuntimeResult != "fake work canceled" {
		t.Fatalf("canceled result = %q", canceled.RuntimeResult)
	}
	if runner.model.Status != runtime.StatusCanceled || runner.model.Result != "fake work canceled" {
		t.Fatalf("runtime model = %#v, want runtime-owned canceled result", runner.model)
	}
	if got := runner.model.Transcript[len(runner.model.Transcript)-1]; got != (runtime.TranscriptEntry{Kind: "canceled", Text: "fake work canceled"}) {
		t.Fatalf("last transcript = %#v", got)
	}
}

func TestInterruptRequestHandoffPreservesQueuedIntent(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })
	runner.submitPrompt("active fake work")
	runner.submitPrompt("queued follow-up")

	interrupted := runner.requestInterrupt("ctrl+x c")

	if interrupted.RuntimeStatus != "canceling" || !interrupted.RuntimeActive {
		t.Fatalf("interrupt handoff = %+v, want canceling active state", interrupted)
	}
	if interrupted.QueuedCount != 1 || !reflect.DeepEqual(interrupted.QueuedText, []string{"queued follow-up"}) {
		t.Fatalf("queued interrupt handoff = count %d text %#v", interrupted.QueuedCount, interrupted.QueuedText)
	}
	if got := runner.model.Queued; !reflect.DeepEqual(got, []runtime.QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}}) {
		t.Fatalf("runtime queue = %#v", got)
	}
}

func TestInterruptRequestWhileIdleStaysRuntimeNoop(t *testing.T) {
	t.Parallel()

	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})

	result := runner.requestInterrupt("ctrl-c")

	if len(dispatched) != 1 || len(dispatched[0]) != 0 {
		t.Fatalf("dispatched effects = %#v, want one empty runtime dispatch", dispatched)
	}
	if result.RuntimeStatus != "idle" || result.RuntimeActive || result.RuntimeResult != "" || result.QueuedCount != 0 {
		t.Fatalf("idle interrupt handoff = %+v, want unchanged idle runtime state", result)
	}
	if runner.model.Status != runtime.StatusIdle || len(runner.model.Transcript) != 0 || runner.model.NextOperation != 0 {
		t.Fatalf("runtime model = %#v, want unchanged idle model", runner.model)
	}
}

func TestInputRunnerRecordsEffectPanicAsDiagnosticMetadata(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message {
		panic("fake supervised effect panic")
	})

	result := runner.submitPrompt("panic path")

	if result.RuntimeStatus != "active" || !result.RuntimeActive {
		t.Fatalf("runtime state = %+v, want active state unchanged by diagnostic wrapper", result)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1", len(result.Diagnostics))
	}
	diagnosticView := result.Diagnostics[0]
	if diagnosticView.Source != string(diagnostic.SourceEffect) || diagnosticView.Severity != string(diagnostic.SeverityError) {
		t.Fatalf("diagnostic view = %+v", diagnosticView)
	}
	if diagnosticView.AffectedArtifact != string(diagnostic.ArtifactRuntimeEffect) || diagnosticView.RecoveryAction != string(diagnostic.RecoveryInspect) || !diagnosticView.UserInputNeeded {
		t.Fatalf("diagnostic recovery metadata = %+v", diagnosticView)
	}
	if !strings.Contains(diagnosticView.BoundedMessage, "supervised effect panic recovered") {
		t.Fatalf("diagnostic message = %q, want recovered panic", diagnosticView.BoundedMessage)
	}
	if runner.model.Status != runtime.StatusActive || len(runner.model.Diagnostics) != 1 {
		t.Fatalf("runtime model = %#v, want active with one diagnostic", runner.model)
	}
}

func TestShutdownWhileIdleRecordsSignalDiagnostic(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithContext(context.Background())

	turn := runner.requestShutdown(context.Canceled)

	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive {
		t.Fatalf("shutdown turn = %+v, want idle runtime", turn)
	}
	if len(runner.model.Diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1", len(runner.model.Diagnostics))
	}
	recorded := runner.model.Diagnostics[0]
	if recorded.Category != diagnostic.CategorySignalShutdown || recorded.Source != diagnostic.SourceSignal {
		t.Fatalf("shutdown diagnostic identity = %#v", recorded)
	}
	if recorded.RecoveryAction != diagnostic.RecoveryIgnoreForRun || recorded.UserInputNeeded {
		t.Fatalf("shutdown diagnostic recovery = %#v", recorded)
	}
}

func TestShutdownWhileFakeWorkActiveRecordsCancellationDiagnostic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runner := newInputRunnerHoldingFakeWorkWithContext(ctx)
	runner.submitPrompt("active fake work")
	cancel()

	turn := runner.requestShutdown(ctx.Err())

	if turn.RuntimeStatus != string(runtime.StatusCanceling) || !turn.RuntimeActive {
		t.Fatalf("shutdown turn = %+v, want canceling active runtime", turn)
	}
	if len(runner.model.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want signal shutdown and cancellation", runner.model.Diagnostics)
	}
	if runner.model.Diagnostics[0].Category != diagnostic.CategorySignalShutdown {
		t.Fatalf("first diagnostic = %#v, want signal shutdown", runner.model.Diagnostics[0])
	}
	if runner.model.Diagnostics[1].Category != diagnostic.CategoryCancellation {
		t.Fatalf("second diagnostic = %#v, want cancellation", runner.model.Diagnostics[1])
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
