package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tools"
	"github.com/jgabor/aila/internal/tui"
)

const (
	recoveryActionInspectHistory     = "inspect_history"
	recoveryActionDeleteCreatedFile  = "delete_created_file"
	recoveryActionRestoreCreatedFile = "restore_created_file"
)

type recoveryTarget struct {
	Command          string
	TargetEventID    string
	Action           string
	Path             string
	TargetVersion    string
	RedoContent      string
	SourceRecoveryID string
}

func (controller *sessionController) runRecoveryCommand(route policy.CommandRoute) (*history.RecoveryRecord, *tui.DecisionView, []tui.DiagnosticView) {
	command := string(route)
	if command != string(policy.CommandRouteUndo) && command != string(policy.CommandRouteRedo) {
		record := unsupportedRecoveryRecord(command, "unsupported recovery command")
		return record, nil, controller.persistRecoveryHistory(record)
	}
	if controller.readHistory == nil {
		record := unsupportedRecoveryRecord(command, "history reader unavailable")
		return record, nil, controller.persistRecoveryHistory(record)
	}
	result := controller.readHistory(controller.ctx, HistoryReadCommand{})
	if result.State != "" && result.State != state.FakeHistoryLoaded {
		record := unsupportedRecoveryRecord(command, "history unavailable for recovery")
		diagnostics := diagnosticViews(result.Diagnostics)
		diagnostics = append(diagnostics, controller.persistRecoveryHistory(record)...)
		return record, nil, diagnostics
	}
	var target recoveryTarget
	var reason string
	if command == string(policy.CommandRouteUndo) {
		target, reason = selectUndoRecoveryTarget(result.Events)
	} else {
		target, reason = selectRedoRecoveryTarget(result.Events)
	}
	if reason != "" {
		record := unsupportedRecoveryRecord(command, reason)
		diagnostics := diagnosticViews(result.Diagnostics)
		diagnostics = append(diagnostics, controller.persistRecoveryHistory(record)...)
		return record, nil, diagnostics
	}

	record, decision := controller.executeRecoveryTarget(target)
	diagnostics := diagnosticViews(result.Diagnostics)
	diagnostics = append(diagnostics, controller.persistRecoveryHistory(record)...)
	return record, decision, diagnostics
}

func selectUndoRecoveryTarget(events []history.FakeEvent) (recoveryTarget, string) {
	undone := map[string]bool{}
	for _, event := range events {
		if event.Kind != history.EventKindRecovery || event.Recovery == nil || event.Recovery.Status != "completed" || event.Recovery.TargetEventID == "" {
			continue
		}
		switch event.Recovery.Command {
		case string(policy.CommandRouteUndo):
			undone[event.Recovery.TargetEventID] = true
		case string(policy.CommandRouteRedo):
			undone[event.Recovery.TargetEventID] = false
		}
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Kind != history.EventKindMutation || event.Mutation == nil || event.Undo == nil || undone[event.EventID] {
			continue
		}
		if event.Undo.Available && event.Undo.Action == recoveryActionDeleteCreatedFile && len(event.Undo.Paths) == 1 {
			version := event.Undo.NewVersion
			if version == "" {
				version = event.Mutation.NewVersion
			}
			if version == "" || version == tools.MissingFileVersion {
				continue
			}
			return recoveryTarget{
				Command:       string(policy.CommandRouteUndo),
				TargetEventID: event.EventID,
				Action:        recoveryActionDeleteCreatedFile,
				Path:          event.Undo.Paths[0],
				TargetVersion: version,
			}, ""
		}
	}
	return recoveryTarget{}, "no supported undo target"
}

func selectRedoRecoveryTarget(events []history.FakeEvent) (recoveryTarget, string) {
	redone := map[string]bool{}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Kind != history.EventKindRecovery || event.Recovery == nil || event.Recovery.Status != "completed" || event.Recovery.TargetEventID == "" {
			continue
		}
		record := event.Recovery
		if record.Command == string(policy.CommandRouteRedo) {
			redone[record.TargetEventID] = true
			continue
		}
		if record.Command != string(policy.CommandRouteUndo) || redone[record.TargetEventID] {
			continue
		}
		if record.RedoAvailable && record.RedoAction == recoveryActionRestoreCreatedFile && len(record.Paths) == 1 && record.RedoContent != "" {
			targetVersion := record.NewVersion
			if targetVersion == "" {
				targetVersion = tools.MissingFileVersion
			}
			return recoveryTarget{
				Command:          string(policy.CommandRouteRedo),
				TargetEventID:    record.TargetEventID,
				Action:           recoveryActionRestoreCreatedFile,
				Path:             record.Paths[0],
				TargetVersion:    targetVersion,
				RedoContent:      record.RedoContent,
				SourceRecoveryID: event.EventID,
			}, ""
		}
	}
	return recoveryTarget{}, "no supported redo target"
}

