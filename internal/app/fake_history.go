package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

// HistoryPersistenceCommand is an explicit app-owned request to append one fake activity event.
type HistoryPersistenceCommand struct {
	Event history.FakeEvent
}

// HistoryPersistenceResult is the typed outcome of a fake history append command.
type HistoryPersistenceResult struct {
	Location    state.FakeHistoryLocation
	Diagnostics []diagnostic.Diagnostic
}

// HistoryReadCommand is an explicit app-owned request to read fake activity history.
type HistoryReadCommand struct{}

// HistoryReadResult is the typed outcome of a read-only fake history command.
type HistoryReadResult struct {
	State       state.FakeHistoryReadState
	Events      []history.FakeEvent
	Diagnostics []diagnostic.Diagnostic
}

type (
	historyPersistenceFunc func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult
	historyReadFunc        func(context.Context, HistoryReadCommand) HistoryReadResult
)

const maxHistoryDisplayEntries = 64

func storeHistoryPersistence(workspacePath string) historyPersistenceFunc {
	return func(ctx context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		store, err := state.OpenProjectStore(ctx, workspacePath)
		if err != nil {
			return HistoryPersistenceResult{Diagnostics: []diagnostic.Diagnostic{historyPersistenceDiagnostic(fmt.Errorf("open project store: %w", err))}}
		}
		result, err := store.AppendFakeHistory(ctx, command.Event)
		if err != nil {
			return HistoryPersistenceResult{Diagnostics: []diagnostic.Diagnostic{historyPersistenceDiagnostic(err)}}
		}
		return HistoryPersistenceResult{Location: result.Location, Diagnostics: result.Diagnostics}
	}
}

func storeHistoryRead(workspacePath string) historyReadFunc {
	return func(ctx context.Context, _ HistoryReadCommand) HistoryReadResult {
		store, err := state.OpenProjectStore(ctx, workspacePath)
		if err != nil {
			return HistoryReadResult{
				State:       state.FakeHistoryRecoveryNeeded,
				Diagnostics: []diagnostic.Diagnostic{historyReadDiagnostic(fmt.Errorf("open project store: %w", err))},
			}
		}
		result, err := store.ReadFakeHistory(ctx)
		if err != nil {
			return HistoryReadResult{
				State:       state.FakeHistoryRecoveryNeeded,
				Diagnostics: []diagnostic.Diagnostic{historyReadDiagnostic(err)},
			}
		}
		return HistoryReadResult{State: result.State, Events: result.Events, Diagnostics: result.Diagnostics}
	}
}

func historyPersistenceDiagnostic(err error) diagnostic.Diagnostic {
	message := "fake history persistence failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateHistory,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactFakeHistory,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
}

func historyReadDiagnostic(err error) diagnostic.Diagnostic {
	message := "fake history read failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateHistory,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactFakeHistory,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
}

func (controller *sessionController) openHistoryView() {
	if controller.readHistory == nil {
		controller.view = tui.ApplyHistoryView(controller.view, nil, 0, true)
		return
	}
	result := controller.readHistory(controller.ctx, HistoryReadCommand{})
	items := historyDisplayItems(result.Events)
	controller.view = tui.ApplyHistoryView(controller.view, items, 0, true)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, diagnosticViews(result.Diagnostics))
}

func historyDisplayItems(events []history.FakeEvent) []tui.HistoryItem {
	if len(events) > maxHistoryDisplayEntries {
		events = events[len(events)-maxHistoryDisplayEntries:]
	}
	items := make([]tui.HistoryItem, 0, len(events))
	for _, event := range events {
		items = append(items, tui.HistoryItem{
			EventID:     event.EventID,
			RunID:       event.RunID,
			SessionID:   event.SessionID,
			Kind:        string(event.Kind),
			Source:      event.Source,
			Provenance:  event.Provenance,
			DisplayText: event.DisplayText,
			Mutation:    historyMutationItem(event.Mutation),
			Undo:        historyUndoItem(event.Undo),
		})
	}
	return items
}

func historyMutationItem(record *history.MutationRecord) *tui.HistoryMutationItem {
	if record == nil {
		return nil
	}
	return &tui.HistoryMutationItem{
		Name:                  record.ToolName,
		Status:                record.Status,
		CommandSource:         record.CommandSource,
		RequestID:             record.RequestID,
		ApprovalID:            record.ApprovalID,
		ApprovalAction:        record.ApprovalAction,
		ChangedPaths:          append([]string(nil), record.ChangedPaths...),
		RequestedPath:         record.RequestedPath,
		ExpectedEffect:        record.ExpectedEffect,
		PreviousVersion:       record.PreviousVersion,
		NewVersion:            record.NewVersion,
		PreviousExists:        record.PreviousExists,
		BytesWritten:          record.BytesWritten,
		ReplacementCount:      record.ReplacementCount,
		ResolvedPathAvailable: record.ResolvedPathAvailable,
		ErrorKind:             record.ErrorKind,
		ErrorMessage:          record.ErrorMessage,
		DecisionRunID:         record.DecisionRunID,
		DecisionCapability:    record.DecisionCapability,
	}
}

