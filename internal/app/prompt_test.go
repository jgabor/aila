package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
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

func TestReadOnlyAgentPromptRoutesToolThroughPermissionEffects(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nAila is a testable terminal coding agent.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := newInputRunnerWithDispatchAndAgent(t.Context(), readDispatchContext(t.Context(), workspace, string(permission.AutonomyRead)), agent.DefaultFakeReadOnlyRunner())

	turn := runner.submitPrompt("build a summary")

	if turn.UserText != "build a summary" || !strings.Contains(turn.AssistantText, "terminal coding agent") {
		t.Fatalf("agent turn transcript = %+v", turn)
	}
	if turn.AssistantSource != "fake" || turn.AssistantModel != "fake-readonly" || turn.Phase != workflow.PhaseBuild.DisplayLabel() || turn.PhaseSource != workflow.PhaseBuild.String() || turn.RuntimeStatus != "idle" || turn.RuntimeActive {
		t.Fatalf("agent turn runtime = %+v", turn)
	}
	if turn.Read == nil || turn.Read.Status != "completed" || !turn.Read.ReadOnly || turn.Read.Path != "README.md" || len(turn.Read.PreviewLines) == 0 {
		t.Fatalf("agent read view = %+v", turn.Read)
	}
	if turn.Read.Decision == nil || !turn.Read.Decision.Allowed || turn.Read.Decision.OperationKind != "read" {
		t.Fatalf("agent read decision = %+v", turn.Read.Decision)
	}
}

func TestReadOnlyAgentProviderFailuresBecomeTypedDiagnostics(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatchAndAgent(t.Context(), runtime.Dispatch, agent.FakeReadOnlyRunner{Failure: agent.FailureProviderAuth})

	turn := runner.submitPrompt("fail auth")

	if turn.AssistantText != "provider authentication failed" || turn.RuntimeResult != "provider authentication failed" || turn.Phase != workflow.PhaseBuild.DisplayLabel() {
		t.Fatalf("provider failure turn = %+v", turn)
	}
	if len(turn.Diagnostics) != 1 || turn.Diagnostics[0].Source != string(diagnostic.SourceProvider) || !strings.Contains(turn.Diagnostics[0].BoundedMessage, "provider_auth_failed") || !turn.Diagnostics[0].UserInputNeeded {
		t.Fatalf("provider diagnostics = %+v", turn.Diagnostics)
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

func TestReadToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "read")

	turn := runner.proposeReadTool(runtime.ReadToolRequest{Path: "notes.txt", StartLine: 2, LineLimit: 2, Source: runtime.ReadSourceMetadata{Caller: "test", RequestID: "read-1"}})

	if turn.UserText != "" || !strings.Contains(turn.AssistantText, "read notes.txt:2-3") || !strings.Contains(turn.AssistantText, "2: beta") {
		t.Fatalf("read turn = %+v, want completed read result with exact path and lines", turn)
	}
	if turn.Read == nil || turn.Read.Status != "completed" || !turn.Read.ReadOnly || turn.Read.Path != "notes.txt" || turn.Read.EffectiveRange.StartLine != 2 || turn.Read.EffectiveRange.EndLine != 3 || !reflect.DeepEqual(turn.Read.PreviewLines, []string{"2: beta", "3: gamma"}) {
		t.Fatalf("read view = %+v, want completed read presentation state", turn.Read)
	}
	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive {
		t.Fatalf("read runtime state = %+v, want idle after explicit read effect", turn)
	}
	if got := runner.model.LastRead; got.ToolName != "read" || got.WorkspaceRelativePath != "notes.txt" || got.EffectiveRange.StartLine != 2 || got.EffectiveRange.EndLine != 3 || got.Error.Kind != runtime.ReadToolErrorNone {
		t.Fatalf("last read = %#v, want successful read result", got)
	}
	assertAllowedToolDecision(t, runner.model.LastRead.Decision, "read", "notes.txt")
	assertViewDecision(t, turn.Read.Decision, "allowed", "read", "notes.txt")
	if _, err := os.Stat(filepath.Join(workspace, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("read tool created durable state err=%v", err)
	}
}

func TestReadToolProposalCanSurfaceRunningReadPresentation(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })

	turn := runner.proposeReadTool(runtime.ReadToolRequest{Path: "notes.txt", StartLine: 4, LineLimit: 6})

	if turn.Read == nil || turn.Read.Status != "running" || !turn.Read.ReadOnly || turn.Read.Path != "notes.txt" || turn.Read.RequestedRange.StartLine != 4 || turn.Read.RequestedRange.Limit != 6 {
		t.Fatalf("running read view = %+v, want active injected read presentation state", turn.Read)
	}
	if turn.RuntimeStatus != string(runtime.StatusActive) || !turn.RuntimeActive || turn.StatusDetail != "read tool dispatch" {
		t.Fatalf("running read runtime turn = %+v, want active read dispatch detail", turn)
	}
}