func (controller *sessionController) executeRecoveryTarget(target recoveryTarget) (*history.RecoveryRecord, *tui.DecisionView) {
	if target.Command == string(policy.CommandRouteUndo) {
		return controller.executeUndoTarget(target)
	}
	return controller.executeRedoTarget(target)
}

func (controller *sessionController) executeUndoTarget(target recoveryTarget) (*history.RecoveryRecord, *tui.DecisionView) {
	expectedEffect := "delete created file for undo"
	decisionRecord := controller.recoveryDecision(target.Command, target.Path, target.TargetVersion, expectedEffect)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return failedRecoveryRecord(target, "permission_denied", decisionRecord.Reason, decision), decisionView(decision)
	}
	validated, validateErr := tools.ValidateDeleteCreatedFileRequest(controller.workspacePath, tools.DeleteCreatedFileRequest{
		Path:           target.Path,
		TargetVersion:  target.TargetVersion,
		ExpectedEffect: expectedEffect,
		Source: tools.MutationSourceMetadata{
			Caller:      "recovery.undo",
			RequestID:   "undo-" + target.TargetEventID,
			Description: expectedEffect,
		},
	})
	if validateErr.Kind != "" {
		return failedRecoveryRecord(target, string(validateErr.Kind), validateErr.Message, decision), decisionView(decision)
	}
	recheckRecord := controller.recoveryDecision(target.Command, validated.WorkspaceRelativePath, validated.TargetVersion, validated.ExpectedEffect)
	decision = runtimeToolDecision(recheckRecord)
	if !recheckRecord.Allowed {
		return failedRecoveryRecord(target, "permission_denied", recheckRecord.Reason, decision), decisionView(decision)
	}

	result := tools.ExecuteDeleteCreatedFile(controller.ctx, validated)
	record := recoveryRecordFromMutationResult(target, result.MutationResult, decision)
	if result.Error.Kind == "" {
		if redoContent, ok := safeRecoveryRedoContent(target, result.DeletedContent); ok {
			record.RedoAvailable = true
			record.RedoAction = recoveryActionRestoreCreatedFile
			record.RedoContent = redoContent
		} else {
			record.RedoAvailable = false
			record.Reason = "deleted content is not safe bounded redo metadata"
		}
	}
	return record, decisionView(decision)
}

func (controller *sessionController) executeRedoTarget(target recoveryTarget) (*history.RecoveryRecord, *tui.DecisionView) {
	expectedEffect := "restore file from recorded undo"
	decisionRecord := controller.recoveryDecision(target.Command, target.Path, target.TargetVersion, expectedEffect)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return failedRecoveryRecord(target, "permission_denied", decisionRecord.Reason, decision), decisionView(decision)
	}
	validated, validateErr := tools.ValidateWriteRequest(controller.workspacePath, tools.WriteRequest{
		Path:           target.Path,
		TargetVersion:  target.TargetVersion,
		Content:        target.RedoContent,
		ExpectedEffect: expectedEffect,
		Source: tools.MutationSourceMetadata{
			Caller:      "recovery.redo",
			RequestID:   "redo-" + target.TargetEventID,
			Description: expectedEffect,
		},
	})
	if validateErr.Kind != "" {
		return failedRecoveryRecord(target, string(validateErr.Kind), validateErr.Message, decision), decisionView(decision)
	}
	recheckRecord := controller.recoveryDecision(target.Command, validated.WorkspaceRelativePath, validated.TargetVersion, validated.ExpectedEffect)
	decision = runtimeToolDecision(recheckRecord)
	if !recheckRecord.Allowed {
		return failedRecoveryRecord(target, "permission_denied", recheckRecord.Reason, decision), decisionView(decision)
	}

	result := tools.ExecuteWrite(controller.ctx, validated)
	record := recoveryRecordFromMutationResult(target, result, decision)
	record.RedoAvailable = false
	return record, decisionView(decision)
}

func (controller *sessionController) recoveryDecision(command string, path string, targetVersion string, expectedEffect string) permission.DecisionRecord {
	operation := permission.NewRecoveryOperation(command, path, targetVersion, expectedEffect)
	operation.RunID = currentSessionID
	operation.Capability = "recovery." + command
	return permission.DecideRecord(permission.AutonomyLevel(controller.autonomyLevel), operation)
}

