package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

const currentSessionID = "current"

// SnapshotPersistenceCommand is an explicit app-owned request to persist visible session state.
type SnapshotPersistenceCommand struct {
	Snapshot state.SessionSnapshot
}

// SnapshotPersistenceResult is the typed outcome of a snapshot persistence command.
type SnapshotPersistenceResult struct {
	Location   state.SessionSnapshotLocation
	Diagnostic *diagnostic.Diagnostic
}

// SnapshotResumeCommand is an explicit app-owned request to read visible session memory.
type SnapshotResumeCommand struct{}

// SnapshotResumeResult is the typed startup outcome for current-session memory.
type SnapshotResumeResult struct {
	State       state.SessionSnapshotReadState
	Snapshot    state.SessionSnapshot
	Diagnostics []diagnostic.Diagnostic
}

type snapshotPersistenceFunc func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult

type sessionController struct {
	ctx     context.Context
	runner  *inputRunner
	view    tui.ViewState
	persist snapshotPersistenceFunc
}

func newController(ctx context.Context, workspacePath string, view tui.ViewState, runner *inputRunner) *sessionController {
	return newSessionControllerWithPersistence(ctx, view, runner, storeSnapshotPersistence(workspacePath))
}

func newSessionControllerWithPersistence(ctx context.Context, view tui.ViewState, runner *inputRunner, persist snapshotPersistenceFunc) *sessionController {
	if ctx == nil {
		ctx = context.Background()
	}
	return &sessionController{ctx: ctx, runner: runner, view: view, persist: persist}
}

func (controller *sessionController) submitPrompt(text string) tui.TranscriptTurn {
	turn := controller.runner.submitPrompt(text)
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) requestInterrupt(reason string) tui.TranscriptTurn {
	turn := controller.runner.requestInterrupt(reason)
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) requestShutdown(err error) tui.TranscriptTurn {
	turn := controller.runner.requestShutdown(err)
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) routeCommand(recommendation policy.CommandRecommendation) {
	controller.view = tui.ApplyCommandRecommendation(controller.view, recommendation)
	controller.runner.routeCommand(recommendation)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model)
	_ = controller.persistCurrentSnapshot(tui.TranscriptTurn{})
}

func (controller *sessionController) persistCurrentSnapshot(turn tui.TranscriptTurn) tui.TranscriptTurn {
	if controller.persist == nil {
		return turn
	}
	result := controller.persist(controller.ctx, SnapshotPersistenceCommand{Snapshot: NewCurrentSessionSnapshot(controller.view)})
	if result.Diagnostic == nil {
		return turn
	}
	view := diagnosticViews([]diagnostic.Diagnostic{*result.Diagnostic})
	turn.Diagnostics = append(turn.Diagnostics, view...)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, view)
	return turn
}

func applyRuntimeModelToView(view tui.ViewState, model runtime.Model) tui.ViewState {
	turn := tui.TranscriptTurn{}
	turn.RuntimeStatus = string(model.Status)
	turn.StatusSource = "runtime.dispatch"
	turn.StatusDetail = "fake in-memory runtime loop"
	turn.RuntimeActive = model.Status == runtime.StatusActive || model.Status == runtime.StatusCanceling
	turn.RuntimeResult = model.Result
	turn.QueuedCount = len(model.Queued)
	turn.QueuedText = queuedText(model.Queued)
	turn.Diagnostics = diagnosticViews(model.Diagnostics)
	return tui.ApplyTranscriptTurn(view, turn)
}

func storeSnapshotPersistence(workspacePath string) snapshotPersistenceFunc {
	return func(ctx context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		store, err := state.OpenProjectStore(ctx, workspacePath)
		if err != nil {
			return SnapshotPersistenceResult{Diagnostic: snapshotPersistenceDiagnostic(fmt.Errorf("open project store: %w", err))}
		}
		location, err := store.WriteCurrentSessionSnapshot(ctx, command.Snapshot)
		if err != nil {
			return SnapshotPersistenceResult{Diagnostic: snapshotPersistenceDiagnostic(err)}
		}
		return SnapshotPersistenceResult{Location: location}
	}
}