func TestReadToolProposalSurfacesValidationFailureWithoutHiddenRetry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "read")

	turn := runner.proposeReadTool(runtime.ReadToolRequest{Path: "../secret.txt"})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("read failure runtime state = turn %+v model %#v, want idle without retry", turn, runner.model)
	}
	if got := runner.model.LastRead.Error; got.Kind != runtime.ReadToolErrorInvalidPath || !strings.Contains(got.Message, "path traversal") {
		t.Fatalf("last read error = %#v, want bounded invalid path failure", got)
	}
	if strings.Contains(turn.AssistantText, "../secret.txt") || strings.Contains(turn.AssistantText, workspace) {
		t.Fatalf("read failure leaked unsafe path context: %q", turn.AssistantText)
	}
	if got := len(runner.model.Transcript); got != 2 {
		t.Fatalf("transcript entries = %d, want proposal plus one result without hidden retry", got)
	}
}

func TestReadToolProposalSurfacesExecutionFailureWithoutWorkflowMutation(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "read")

	turn := runner.proposeReadTool(runtime.ReadToolRequest{Path: "missing.txt"})

	if runner.model.Status != runtime.StatusIdle || turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive {
		t.Fatalf("missing read runtime state = turn %+v model %#v, want idle result", turn, runner.model)
	}
	if got := runner.model.LastRead.Error; got.Kind != runtime.ReadToolErrorMissingFile || !strings.Contains(got.Message, "file does not exist") {
		t.Fatalf("missing read error = %#v, want missing file", got)
	}
	if !strings.Contains(turn.AssistantText, "read missing.txt failed: missing_file") {
		t.Fatalf("missing read assistant = %q, want bounded failure result", turn.AssistantText)
	}
	if len(runner.model.Diagnostics) != 0 {
		t.Fatalf("missing read diagnostics = %#v, want no workflow or recovery mutation", runner.model.Diagnostics)
	}
}

func TestReadToolProposalDeniedWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "off")

	turn := runner.proposeReadTool(runtime.ReadToolRequest{Path: "notes.txt"})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("denied read runtime state = turn %+v model %#v", turn, runner.model)
	}
	if runner.model.LastRead.Error.Kind != runtime.ReadToolErrorPermission || !strings.Contains(turn.AssistantText, "autonomy off") {
		t.Fatalf("denied read result = %#v assistant=%q", runner.model.LastRead, turn.AssistantText)
	}
	assertDeniedToolDecision(t, runner.model.LastRead.Decision, "read", "notes.txt")
	assertViewDecision(t, turn.Read.Decision, "denied", "read", "notes.txt")
}

func TestSearchToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "app.go"), []byte("alpha\nneedle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "read")

	turn := runner.proposeSearchTool(runtime.SearchToolRequest{ToolName: runtime.SearchToolGrep, Query: "needle", IncludePattern: "**/*.go", Source: runtime.SearchSourceMetadata{Caller: "test", RequestID: "grep-1"}})

	if turn.UserText != "" || !strings.Contains(turn.AssistantText, "grep needle in **/*.go: 1 matches") || !strings.Contains(turn.AssistantText, "src/app.go:2: needle here") {
		t.Fatalf("search turn = %+v, want completed grep result with exact path and line", turn)
	}
	if turn.Search == nil || turn.Search.Status != "completed" || !turn.Search.ReadOnly || turn.Search.Name != "grep" || turn.Search.Query != "needle" || len(turn.Search.Matches) != 1 || turn.Search.Matches[0].Path != "src/app.go" || turn.Search.Matches[0].LineNumber != 2 {
		t.Fatalf("search view = %+v, want completed search presentation state", turn.Search)
	}
	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive {
		t.Fatalf("search runtime state = %+v, want idle after explicit search effect", turn)
	}
	if got := runner.model.LastSearch; got.ToolName != "grep" || len(got.Matches) != 1 || got.Error.Kind != runtime.SearchToolErrorNone {
		t.Fatalf("last search = %#v, want successful search result", got)
	}
	assertAllowedToolDecision(t, runner.model.LastSearch.Decision, "grep", "needle in **/*.go")
	assertViewDecision(t, turn.Search.Decision, "allowed", "grep", "needle in **/*.go")
	if _, err := os.Stat(filepath.Join(workspace, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("search tool created durable state err=%v", err)
	}
}

func TestSearchToolProposalCanSurfaceRunningSearchPresentation(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })

	turn := runner.proposeSearchTool(runtime.SearchToolRequest{ToolName: runtime.SearchToolFind, Pattern: "**/*.go", MaxResults: 10})

	if turn.Search == nil || turn.Search.Status != "running" || !turn.Search.ReadOnly || turn.Search.Name != "find" || turn.Search.Pattern != "**/*.go" || turn.Search.OmittedResults != 0 || len(turn.Search.Matches) != 0 {
		t.Fatalf("running search view = %+v, want active injected search presentation state", turn.Search)
	}
	if turn.RuntimeStatus != string(runtime.StatusActive) || !turn.RuntimeActive || turn.StatusDetail != "search tool dispatch" {
		t.Fatalf("running search runtime turn = %+v, want active search dispatch detail", turn)
	}
}

func TestSearchToolProposalSurfacesValidationFailureWithoutHiddenRetry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "read")

	turn := runner.proposeSearchTool(runtime.SearchToolRequest{ToolName: runtime.SearchToolGrep, Query: "[", Regex: true})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("search failure runtime state = turn %+v model %#v, want idle without retry", turn, runner.model)
	}
	if got := runner.model.LastSearch.Error; got.Kind != runtime.SearchToolErrorInvalidQuery || !strings.Contains(got.Message, "regex") {
		t.Fatalf("last search error = %#v, want bounded invalid query failure", got)
	}
	if strings.Contains(turn.AssistantText, workspace) {
		t.Fatalf("search failure leaked workspace path: %q", turn.AssistantText)
	}
	if got := len(runner.model.Transcript); got != 2 {
		t.Fatalf("transcript entries = %d, want proposal plus one result without hidden retry", got)
	}
}

func TestSearchToolProposalDeniedWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(context.Background(), workspace, "off")

	turn := runner.proposeSearchTool(runtime.SearchToolRequest{ToolName: runtime.SearchToolFind, Pattern: "*.go"})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("denied search runtime state = turn %+v model %#v", turn, runner.model)
	}
	if runner.model.LastSearch.Error.Kind != runtime.SearchToolErrorPermission || !strings.Contains(turn.AssistantText, "autonomy off") {
		t.Fatalf("denied search result = %#v assistant=%q", runner.model.LastSearch, turn.AssistantText)
	}
	assertDeniedToolDecision(t, runner.model.LastSearch.Decision, "find", "*.go")
	assertViewDecision(t, turn.Search.Decision, "denied", "find", "*.go")
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

func TestBashToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "alpha\n")
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))

	turn := runner.proposeBashTool(runtime.BashToolRequest{Argv: []string{"ls", "-1"}, WorkingDir: ".", Source: runtime.BashSourceMetadata{Caller: "test", RequestID: "bash-1"}})
	if turn.Command == nil || turn.Command.Status != "completed" || !turn.Command.ReadOnly || turn.Command.Name != "bash" || turn.Command.CommandFamily != "ls" {
		t.Fatalf("command view = %+v, want completed bash presentation state", turn.Command)
	}
	if !containsAnyString(turn.Command.StdoutLines, "notes.txt") {
		t.Fatalf("stdout lines = %v, want notes.txt", turn.Command.StdoutLines)
	}
	if got := runner.model.LastBash; got.ToolName != "bash" || got.CommandFamily != "ls" || got.Error.Kind != runtime.BashToolErrorNone {
		t.Fatalf("last bash = %+v", got)
	}
	assertAllowedToolDecision(t, runner.model.LastBash.Decision, "bash", "")
	assertViewDecision(t, turn.Command.Decision, "allowed", "bash", "")
}