func recoveryRecordFromMutationResult(target recoveryTarget, result tools.MutationResult, decision runtime.ToolDecision) *history.RecoveryRecord {
	status := result.Status
	if status == "" {
		status = "completed"
	}
	record := &history.RecoveryRecord{
		Command:            target.Command,
		Status:             status,
		TargetEventID:      target.TargetEventID,
		Action:             target.Action,
		Paths:              []string{target.Path},
		PreviousVersion:    result.PreviousVersion,
		NewVersion:         result.NewVersion,
		DecisionRunID:      decision.RunID,
		DecisionCapability: decision.Capability,
	}
	if result.Error.Kind != "" && result.Error.Kind != tools.MutationErrorNone {
		record.Status = "failed"
		record.ErrorKind = string(result.Error.Kind)
		record.ErrorMessage = result.Error.Message
		record.Reason = result.Error.Message
	}
	return record
}

func failedRecoveryRecord(target recoveryTarget, errorKind string, message string, decision runtime.ToolDecision) *history.RecoveryRecord {
	return &history.RecoveryRecord{
		Command:            target.Command,
		Status:             "failed",
		TargetEventID:      target.TargetEventID,
		Action:             target.Action,
		Paths:              []string{target.Path},
		PreviousVersion:    target.TargetVersion,
		RedoAvailable:      false,
		Reason:             message,
		ErrorKind:          errorKind,
		ErrorMessage:       message,
		DecisionRunID:      decision.RunID,
		DecisionCapability: decision.Capability,
	}
}

func unsupportedRecoveryRecord(command string, reason string) *history.RecoveryRecord {
	if command == "" {
		command = string(policy.CommandRouteUndo)
	}
	return &history.RecoveryRecord{
		Command:       command,
		Status:        "unsupported",
		Action:        recoveryActionInspectHistory,
		RedoAvailable: false,
		Reason:        reason,
	}
}

func safeRecoveryRedoContent(target recoveryTarget, content string) (string, bool) {
	probe := history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          history.EventKindRecovery,
		EventID:       "probe-recovery",
		RunID:         currentSessionID,
		SessionID:     currentSessionID,
		Source:        "recovery.command",
		Provenance:    "recovery.undo",
		DisplayText:   "recovery undo completed " + target.Path,
		Recovery: &history.RecoveryRecord{
			Command:         string(policy.CommandRouteUndo),
			Status:          "completed",
			TargetEventID:   target.TargetEventID,
			Action:          target.Action,
			Paths:           []string{target.Path},
			PreviousVersion: target.TargetVersion,
			NewVersion:      tools.MissingFileVersion,
			RedoAvailable:   true,
			RedoAction:      recoveryActionRestoreCreatedFile,
			RedoContent:     content,
		},
	}
	normalized, err := history.NormalizeFakeEvent(probe)
	if err != nil || normalized.Recovery == nil || normalized.Recovery.RedoContent != content {
		return "", false
	}
	return content, true
}

func (controller *sessionController) persistRecoveryHistory(record *history.RecoveryRecord) []tui.DiagnosticView {
	if record == nil {
		return nil
	}
	display := recoveryHistoryDisplay(record)
	return controller.persistHistoryStructuredEvent(history.EventKindRecovery, "recovery."+record.Command, "recovery.command", display, nil, nil, record)
}

func recoveryHistoryDisplay(record *history.RecoveryRecord) string {
	path := strings.Join(record.Paths, ",")
	if path == "" {
		path = "none"
	}
	display := fmt.Sprintf("recovery %s %s %s", record.Command, record.Status, path)
	if record.TargetEventID != "" {
		display += " target " + record.TargetEventID
	}
	if record.RedoAvailable {
		display += " redo " + record.RedoAction
	}
	if record.Reason != "" && record.Status != "completed" {
		display += " reason " + record.Reason
	}
	return display
}

func recoveryView(record *history.RecoveryRecord, decision *tui.DecisionView) *tui.RecoveryView {
	if record == nil {
		return nil
	}
	return &tui.RecoveryView{
		Command:         record.Command,
		Status:          record.Status,
		TargetEventID:   record.TargetEventID,
		Action:          record.Action,
		Paths:           append([]string(nil), record.Paths...),
		PreviousVersion: record.PreviousVersion,
		NewVersion:      record.NewVersion,
		RedoAvailable:   record.RedoAvailable,
		RedoAction:      record.RedoAction,
		Reason:          record.Reason,
		ErrorKind:       record.ErrorKind,
		ErrorMessage:    record.ErrorMessage,
		Decision:        decision,
	}
}
