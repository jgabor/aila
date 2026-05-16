package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

func TestSessionControllerPersistsPromptSnapshotThroughExplicitCommand(t *testing.T) {
	t.Parallel()

	var commands []SnapshotPersistenceCommand
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), newInputRunnerWithDispatch(runtime.Dispatch), func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		commands = append(commands, command)
		return SnapshotPersistenceResult{}
	})

	turn := controller.submitPrompt("explain snapshots")

	if turn.UserText != "explain snapshots" || turn.AssistantText != "Fake Aila response: explain snapshots" {
		t.Fatalf("prompt turn = %+v", turn)
	}
	if len(commands) != 1 {
		t.Fatalf("persist commands = %d, want 1", len(commands))
	}
	snapshot := commands[0].Snapshot
	if err := state.ValidateSessionSnapshotContract(snapshot); err != nil {
		t.Fatalf("persisted snapshot contract invalid: %v", err)
	}
	if snapshot.SessionID != currentSessionID || snapshot.Runtime.Status != string(runtime.StatusIdle) || snapshot.Active {
		t.Fatalf("snapshot runtime = %#v active=%v", snapshot.Runtime, snapshot.Active)
	}
	wantTranscript := []state.SessionSnapshotTurn{
		{Role: "user", Source: "prompt", Text: "explain snapshots"},
		{Role: "assistant", Source: "fake-runtime", Text: "Fake Aila response: explain snapshots"},
	}
	if !reflect.DeepEqual(snapshot.Transcript, wantTranscript) {
		t.Fatalf("snapshot transcript = %#v, want %#v", snapshot.Transcript, wantTranscript)
	}
	if len(snapshot.Concerns) == 0 || !strings.Contains(snapshot.Concerns[0].Text, "phase=IDLE") {
		t.Fatalf("snapshot concerns = %#v, want visible status context", snapshot.Concerns)
	}
}

func TestContinueStartupLoadsCurrentSnapshotIntoInjectedViewState(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	snapshot := validAppSessionSnapshot()
	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	view, err := initialDisplayStateWithResume(context.Background(), workspace, true)
	if err != nil {
		t.Fatalf("resume startup: %v", err)
	}

	if view.RuntimeStatus != "idle" || view.StatusSource != "runtime.dispatch" || view.RuntimeResult != "fake remembered result" || view.RuntimeActive {
		t.Fatalf("runtime view = status=%q source=%q result=%q active=%v", view.RuntimeStatus, view.StatusSource, view.RuntimeResult, view.RuntimeActive)
	}
	if got, want := view.QueuedText, []string{"queued follow-up"}; !reflect.DeepEqual(got, want) || view.QueuedCount != 1 {
		t.Fatalf("queued view = %v count=%d, want %v count=1", got, view.QueuedCount, want)
	}
	if len(view.Transcript) != 2 || view.Transcript[0].UserText != "remembered prompt" || view.Transcript[1].AssistantText != "remembered answer" {
		t.Fatalf("transcript view = %#v", view.Transcript)
	}
	if len(view.Diagnostics) != 1 || view.Diagnostics[0].BoundedMessage != "remembered diagnostic" {
		t.Fatalf("diagnostics = %#v", view.Diagnostics)
	}
	if view.MemorySource != "state.current-session-snapshot" || view.MemorySessionID != "current" {
		t.Fatalf("memory source/session = %q/%q, want injected current-session memory", view.MemorySource, view.MemorySessionID)
	}
	if got, want := view.MemoryBlockers, []string{"remembered blocker"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("memory blockers = %#v, want %#v", got, want)
	}
	if got, want := view.MemoryConcerns, []string{"remembered concern"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("memory concerns = %#v, want %#v", got, want)
	}
}

func TestContinueStartupMissingSnapshotFallsBackToIdleState(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	view, err := initialDisplayStateWithResume(context.Background(), workspace, true)
	if err != nil {
		t.Fatalf("resume startup without snapshot: %v", err)
	}

	if view.Scenario != "idle-empty" || view.RuntimeStatus != "" || len(view.Transcript) != 0 || view.QueuedCount != 0 {
		t.Fatalf("missing snapshot did not fall back to idle state: %#v", view)
	}
	for _, forbidden := range []string{"Fake Aila response", "remembered prompt", "hardcoded memory"} {
		if strings.Contains(tui.RenderPlain(view, tui.Size{Width: 80, Height: 24}), forbidden) {
			t.Fatalf("missing snapshot render contains forbidden memory %q", forbidden)
		}
	}
}

