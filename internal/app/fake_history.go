package app

import (
	"context"
	"fmt"

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
		})
	}
	return items
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

func (controller *sessionController) persistHistoryEvent(kind history.EventKind, provenance string, source string, displayText string) []tui.DiagnosticView {
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
	}})
	return diagnosticViews(result.Diagnostics)
}