func historyUndoItem(metadata *history.UndoMetadata) *tui.HistoryUndoItem {
	if metadata == nil {
		return nil
	}
	return &tui.HistoryUndoItem{
		Available:       metadata.Available,
		Action:          metadata.Action,
		Paths:           append([]string(nil), metadata.Paths...),
		PreviousVersion: metadata.PreviousVersion,
		NewVersion:      metadata.NewVersion,
		Reason:          metadata.Reason,
	}
}

func (controller *sessionController) persistPromptHistory(turn tui.TranscriptTurn) []tui.DiagnosticView {
	var diagnostics []tui.DiagnosticView
	if turn.UserText != "" {
		diagnostics = append(diagnostics, controller.persistHistoryEvent(history.EventKindPrompt, "prompt.submit", "user", turn.UserText)...)
	}
	if turn.AssistantText != "" {
		diagnostics = append(diagnostics, controller.persistHistoryEvent(history.EventKindResponse, "runtime.response", "fake-runtime", turn.AssistantText)...)
	}
	diagnostics = append(diagnostics, controller.persistRuntimeHistory(turn)...)
	diagnostics = append(diagnostics, controller.persistMutationHistory(turn)...)
	return mergeTUIDiagnostics(nil, diagnostics)
}

func (controller *sessionController) persistQueuedPromptHistory(queuedBefore int, turn tui.TranscriptTurn) []tui.DiagnosticView {
	if turn.UserText != "" || turn.QueuedCount <= queuedBefore || queuedBefore < 0 || queuedBefore >= len(turn.QueuedText) {
		return nil
	}
	var diagnostics []tui.DiagnosticView
	for _, text := range turn.QueuedText[queuedBefore:] {
		diagnostics = append(diagnostics, controller.persistHistoryEvent(history.EventKindPrompt, "prompt.queue", "prompt", text)...)
	}
	return mergeTUIDiagnostics(nil, diagnostics)
}

func (controller *sessionController) persistCommandHistory(recommendation policy.CommandRecommendation) []tui.DiagnosticView {
	if recommendation.Route == policy.CommandRouteNone {
		return nil
	}
	display := string(recommendation.Route)
	if recommendation.Kind != "" {
		display += " via " + string(recommendation.Kind)
	}
	return controller.persistHistoryEvent(history.EventKindCommand, "policy.command", "policy.command", display)
}

func (controller *sessionController) persistRuntimeModelHistory(model runtime.Model) []tui.DiagnosticView {
	turn := tui.TranscriptTurn{}
	turn.RuntimeStatus = string(model.Status)
	turn.StatusSource = "runtime.dispatch"
	turn.StatusDetail = "fake in-memory runtime loop"
	turn.RuntimeActive = model.Status == runtime.StatusActive || model.Status == runtime.StatusCanceling
	turn.RuntimeResult = model.Result
	turn.QueuedCount = len(model.Queued)
	turn.QueuedText = queuedText(model.Queued)
	return controller.persistRuntimeHistory(turn)
}

func (controller *sessionController) persistRuntimeHistory(turn tui.TranscriptTurn) []tui.DiagnosticView {
	if turn.RuntimeStatus == "" {
		return nil
	}
	display := "runtime " + turn.RuntimeStatus
	if turn.RuntimeActive {
		display += " active"
	}
	if turn.RuntimeResult != "" {
		display += ": " + turn.RuntimeResult
	}
	if turn.QueuedCount > 0 {
		display += fmt.Sprintf(" queued=%d", turn.QueuedCount)
	}
	return controller.persistHistoryEvent(history.EventKindRuntime, turn.StatusSource, "runtime.dispatch", display)
}

func (controller *sessionController) persistMutationHistory(turn tui.TranscriptTurn) []tui.DiagnosticView {
	if turn.Mutation == nil || turn.Mutation.Status == "running" {
		return nil
	}
	result := controller.runner.model.LastMutation
	if result.ToolName == "" && result.RequestedPath == "" && result.WorkspaceRelativePath == "" {
		return nil
	}
	record := mutationHistoryRecord(result, turn.ApprovalDecision)
	undo := mutationUndoMetadata(record)
	display := mutationHistoryDisplay(record, undo)
	return controller.persistHistoryStructuredEvent(history.EventKindMutation, "mutation.result", "mutation.tool", display, record, undo)
}