func resumeCurrentSessionSnapshot(ctx context.Context, workspacePath string, view tui.ViewState) tui.ViewState {
	result := readCurrentSessionSnapshot(ctx, workspacePath, SnapshotResumeCommand{})
	switch result.State {
	case state.SessionSnapshotLoaded:
		return applyCurrentSessionSnapshot(view, result.Snapshot)
	case state.SessionSnapshotRecoveryNeeded:
		view.Diagnostics = mergeTUIDiagnostics(view.Diagnostics, diagnosticViews(result.Diagnostics))
	}
	return view
}

func readCurrentSessionSnapshot(ctx context.Context, workspacePath string, _ SnapshotResumeCommand) SnapshotResumeResult {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return SnapshotResumeResult{
			State:       state.SessionSnapshotRecoveryNeeded,
			Diagnostics: []diagnostic.Diagnostic{snapshotResumeDiagnostic(fmt.Errorf("open project store: %w", err))},
		}
	}
	result, err := store.ReadCurrentSessionSnapshot(ctx)
	if err != nil {
		return SnapshotResumeResult{
			State:       state.SessionSnapshotRecoveryNeeded,
			Diagnostics: []diagnostic.Diagnostic{snapshotResumeDiagnostic(err)},
		}
	}
	return SnapshotResumeResult{State: result.State, Snapshot: result.Snapshot, Diagnostics: result.Diagnostics}
}

func applyCurrentSessionSnapshot(view tui.ViewState, snapshot state.SessionSnapshot) tui.ViewState {
	view.MemorySource = "state.current-session-snapshot"
	view.MemorySessionID = snapshot.SessionID
	view.RuntimeStatus = snapshot.Runtime.Status
	view.StatusSource = snapshot.Runtime.Source
	view.StatusDetail = snapshot.Runtime.Detail
	view.RuntimeResult = snapshot.Runtime.Result
	view.RuntimeActive = snapshot.Active
	view.QueuedText = snapshotQueuedText(snapshot.Queued)
	view.QueuedCount = len(view.QueuedText)
	view.Transcript = snapshotTranscriptTurns(snapshot.Transcript)
	view.MemoryBlockers = snapshotBlockerText(snapshot.Blockers)
	view.MemoryConcerns = snapshotConcernText(snapshot.Concerns)
	view.Diagnostics = mergeTUIDiagnostics(view.Diagnostics, snapshotDiagnosticViews(snapshot.Diagnostics))
	return view
}

func snapshotBlockerText(entries []state.SessionSnapshotBlocker) []string {
	blockers := make([]string, 0, len(entries))
	for _, entry := range entries {
		blockers = append(blockers, entry.Text)
	}
	return blockers
}

func snapshotConcernText(entries []state.SessionSnapshotConcern) []string {
	concerns := make([]string, 0, len(entries))
	for _, entry := range entries {
		concerns = append(concerns, entry.Text)
	}
	return concerns
}

func snapshotQueuedText(entries []state.SessionSnapshotQueuedEntry) []string {
	queued := make([]string, 0, len(entries))
	for _, entry := range entries {
		queued = append(queued, entry.Text)
	}
	return queued
}

func snapshotTranscriptTurns(turns []state.SessionSnapshotTurn) []tui.TranscriptTurn {
	transcript := make([]tui.TranscriptTurn, 0, len(turns))
	for _, turn := range turns {
		switch turn.Role {
		case "user":
			transcript = append(transcript, tui.TranscriptTurn{UserText: turn.Text})
		case "assistant":
			transcript = append(transcript, tui.TranscriptTurn{AssistantText: turn.Text})
		}
	}
	return transcript
}

func snapshotDiagnosticViews(diagnostics []state.SessionSnapshotDiagnostic) []tui.DiagnosticView {
	views := make([]tui.DiagnosticView, 0, len(diagnostics))
	for _, item := range diagnostics {
		views = append(views, tui.DiagnosticView{
			Severity:         item.Severity,
			Source:           item.Source,
			RecoveryAction:   string(diagnostic.RecoveryInspect),
			AffectedArtifact: string(diagnostic.ArtifactSessionSnapshot),
			BoundedMessage:   item.Message,
		})
	}
	return views
}