func TestContinueStartupInvalidSnapshotSurfacesRecoveryWithoutOverwrite(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	location, err := state.CurrentSessionSnapshotLocation(store.Layout())
	if err != nil {
		t.Fatalf("snapshot location: %v", err)
	}
	const invalid = `{"schema_version":2,"session_id":"bad token=secret-value"}`
	if err := os.Mkdir(filepath.Dir(location.Path), 0o755); err != nil {
		t.Fatalf("create sessions dir: %v", err)
	}
	if err := os.WriteFile(location.Path, []byte(invalid), 0o644); err != nil {
		t.Fatalf("seed invalid snapshot: %v", err)
	}

	view, err := initialDisplayStateWithResume(context.Background(), workspace, true)
	if err != nil {
		t.Fatalf("resume invalid snapshot: %v", err)
	}

	if len(view.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one recovery diagnostic", view.Diagnostics)
	}
	diagnosticView := view.Diagnostics[0]
	if diagnosticView.Source != string(diagnostic.SourceStateSnapshot) || diagnosticView.AffectedArtifact != string(diagnostic.ArtifactSessionSnapshot) || diagnosticView.RecoveryAction != string(diagnostic.RecoveryManualRepair) || !diagnosticView.UserInputNeeded {
		t.Fatalf("diagnostic metadata = %+v", diagnosticView)
	}
	if strings.Contains(diagnosticView.BoundedMessage, workspace) || strings.Contains(diagnosticView.BoundedMessage, "secret-value") || len(diagnosticView.BoundedMessage) > diagnostic.MaxMessageBytes {
		t.Fatalf("diagnostic is not path-safe/bounded: %q", diagnosticView.BoundedMessage)
	}
	content, err := os.ReadFile(location.Path)
	if err != nil {
		t.Fatalf("read invalid snapshot: %v", err)
	}
	if string(content) != invalid {
		t.Fatalf("invalid snapshot was overwritten: got %q want %q", string(content), invalid)
	}
}

func TestSessionControllerPersistsQueuedInterruptDiagnosticsBlockersAndConcerns(t *testing.T) {
	t.Parallel()

	var last SnapshotPersistenceCommand
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil }), func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		last = command
		return SnapshotPersistenceResult{}
	})
	controller.view.ProjectStoreStatus = "degraded"
	controller.view.ProjectStoreSource = "state.open"
	controller.view.ProjectStoreDetail = "project metadata needs manual review"
	controller.view.Diagnostics = []tui.DiagnosticView{{
		Severity:         string(diagnostic.SeverityWarning),
		Source:           string(diagnostic.SourceStateOpen),
		RecoveryAction:   string(diagnostic.RecoveryInspect),
		AffectedArtifact: string(diagnostic.ArtifactProjectStore),
		UserInputNeeded:  true,
		BoundedMessage:   "bounded startup diagnostic",
	}}

	controller.submitPrompt("active fake work")
	controller.submitPrompt("queued follow-up")
	controller.requestInterrupt("ctrl-c")

	snapshot := last.Snapshot
	if snapshot.Runtime.Status != string(runtime.StatusCanceling) || !snapshot.Active {
		t.Fatalf("snapshot runtime = %#v active=%v, want canceling active", snapshot.Runtime, snapshot.Active)
	}
	if got, want := snapshot.Queued, []state.SessionSnapshotQueuedEntry{{ID: "queue-1", Source: "prompt", Text: "queued follow-up"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot queue = %#v, want %#v", got, want)
	}
	if len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Message != "bounded startup diagnostic" {
		t.Fatalf("snapshot diagnostics = %#v", snapshot.Diagnostics)
	}
	if len(snapshot.Blockers) < 2 {
		t.Fatalf("snapshot blockers = %#v, want interrupt and store blockers", snapshot.Blockers)
	}
	if len(snapshot.Concerns) == 0 || !strings.Contains(snapshot.Concerns[0].Text, "primary_model=deepseek/deepseek-chat") {
		t.Fatalf("snapshot concerns = %#v, want visible model status", snapshot.Concerns)
	}
}