func mutationHistoryRecord(result runtime.MutationToolResult, approval *tui.ApprovalDecisionView) *history.MutationRecord {
	path := mutationHistoryPath(result.WorkspaceRelativePath)
	if path == "" {
		path = mutationHistoryPath(result.RequestedPath)
	}
	if path == "" {
		path = "unresolved"
	}
	requestedPath := mutationHistoryPath(result.RequestedPath)
	status := result.Status
	if status == "" {
		status = "completed"
	}
	commandSource := result.Source.Caller
	if commandSource == "" {
		commandSource = result.Decision.Capability
	}
	if commandSource == "" {
		commandSource = "mutation.tool"
	}
	record := &history.MutationRecord{
		ToolName:              defaultString(result.ToolName, "mutation"),
		Status:                status,
		CommandSource:         commandSource,
		RequestID:             result.Source.RequestID,
		ChangedPaths:          []string{path},
		RequestedPath:         requestedPath,
		ExpectedEffect:        result.ExpectedEffect,
		PreviousVersion:       result.PreviousVersion,
		NewVersion:            result.NewVersion,
		PreviousExists:        result.PreviousExists,
		BytesWritten:          result.BytesWritten,
		ReplacementCount:      result.ReplacementCount,
		ResolvedPathAvailable: result.ResolvedPathAvailable,
		ErrorMessage:          result.Error.Message,
		DecisionRunID:         result.Decision.RunID,
		DecisionCapability:    result.Decision.Capability,
	}
	if result.Error.Kind != "" && result.Error.Kind != runtime.MutationToolErrorNone {
		record.ErrorKind = string(result.Error.Kind)
	}
	if approval != nil {
		record.ApprovalID = approval.ProposalID
		record.ApprovalAction = approval.Action
	}
	return record
}

func mutationUndoMetadata(record *history.MutationRecord) *history.UndoMetadata {
	path := ""
	if len(record.ChangedPaths) > 0 {
		path = record.ChangedPaths[0]
	}
	metadata := &history.UndoMetadata{
		Available:       false,
		Paths:           []string{path},
		PreviousVersion: record.PreviousVersion,
		NewVersion:      record.NewVersion,
	}
	if record.Status != "completed" {
		metadata.Action = "inspect_result"
		metadata.Reason = "mutation did not complete"
		return metadata
	}
	if path == "" || path == "unresolved" {
		metadata.Action = "inspect_result"
		metadata.Reason = "changed path unavailable"
		return metadata
	}
	if record.ToolName == "write" && !record.PreviousExists && record.PreviousVersion == "missing" {
		metadata.Available = true
		metadata.Action = "delete_created_file"
		metadata.Reason = ""
		return metadata
	}
	metadata.Action = "restore_previous_content"
	metadata.Reason = "previous content not recorded"
	return metadata
}

func mutationHistoryDisplay(record *history.MutationRecord, undo *history.UndoMetadata) string {
	path := strings.Join(record.ChangedPaths, ",")
	display := fmt.Sprintf("mutation %s %s %s", record.ToolName, record.Status, path)
	if record.ApprovalID != "" {
		display += " approval " + record.ApprovalID
	}
	if undo != nil && undo.Available {
		display += " undo " + undo.Action
	} else if undo != nil && undo.Reason != "" {
		display += " undo unavailable"
	}
	return display
}

func mutationHistoryPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "." || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") || strings.Contains(path, "$HOME") || strings.Contains(path, "${HOME}") || strings.Contains(strings.ToUpper(path), "XDG_") {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == ".aila" || part == ".agentera" {
			return ""
		}
	}
	return path
}

func (controller *sessionController) persistHistoryEvent(kind history.EventKind, provenance string, source string, displayText string) []tui.DiagnosticView {
	return controller.persistHistoryStructuredEvent(kind, provenance, source, displayText, nil, nil)
}

func (controller *sessionController) persistHistoryStructuredEvent(kind history.EventKind, provenance string, source string, displayText string, mutation *history.MutationRecord, undo *history.UndoMetadata) []tui.DiagnosticView {
	if controller.persistHistory == nil || displayText == "" {
		return nil
	}
	controller.historySequence++
	result := controller.persistHistory(controller.ctx, HistoryPersistenceCommand{Event: history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       fmt.Sprintf("%s-%d", currentSessionID, controller.historySequence),
		RunID:         currentSessionID,
		SessionID:     currentSessionID,
		Source:        source,
		Provenance:    provenance,
		DisplayText:   displayText,
		Mutation:      mutation,
		Undo:          undo,
	}})
	return diagnosticViews(result.Diagnostics)
}