// NewCurrentSessionSnapshot converts app-owned visible state into the current snapshot schema.
func NewCurrentSessionSnapshot(view tui.ViewState) state.SessionSnapshot {
	return state.SessionSnapshot{
		SchemaVersion: state.CurrentSessionSnapshotSchemaVersion,
		SessionID:     currentSessionID,
		Runtime: state.SessionSnapshotRuntime{
			Status: view.RuntimeStatus,
			Source: view.StatusSource,
			Detail: view.StatusDetail,
			Result: view.RuntimeResult,
		},
		Active:      view.RuntimeActive,
		Transcript:  snapshotTranscript(view.Transcript),
		Queued:      snapshotQueued(view.QueuedText),
		Diagnostics: snapshotDiagnostics(view.Diagnostics),
		Blockers:    snapshotBlockers(view),
		Concerns:    snapshotConcerns(view),
	}
}

func snapshotTranscript(transcript []tui.TranscriptTurn) []state.SessionSnapshotTurn {
	turns := make([]state.SessionSnapshotTurn, 0, len(transcript)*2)
	for _, turn := range transcript {
		if turn.UserText != "" {
			turns = append(turns, state.SessionSnapshotTurn{Role: "user", Source: "prompt", Text: turn.UserText})
		}
		if turn.AssistantText != "" {
			turns = append(turns, state.SessionSnapshotTurn{Role: "assistant", Source: "fake-runtime", Text: turn.AssistantText})
		}
	}
	return turns
}

func snapshotQueued(entries []string) []state.SessionSnapshotQueuedEntry {
	queued := make([]state.SessionSnapshotQueuedEntry, 0, len(entries))
	for index, text := range entries {
		queued = append(queued, state.SessionSnapshotQueuedEntry{ID: fmt.Sprintf("queue-%d", index+1), Source: "prompt", Text: text})
	}
	return queued
}

func snapshotDiagnostics(diagnostics []tui.DiagnosticView) []state.SessionSnapshotDiagnostic {
	items := make([]state.SessionSnapshotDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		items = append(items, state.SessionSnapshotDiagnostic{Severity: diagnostic.Severity, Source: diagnostic.Source, Message: diagnostic.BoundedMessage})
	}
	return items
}

func snapshotBlockers(view tui.ViewState) []state.SessionSnapshotBlocker {
	var blockers []state.SessionSnapshotBlocker
	if view.RuntimeStatus == string(runtime.StatusCanceling) {
		blockers = append(blockers, state.SessionSnapshotBlocker{Source: view.StatusSource, Text: "interrupt pending"})
	}
	if view.ProjectStoreStatus != "" && view.ProjectStoreStatus != "initialized" {
		blockers = append(blockers, state.SessionSnapshotBlocker{Source: view.ProjectStoreSource, Text: "project store " + view.ProjectStoreStatus + ": " + view.ProjectStoreDetail})
	}
	return blockers
}

func snapshotConcerns(view tui.ViewState) []state.SessionSnapshotConcern {
	var concerns []state.SessionSnapshotConcern
	if view.Phase != "" || view.PrimaryModel != "" || view.UtilityModel != "" || view.Autonomy != "" {
		concerns = append(concerns, state.SessionSnapshotConcern{Source: "display.status", Text: strings.Join([]string{
			"phase=" + view.Phase,
			"primary_model=" + view.PrimaryModel,
			"utility_model=" + view.UtilityModel,
			"autonomy=" + view.Autonomy,
		}, " ")})
	}
	if view.SurfaceTitle != "" {
		concerns = append(concerns, state.SessionSnapshotConcern{Source: "policy.command", Text: "visible surface=" + view.SurfaceTitle})
	}
	return concerns
}

func snapshotPersistenceDiagnostic(err error) *diagnostic.Diagnostic {
	message := "current session snapshot persistence failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactSessionSnapshot,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func snapshotResumeDiagnostic(err error) diagnostic.Diagnostic {
	message := "current session snapshot resume failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityError,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactSessionSnapshot,
		RecoveryAction:   diagnostic.RecoveryManualRepair,
		UserInputNeeded:  true,
	})
}

func mergeTUIDiagnostics(existing []tui.DiagnosticView, added []tui.DiagnosticView) []tui.DiagnosticView {
	merged := append([]tui.DiagnosticView(nil), existing...)
	for _, diagnostic := range added {
		found := false
		for _, current := range merged {
			if current == diagnostic {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, diagnostic)
		}
	}
	return merged
}