func TestBashToolProposalCanSurfaceRunningPresentation(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })
	turn := runner.proposeBashTool(runtime.BashToolRequest{Argv: []string{"git", "status", "--short"}, WorkingDir: "."})
	if turn.Command == nil || turn.Command.Status != "running" || !turn.Command.ReadOnly || turn.Command.Name != "bash" || turn.Command.Argv[0] != "git" {
		t.Fatalf("running command view = %+v, want active injected command presentation state", turn.Command)
	}
	if turn.Command.CommandFamily != "" || turn.Command.ExitCode != 0 || len(turn.Command.StdoutLines) != 0 {
		t.Fatalf("running command view looks completed: %+v", turn.Command)
	}
}

func TestBashToolProposalSurfacesValidationFailureWithoutHiddenRetry(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	turn := runner.proposeBashTool(runtime.BashToolRequest{Argv: []string{"git", "checkout", "main"}})
	if turn.Command == nil || turn.Command.Status != "failed" || turn.Command.ErrorKind != string(runtime.BashToolErrorUnsafeCommand) {
		t.Fatalf("validation failure command view = %+v", turn.Command)
	}
	if got := runner.model.LastBash.Error; got.Kind != runtime.BashToolErrorUnsafeCommand || !strings.Contains(got.Message, "git subcommand") {
		t.Fatalf("last bash error = %+v", got)
	}
}

func TestBashToolProposalDeniedWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyOff))
	turn := runner.proposeBashTool(runtime.BashToolRequest{Argv: []string{"pwd"}})
	if turn.Command == nil || turn.Command.ErrorKind != string(runtime.BashToolErrorPermission) || !strings.Contains(turn.AssistantText, "autonomy off") {
		t.Fatalf("denied command view = %+v assistant=%q", turn.Command, turn.AssistantText)
	}
	assertDeniedToolDecision(t, runner.model.LastBash.Decision, "bash", "")
	assertViewDecision(t, turn.Command.Decision, "denied", "bash", "")
}

func assertAllowedToolDecision(t *testing.T, decision runtime.ToolDecision, tool string, target string) {
	t.Helper()
	if !decision.Present || decision.Autonomy != string(permission.AutonomyRead) || decision.Source != "autonomy_policy" || !decision.Allowed || !decision.Automatic || decision.ApprovalRequired || decision.OperationKind != string(permission.OperationRead) || decision.Tool != tool || decision.ExpectedEffect == "" || !decision.Reversible {
		t.Fatalf("allowed decision = %+v, want read-only allow for %s", decision, tool)
	}
	if target != "" && decision.Target != target {
		t.Fatalf("allowed decision target = %q, want %q", decision.Target, target)
	}
}

func assertDeniedToolDecision(t *testing.T, decision runtime.ToolDecision, tool string, target string) {
	t.Helper()
	if !decision.Present || decision.Autonomy != string(permission.AutonomyOff) || decision.Source != "autonomy_policy" || decision.Allowed || decision.Automatic || !decision.ApprovalRequired || decision.OperationKind != string(permission.OperationRead) || decision.Tool != tool || !strings.Contains(decision.Reason, "autonomy off") {
		t.Fatalf("denied decision = %+v, want read-only denial for %s", decision, tool)
	}
	if target != "" && decision.Target != target {
		t.Fatalf("denied decision target = %q, want %q", decision.Target, target)
	}
}

func assertViewDecision(t *testing.T, decision *tui.DecisionView, want string, name string, target string) {
	t.Helper()
	if decision == nil || decision.Source != "autonomy_policy" || decision.OperationKind != string(permission.OperationRead) || decision.Name != name || decision.ExpectedEffect == "" || !decision.Reversible {
		t.Fatalf("view decision = %+v, want %s decision for %s", decision, want, name)
	}
	if want == "allowed" && (!decision.Allowed || !decision.Automatic || decision.ApprovalRequired) {
		t.Fatalf("view allowed decision = %+v", decision)
	}
	if want == "denied" && (decision.Allowed || decision.Automatic || !decision.ApprovalRequired || !strings.Contains(decision.Reason, "autonomy off")) {
		t.Fatalf("view denied decision = %+v", decision)
	}
	if target != "" && decision.Target != target {
		t.Fatalf("view decision target = %q, want %q", decision.Target, target)
	}
}

