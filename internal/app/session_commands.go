package app

import (
	"context"
	"fmt"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

// SnapshotClearCommand is an explicit app-owned request to clear visible current-session memory.
type SnapshotClearCommand struct{}

// SnapshotClearResult is the typed outcome of clearing current-session memory.
type SnapshotClearResult struct {
	Location   state.SessionSnapshotLocation
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) openNewSessionView() []tui.DiagnosticView {
	fresh := resetVisibleSessionState(controller.view)
	fresh.RuntimeStatus = "idle"
	fresh.StatusSource = "app.session"
	fresh.StatusDetail = "fresh session started"
	diagnostics := controller.persistSnapshotForView(fresh)
	controller.view = tui.ApplySessionView(fresh, &tui.SessionView{
		Action:       "new",
		Source:       "app.session",
		Status:       "fresh",
		SessionID:    currentSessionID,
		MemoryStatus: "fresh",
		Detail:       "started fresh session and preserved project store",
	})
	return diagnostics
}

func (controller *sessionController) openClearSessionView() []tui.DiagnosticView {
	diagnostics := controller.clearCurrentSnapshot()
	cleared := resetVisibleSessionState(controller.view)
	cleared.RuntimeStatus = "idle"
	cleared.StatusSource = "app.session"
	cleared.StatusDetail = "session cleared"
	status := "cleared"
	detail := "cleared visible session and current memory"
	if len(diagnostics) > 0 {
		status = "recovery_needed"
		detail = "session clear needs inspection"
	}
	controller.view = tui.ApplySessionView(cleared, &tui.SessionView{
		Action:       "clear",
		Source:       "app.session",
		Status:       status,
		SessionID:    currentSessionID,
		MemoryStatus: "cleared",
		Detail:       detail,
	})
	return diagnostics
}

func (controller *sessionController) openContinueSessionView() []tui.DiagnosticView {
	result := readCurrentSessionSnapshot(controller.ctx, controller.workspacePath, SnapshotResumeCommand{})
	session := &tui.SessionView{
		Action:       "continue",
		Source:       "app.session",
		Status:       string(result.State),
		SessionID:    currentSessionID,
		MemoryStatus: "no_memory",
		Detail:       "no current session memory recorded",
	}
	diagnostics := diagnosticViews(result.Diagnostics)
	switch result.State {
	case state.SessionSnapshotLoaded:
		restored := applyCurrentSessionSnapshot(resetVisibleSessionState(controller.view), result.Snapshot)
		controller.runner.model = applySnapshotToRuntimeModel(controller.runner.model, result.Snapshot)
		sessionID := result.Snapshot.SessionID
		if sessionID == "" {
			sessionID = currentSessionID
		}
		memoryStatus := restoredMemoryStatus(result.Snapshot)
		session.Status = "loaded"
		session.SessionID = sessionID
		session.MemoryStatus = memoryStatus
		session.Detail = "restored current session snapshot"
		session.Focus = true
		session.Items = []tui.SessionItemView{{
			ID:           sessionID,
			Status:       "loaded",
			MemoryStatus: memoryStatus,
			Detail:       "current session",
		}}
		controller.view = tui.ApplySessionView(restored, session)
	case state.SessionSnapshotRecoveryNeeded:
		session.Status = "recovery_needed"
		session.MemoryStatus = "recovery_needed"
		session.Detail = "current session memory requires recovery"
		controller.view = tui.ApplySessionView(controller.view, session)
	default:
		controller.view = tui.ApplySessionView(controller.view, session)
	}
	return diagnostics
}

func resetVisibleSessionState(view tui.ViewState) tui.ViewState {
	return tui.ViewState{
		Scenario:           view.Scenario,
		AppName:            view.AppName,
		Phase:              workflow.PhaseIdle.DisplayLabel(),
		PhaseSource:        workflow.PhaseIdle.String(),
		PrimaryModel:       view.PrimaryModel,
		UtilityModel:       view.UtilityModel,
		Autonomy:           view.Autonomy,
		ProjectStoreStatus: view.ProjectStoreStatus,
		ProjectStoreSource: view.ProjectStoreSource,
		ProjectStoreDetail: view.ProjectStoreDetail,
		FooterGit:          view.FooterGit,
		FooterContext:      view.FooterContext,
	}
}

func restoredMemoryStatus(snapshot state.SessionSnapshot) string {
	if len(snapshot.Transcript) == 0 && len(snapshot.Queued) == 0 && len(snapshot.Diagnostics) == 0 && len(snapshot.Blockers) == 0 && len(snapshot.Concerns) == 0 && snapshot.Run == nil {
		return "empty"
	}
	return "visible"
}

func (controller *sessionController) persistSnapshotForView(view tui.ViewState) []tui.DiagnosticView {
	if controller.persist == nil {
		return nil
	}
	result := controller.persist(controller.ctx, SnapshotPersistenceCommand{Snapshot: NewCurrentSessionSnapshot(view, controller.runner.model)})
	if result.Diagnostic == nil {
		return nil
	}
	return diagnosticViews([]diagnostic.Diagnostic{*result.Diagnostic})
}

func (controller *sessionController) clearCurrentSnapshot() []tui.DiagnosticView {
	result := clearCurrentSessionSnapshot(controller.ctx, controller.workspacePath, SnapshotClearCommand{})
	if result.Diagnostic == nil {
		return nil
	}
	return diagnosticViews([]diagnostic.Diagnostic{*result.Diagnostic})
}

func clearCurrentSessionSnapshot(ctx context.Context, workspacePath string, _ SnapshotClearCommand) SnapshotClearResult {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return SnapshotClearResult{Diagnostic: snapshotClearDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	location, err := store.ClearCurrentSessionSnapshot(ctx)
	if err != nil {
		return SnapshotClearResult{Diagnostic: snapshotClearDiagnostic(err)}
	}
	return SnapshotClearResult{Location: location}
}

func snapshotClearDiagnostic(err error) *diagnostic.Diagnostic {
	message := "current session snapshot clear failed"
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