func TestSessionControllerSurfacesBoundedPersistenceFailureWithoutWorkflowMutation(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	controller := newSessionControllerWithPersistence(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		return SnapshotPersistenceResult{Diagnostic: snapshotPersistenceDiagnostic(errors.New("write /tmp/secret/path/current.json: permission denied because token=abc123"))}
	})

	turn := controller.submitPrompt("persist failure")

	if controller.view.Phase != view.Phase || controller.view.PhaseSource != view.PhaseSource {
		t.Fatalf("workflow display mutated from %q/%q to %q/%q", view.Phase, view.PhaseSource, controller.view.Phase, controller.view.PhaseSource)
	}
	if len(turn.Diagnostics) != 1 {
		t.Fatalf("turn diagnostics = %#v, want one persistence diagnostic", turn.Diagnostics)
	}
	diagnosticView := turn.Diagnostics[0]
	if diagnosticView.AffectedArtifact != string(diagnostic.ArtifactSessionSnapshot) || diagnosticView.RecoveryAction != string(diagnostic.RecoveryInspect) || !diagnosticView.UserInputNeeded {
		t.Fatalf("diagnostic metadata = %+v", diagnosticView)
	}
	if strings.Contains(diagnosticView.BoundedMessage, "/tmp/secret") || strings.Contains(diagnosticView.BoundedMessage, "abc123") || len(diagnosticView.BoundedMessage) > diagnostic.MaxMessageBytes {
		t.Fatalf("diagnostic message was not bounded/redacted: %q", diagnosticView.BoundedMessage)
	}
}

func TestSessionControllerPersistsCommandPathsWithoutChangingRuntimeRouting(t *testing.T) {
	t.Parallel()

	var commands []SnapshotPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		commands = append(commands, command)
		return SnapshotPersistenceResult{}
	})

	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteStatus, Kind: policy.CommandInputSlash})
	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteHelp, Kind: policy.CommandInputSlash})
	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteQuit, Kind: policy.CommandInputShortcut})

	if len(commands) != 3 {
		t.Fatalf("persist commands = %d, want status/help/quit", len(commands))
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("runtime dispatches = %#v, want only status runtime effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.FakeCommandEffect); !ok {
		t.Fatalf("status effect = %T, want FakeCommandEffect", dispatched[0][0])
	}
	if runner.model.LastCommand != "status" || runner.model.NextOperation != 1 {
		t.Fatalf("runtime model = %#v, want only status routed through runtime", runner.model)
	}
	if commands[0].Snapshot.Runtime.Result != "fake command result: status" {
		t.Fatalf("status snapshot runtime result = %q", commands[0].Snapshot.Runtime.Result)
	}
	if got := commands[1].Snapshot.Concerns[len(commands[1].Snapshot.Concerns)-1]; got.Source != "policy.command" || got.Text != "visible surface=help" {
		t.Fatalf("help snapshot concern = %#v", got)
	}
}

func snapshotTestView() tui.ViewState {
	view := tui.IdleEmptyState()
	view.Phase = workflow.PhaseIdle.DisplayLabel()
	view.PhaseSource = workflow.PhaseIdle.String()
	view.PrimaryModel = "deepseek/deepseek-chat"
	view.UtilityModel = "deepseek/deepseek-chat"
	view.Autonomy = "suggest"
	view.ProjectStoreStatus = "initialized"
	view.ProjectStoreSource = "state.open"
	view.ProjectStoreDetail = "project store ready"
	return view
}

func validAppSessionSnapshot() state.SessionSnapshot {
	return state.SessionSnapshot{
		SchemaVersion: state.CurrentSessionSnapshotSchemaVersion,
		SessionID:     currentSessionID,
		Runtime: state.SessionSnapshotRuntime{
			Status: "idle",
			Source: "runtime.dispatch",
			Detail: "fake in-memory runtime loop",
			Result: "fake remembered result",
		},
		Active: false,
		Transcript: []state.SessionSnapshotTurn{
			{Role: "user", Source: "prompt", Text: "remembered prompt"},
			{Role: "assistant", Source: "fake-runtime", Text: "remembered answer"},
		},
		Queued: []state.SessionSnapshotQueuedEntry{
			{ID: "queue-1", Source: "prompt", Text: "queued follow-up"},
		},
		Diagnostics: []state.SessionSnapshotDiagnostic{
			{Severity: string(diagnostic.SeverityWarning), Source: string(diagnostic.SourceStateSnapshot), Message: "remembered diagnostic"},
		},
		Blockers: []state.SessionSnapshotBlocker{
			{Source: "runtime.dispatch", Text: "remembered blocker"},
		},
		Concerns: []state.SessionSnapshotConcern{
			{Source: "display.status", Text: "remembered concern"},
		},
	}
}

func TestContinueStartupSnapshotScopeStaysCurrentMemoryOnly(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal(validAppSessionSnapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	for _, forbidden := range []string{"session_index", "event_replay", "context_compaction", "history_browser", "provider_history", "approval_persistence", "undo"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("snapshot schema introduced future-scope field %q: %s", forbidden, content)
		}
	}
}