func containsAnyString(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func writeAppTestFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func readAppTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func appTestFileVersion(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type appFakeFetchClient struct {
	response *http.Response
	err      error
}

func (client appFakeFetchClient) Do(*http.Request) (*http.Response, error) {
	return client.response, client.err
}

func TestFetchToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	client := appFakeFetchClient{response: &http.Response{
		StatusCode:    200,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": []string{"text/plain"}},
		Body:          io.NopCloser(strings.NewReader("hello from docs")),
		ContentLength: int64(len("hello from docs")),
	}}
	runner := newInputRunnerWithReadContextAndFetchClient(context.Background(), workspace, "read", client)

	turn := runner.proposeFetchTool(runtime.FetchToolRequest{URL: "https://example.com/docs", MaxPreviewBytes: 64, Source: runtime.FetchSourceMetadata{Caller: "test", RequestID: "fetch-1"}})

	if turn.UserText != "" || !strings.Contains(turn.AssistantText, "fetch https://example.com/docs: completed 200") {
		t.Fatalf("fetch turn = %+v, want completed fetch result", turn)
	}
	if turn.Fetch == nil || turn.Fetch.Status != "completed" || !turn.Fetch.ReadOnly || turn.Fetch.URL != "https://example.com/docs" || turn.Fetch.HTTPStatusCode != 200 || !reflect.DeepEqual(turn.Fetch.PreviewLines, []string{"hello from docs"}) {
		t.Fatalf("fetch view = %+v, want completed fetch presentation state", turn.Fetch)
	}
	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive || turn.StatusDetail != "fetch tool dispatch" {
		t.Fatalf("fetch runtime state = %+v, want idle after explicit fetch effect", turn)
	}
	if got := runner.model.LastFetch; got.ToolName != "fetch" || got.EffectiveURL != "https://example.com/docs" || got.Error.Kind != runtime.FetchToolErrorNone {
		t.Fatalf("last fetch = %#v, want successful fetch result", got)
	}
	assertAllowedToolDecision(t, runner.model.LastFetch.Decision, "fetch", "https://example.com/docs")
	assertViewDecision(t, turn.Fetch.Decision, "allowed", "fetch", "https://example.com/docs")
	if _, err := os.Stat(filepath.Join(workspace, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("fetch tool created durable state err=%v", err)
	}
}

func TestFetchToolProposalCanSurfaceRunningPresentation(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })

	turn := runner.proposeFetchTool(runtime.FetchToolRequest{URL: "https://example.com/docs", Method: "GET"})

	if turn.Fetch == nil || turn.Fetch.Status != "running" || !turn.Fetch.ReadOnly || turn.Fetch.URL != "https://example.com/docs" || turn.Fetch.Method != "GET" {
		t.Fatalf("running fetch view = %+v, want active injected fetch presentation state", turn.Fetch)
	}
	if turn.RuntimeStatus != string(runtime.StatusActive) || !turn.RuntimeActive || turn.StatusDetail != "fetch tool dispatch" {
		t.Fatalf("running fetch runtime turn = %+v, want active fetch dispatch detail", turn)
	}
}

func TestFetchToolProposalSurfacesValidationFailureWithoutHiddenRetry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContextAndFetchClient(context.Background(), workspace, "read", appFakeFetchClient{})

	turn := runner.proposeFetchTool(runtime.FetchToolRequest{URL: "file:///etc/passwd"})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || turn.RuntimeActive || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("fetch failure runtime state = turn %+v model %#v, want idle without retry", turn, runner.model)
	}
	if got := runner.model.LastFetch.Error; got.Kind != runtime.FetchToolErrorInvalidURL {
		t.Fatalf("last fetch error = %#v, want invalid url failure", got)
	}
	if runner.model.LastFetch.Decision.Present || turn.Fetch.Decision != nil {
		t.Fatalf("invalid fetch should not fabricate decision metadata: last=%+v view=%+v", runner.model.LastFetch.Decision, turn.Fetch.Decision)
	}
	if strings.Contains(turn.AssistantText, "/etc/passwd") || strings.Contains(turn.AssistantText, workspace) {
		t.Fatalf("fetch failure leaked unsafe path context: %q", turn.AssistantText)
	}
	if got := len(runner.model.Transcript); got != 2 {
		t.Fatalf("transcript entries = %d, want proposal plus one result without hidden retry", got)
	}
}

func TestFetchToolProposalDeniedWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContextAndFetchClient(context.Background(), workspace, "off", appFakeFetchClient{})

	turn := runner.proposeFetchTool(runtime.FetchToolRequest{URL: "https://example.com/docs"})

	if turn.RuntimeStatus != string(runtime.StatusIdle) || runner.model.Status != runtime.StatusIdle {
		t.Fatalf("denied fetch runtime state = turn %+v model %#v", turn, runner.model)
	}
	if runner.model.LastFetch.Error.Kind != runtime.FetchToolErrorPermission || !strings.Contains(turn.AssistantText, "autonomy off") {
		t.Fatalf("denied fetch result = %#v assistant=%q", runner.model.LastFetch, turn.AssistantText)
	}
	assertDeniedToolDecision(t, runner.model.LastFetch.Decision, "fetch", "https://example.com/docs")
	assertViewDecision(t, turn.Fetch.Decision, "denied", "fetch", "https://example.com/docs")
}

func TestFetchToolProposalSurfacesNetworkFailureWithoutProviderFallback(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContextAndFetchClient(context.Background(), workspace, "read", appFakeFetchClient{err: errors.New("network boom")})

	turn := runner.proposeFetchTool(runtime.FetchToolRequest{URL: "https://example.com/docs"})

	if got := runner.model.LastFetch.Error; got.Kind != runtime.FetchToolErrorExecution || strings.Contains(got.Message, "network boom") {
		t.Fatalf("network fetch error = %#v", got)
	}
	if !strings.Contains(turn.AssistantText, "fetch https://example.com/docs failed: execution_error") || strings.Contains(strings.ToLower(turn.AssistantText), "provider") {
		t.Fatalf("network fetch assistant = %q", turn.AssistantText)
	}
	if len(runner.model.Diagnostics) != 0 {
		t.Fatalf("network fetch diagnostics = %#v, want no workflow or recovery mutation", runner.model.Diagnostics)
	}
}

func TestWriteToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	turn := runner.proposeWriteTool(runtime.MutationToolRequest{Path: "notes.txt", TargetVersion: "missing", Content: "hello\n", ExpectedEffect: "create notes", Source: runtime.MutationSourceMetadata{Caller: "test", RequestID: "write-1"}})
	if turn.Mutation == nil || turn.Mutation.Status != "completed" || turn.Mutation.Name != "write" || turn.Mutation.Path != "notes.txt" || turn.Mutation.BytesWritten != len("hello\n") || turn.Mutation.PreviousExists {
		t.Fatalf("mutation view = %+v", turn.Mutation)
	}
	if got := readAppTestFile(t, filepath.Join(workspace, "notes.txt")); got != "hello\n" {
		t.Fatalf("written file = %q", got)
	}
	if got := runner.model.LastMutation; got.ToolName != "write" || got.WorkspaceRelativePath != "notes.txt" || (got.Error.Kind != "" && got.Error.Kind != runtime.MutationToolErrorNone) || got.PreviousVersion != "missing" || got.NewVersion == "" {
		t.Fatalf("last mutation = %+v", got)
	}
	assertMutationDecision(t, runner.model.LastMutation.Decision, string(permission.AutonomyWrite), "write", "notes.txt", true)
}

func TestEditToolProposalRoutesThroughExplicitAppEffect(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	path := filepath.Join(workspace, "notes.txt")
	writeAppTestFile(t, workspace, "notes.txt", "alpha\nbeta\n")
	version := appTestFileVersion(t, path)
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	turn := runner.proposeEditTool(runtime.MutationToolRequest{Path: "notes.txt", TargetVersion: version, OldText: "beta", NewText: "gamma", ExpectedEffect: "replace beta", Source: runtime.MutationSourceMetadata{Caller: "test", RequestID: "edit-1"}})
	if turn.Mutation == nil || turn.Mutation.Status != "completed" || turn.Mutation.Name != "edit" || turn.Mutation.ReplacementCount != 1 || turn.Mutation.BytesWritten != len("alpha\ngamma\n") {
		t.Fatalf("edit view = %+v", turn.Mutation)
	}
	if got := readAppTestFile(t, path); got != "alpha\ngamma\n" {
		t.Fatalf("edited file = %q", got)
	}
	assertMutationDecision(t, runner.model.LastMutation.Decision, string(permission.AutonomyWrite), "edit", "notes.txt", true)
}

func TestWriteToolProposalDeniedDoesNotMutate(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	turn := runner.proposeWriteTool(runtime.MutationToolRequest{Path: "notes.txt", TargetVersion: "missing", Content: "hello\n", ExpectedEffect: "create notes"})
	if turn.Mutation == nil || turn.Mutation.Status != "denied" || turn.Mutation.ErrorKind != string(runtime.MutationToolErrorPermission) || !strings.Contains(turn.AssistantText, "read autonomy") {
		t.Fatalf("denied mutation turn = %+v view=%+v", turn, turn.Mutation)
	}
	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied write created file: %v", err)
	}
	assertMutationDecision(t, runner.model.LastMutation.Decision, string(permission.AutonomyRead), "write", "notes.txt", false)
}

func TestWriteToolProposalVersionMismatchDoesNotMutate(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "original")
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	turn := runner.proposeWriteTool(runtime.MutationToolRequest{Path: "notes.txt", TargetVersion: "missing", Content: "changed", ExpectedEffect: "replace notes"})
	if turn.Mutation == nil || turn.Mutation.Status != "failed" || turn.Mutation.ErrorKind != string(runtime.MutationToolErrorTargetVersionMismatch) {
		t.Fatalf("version mismatch view = %+v", turn.Mutation)
	}
	if got := readAppTestFile(t, filepath.Join(workspace, "notes.txt")); got != "original" {
		t.Fatalf("version mismatch mutated file: %q", got)
	}
}

func TestFakeApprovalWriteDecisionRunsExplicitMutationEffect(t *testing.T) {
	workspace := t.TempDir()
	configureFakeApprovalWrite("internal/fake-approval-write.txt", "approved from approval\n")
	t.Cleanup(func() { configureFakeApprovalWrite("", "") })
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))

	_ = runner.proposeApproval(fakeApprovalWriteProposal())
	turn := runner.decideApproval(tui.ApprovalDecisionInput{ProposalID: fakeApprovalWriteProposalID, Action: string(runtime.ApprovalActionApprove)})

	if turn.Mutation == nil || turn.Mutation.Name != "write" || turn.Mutation.Status != "completed" || turn.Mutation.Path != "internal/fake-approval-write.txt" {
		t.Fatalf("approval write turn = %+v mutation=%+v", turn, turn.Mutation)
	}
	if turn.ApprovalDecision == nil || turn.ApprovalDecision.ProposalID != fakeApprovalWriteProposalID || turn.ApprovalDecision.Action != string(runtime.ApprovalActionApprove) {
		t.Fatalf("approval decision view = %+v", turn.ApprovalDecision)
	}
	if got := readAppTestFile(t, filepath.Join(workspace, "internal", "fake-approval-write.txt")); got != "approved from approval\n" {
		t.Fatalf("approval write content = %q", got)
	}
	assertMutationDecision(t, runner.model.LastMutation.Decision, string(permission.AutonomyWrite), "write", "internal/fake-approval-write.txt", true)
}

func TestFakeApprovalWriteDenialDoesNotMutate(t *testing.T) {
	workspace := t.TempDir()
	configureFakeApprovalWrite("internal/fake-approval-write.txt", "")
	t.Cleanup(func() { configureFakeApprovalWrite("", "") })
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))

	_ = runner.proposeApproval(fakeApprovalWriteProposal())
	turn := runner.decideApproval(tui.ApprovalDecisionInput{ProposalID: fakeApprovalWriteProposalID, Action: string(runtime.ApprovalActionDeny)})

	if turn.Mutation != nil || turn.ApprovalDecision == nil || turn.ApprovalDecision.Action != string(runtime.ApprovalActionDeny) {
		t.Fatalf("denied approval write turn = %+v", turn)
	}
	if _, err := os.Stat(filepath.Join(workspace, "internal", "fake-approval-write.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied approval write created file: %v", err)
	}
}

func assertMutationDecision(t *testing.T, decision runtime.ToolDecision, autonomy string, tool string, target string, allowed bool) {
	t.Helper()
	if !decision.Present || decision.Autonomy != autonomy || decision.Source != "autonomy_policy" || decision.Allowed != allowed || decision.Automatic != allowed || decision.OperationKind != string(permission.OperationMutation) || decision.Tool != tool || decision.Target != target || decision.ExpectedEffect == "" {
		t.Fatalf("mutation decision = %+v", decision)
	}
}
