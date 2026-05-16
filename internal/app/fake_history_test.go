package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

func TestSessionControllerPersistsVisibleFakeActivityThroughExplicitHistoryCommands(t *testing.T) {
	t.Parallel()

	var commands []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	})

	controller.submitPrompt("explain history")
	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteStatus, Kind: policy.CommandInputSlash}, controller.view)
	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteHelp, Kind: policy.CommandInputSlash}, controller.view)
	controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteQuit, Kind: policy.CommandInputShortcut}, controller.view)

	wantKinds := []history.EventKind{
		history.EventKindPrompt,
		history.EventKindResponse,
		history.EventKindRuntime,
		history.EventKindCommand,
		history.EventKindRuntime,
		history.EventKindCommand,
		history.EventKindCommand,
	}
	gotKinds := make([]history.EventKind, 0, len(commands))
	for _, command := range commands {
		gotKinds = append(gotKinds, command.Event.Kind)
		if _, err := history.NormalizeFakeEvent(command.Event); err != nil {
			t.Fatalf("history command event invalid: %#v err=%v", command.Event, err)
		}
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("history event kinds = %v, want %v", gotKinds, wantKinds)
	}
	assertHistoryEvent(t, commands[0].Event, history.EventKindPrompt, "prompt.submit", "user", "explain history")
	assertHistoryEvent(t, commands[1].Event, history.EventKindResponse, "runtime.response", "fake-runtime", "Fake Aila response: explain history")
	assertHistoryEvent(t, commands[2].Event, history.EventKindRuntime, "runtime.dispatch", "runtime.dispatch", "runtime idle: Fake Aila response: explain history")
	assertHistoryEvent(t, commands[3].Event, history.EventKindCommand, "policy.command", "policy.command", "status via slash")
	assertHistoryEvent(t, commands[4].Event, history.EventKindRuntime, "runtime.dispatch", "runtime.dispatch", "runtime idle: fake command result: status")
	assertHistoryEvent(t, commands[5].Event, history.EventKindCommand, "policy.command", "policy.command", "help via slash")
	assertHistoryEvent(t, commands[6].Event, history.EventKindCommand, "policy.command", "policy.command", "quit via shortcut")
	for index, command := range commands {
		if want := fmt.Sprintf("current-%d", index+1); command.Event.EventID != want || command.Event.RunID != currentSessionID || command.Event.SessionID != currentSessionID {
			t.Fatalf("event identity %d = %#v, want event_id %q current run/session", index, command.Event, want)
		}
	}
}

func TestSessionControllerOpensReadOnlyHistoryWithoutPersistenceOrRuntimeMutation(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.Phase = "Idle"
	view.PhaseSource = "idle"
	var persisted []HistoryPersistenceCommand
	readEvent := history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          history.EventKindPrompt,
		EventID:       "event-1",
		RunID:         "run-1",
		SessionID:     "session-1",
		Source:        "user",
		Provenance:    "prompt.submit",
		DisplayText:   "show me history",
	}
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		t.Fatal("history open must not persist session snapshot")
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		persisted = append(persisted, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded, Events: []history.FakeEvent{readEvent}}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteHistory, Kind: policy.CommandInputSlash}, controller.view)

	if len(persisted) != 0 {
		t.Fatalf("history open persisted fake history commands: %#v", persisted)
	}
	if got.Phase != view.Phase || got.PhaseSource != view.PhaseSource {
		t.Fatalf("history open mutated workflow display from %q/%q to %q/%q", view.Phase, view.PhaseSource, got.Phase, got.PhaseSource)
	}
	if got.RuntimeStatus != view.RuntimeStatus || got.RuntimeResult != view.RuntimeResult {
		t.Fatalf("history open mutated runtime display: before=%+v after=%+v", view, got)
	}
	if got.SurfaceTitle != "history" || !got.HistoryFocus || got.HistoryEmpty || len(got.HistoryItems) != 1 {
		t.Fatalf("history view state = %+v, want focused one-item read-only history", got)
	}
	if got.HistoryItems[0].EventID != "event-1" || got.HistoryItems[0].DisplayText != "show me history" {
		t.Fatalf("history item = %+v", got.HistoryItems[0])
	}
}

func TestSessionControllerPersistsApprovedMutationHistoryWithUndoMetadata(t *testing.T) {
	workspace := t.TempDir()
	configureFakeApprovalWrite("notes.txt", "approved from approval\n")
	t.Cleanup(func() { configureFakeApprovalWrite("", "") })
	var commands []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite)), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	})

	_ = controller.runner.proposeApproval(fakeApprovalWriteProposal())
	turn := controller.decideApproval(tui.ApprovalDecisionInput{ProposalID: fakeApprovalWriteProposalID, Action: string(runtime.ApprovalActionApprove)})

	if turn.Mutation == nil || turn.Mutation.Status != "completed" {
		t.Fatalf("approval mutation turn = %+v", turn)
	}
	if len(commands) != 1 {
		t.Fatalf("history commands = %#v, want one mutation record", commands)
	}
	event := commands[0].Event
	if _, err := history.NormalizeFakeEvent(event); err != nil {
		t.Fatalf("mutation history event invalid: %#v err=%v", event, err)
	}
	if event.Kind != history.EventKindMutation || event.Mutation == nil || event.Undo == nil {
		t.Fatalf("history event = %#v, want structured mutation", event)
	}
	if event.Mutation.ApprovalID != fakeApprovalWriteProposalID || event.Mutation.ApprovalAction != string(runtime.ApprovalActionApprove) {
		t.Fatalf("approval metadata = %#v", event.Mutation)
	}
	if event.Mutation.CommandSource != "approval-write" || event.Mutation.RequestID != "fake-approval-write" || !reflect.DeepEqual(event.Mutation.ChangedPaths, []string{"notes.txt"}) {
		t.Fatalf("mutation source/path metadata = %#v", event.Mutation)
	}
	if !event.Undo.Available || event.Undo.Action != "delete_created_file" || !reflect.DeepEqual(event.Undo.Paths, []string{"notes.txt"}) {
		t.Fatalf("undo metadata = %#v", event.Undo)
	}
}

func TestSessionControllerUndoDeletesSupportedMutationAndRecordsRecovery(t *testing.T) {
	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "approved write")
	version := appTestFileVersion(t, filepath.Join(workspace, "notes.txt"))
	var commands []HistoryPersistenceCommand
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded, Events: []history.FakeEvent{supportedCreateFileMutationEvent("event-mutation", version)}}
	})
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyWrite)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteUndo, Kind: policy.CommandInputSlash}, controller.view)

	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("undo did not remove target file: %v", err)
	}
	if got.Recovery == nil || got.Recovery.Command != "undo" || got.Recovery.Status != "completed" || got.Recovery.TargetEventID != "event-mutation" || !got.Recovery.RedoAvailable || got.Recovery.Decision == nil || !got.Recovery.Decision.Allowed {
		t.Fatalf("undo recovery view = %+v", got.Recovery)
	}
	if len(commands) != 2 {
		t.Fatalf("history commands = %#v, want command plus recovery", commands)
	}
	event := commands[1].Event
	if _, err := history.NormalizeFakeEvent(event); err != nil {
		t.Fatalf("recovery history event invalid: %#v err=%v", event, err)
	}
	if event.Kind != history.EventKindRecovery || event.Recovery == nil || event.Recovery.Command != "undo" || event.Recovery.Status != "completed" || event.Recovery.RedoContent != "approved write" {
		t.Fatalf("recovery event = %#v", event)
	}
}

func TestSessionControllerRedoRestoresRecordedUndoAndRecordsRecovery(t *testing.T) {
	workspace := t.TempDir()
	var commands []HistoryPersistenceCommand
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded, Events: []history.FakeEvent{
			supportedCreateFileMutationEvent("event-mutation", "sha256:original"),
			completedUndoRecoveryEvent("event-undo", "event-mutation", "notes.txt", "sha256:original", "approved write"),
		}}
	})
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyWrite)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteRedo, Kind: policy.CommandInputSlash}, controller.view)

	if content := readAppTestFile(t, filepath.Join(workspace, "notes.txt")); content != "approved write" {
		t.Fatalf("redo restored content = %q", content)
	}
	if got.Recovery == nil || got.Recovery.Command != "redo" || got.Recovery.Status != "completed" || got.Recovery.TargetEventID != "event-mutation" || got.Recovery.RedoAvailable {
		t.Fatalf("redo recovery view = %+v", got.Recovery)
	}
	if len(commands) != 2 || commands[1].Event.Recovery == nil || commands[1].Event.Recovery.Command != "redo" || commands[1].Event.Recovery.NewVersion == "" {
		t.Fatalf("redo history commands = %#v", commands)
	}
}

func TestSessionControllerUndoStaleTargetRecordsFailureWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "changed")
	var commands []HistoryPersistenceCommand
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded, Events: []history.FakeEvent{supportedCreateFileMutationEvent("event-mutation", "sha256:stale")}}
	})
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyWrite)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteUndo, Kind: policy.CommandInputSlash}, controller.view)

	if content := readAppTestFile(t, filepath.Join(workspace, "notes.txt")); content != "changed" {
		t.Fatalf("stale undo mutated file = %q", content)
	}
	if got.Recovery == nil || got.Recovery.Status != "failed" || got.Recovery.ErrorKind != "target_version_mismatch" {
		t.Fatalf("stale undo recovery = %+v", got.Recovery)
	}
	if len(commands) != 2 || commands[1].Event.Recovery == nil || commands[1].Event.Recovery.Status != "failed" {
		t.Fatalf("stale undo history commands = %#v", commands)
	}
}

func TestSessionControllerUndoWithoutSupportedTargetRecordsUnsupportedNoOp(t *testing.T) {
	workspace := t.TempDir()
	var commands []HistoryPersistenceCommand
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded}
	})
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyWrite)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteUndo, Kind: policy.CommandInputSlash}, controller.view)

	if got.Recovery == nil || got.Recovery.Status != "unsupported" || got.Recovery.Action != "inspect_history" || !strings.Contains(got.Recovery.Reason, "no supported undo target") {
		t.Fatalf("unsupported undo recovery = %+v", got.Recovery)
	}
	if len(commands) != 2 || commands[1].Event.Recovery == nil || commands[1].Event.Recovery.Status != "unsupported" {
		t.Fatalf("unsupported undo history commands = %#v", commands)
	}
}

func TestSessionControllerUndoDeniedByPermissionRecordsFailureWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "approved write")
	version := appTestFileVersion(t, filepath.Join(workspace, "notes.txt"))
	var commands []HistoryPersistenceCommand
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyRead)
	controller := newSessionControllerWithPersistenceAndHistoryRead(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		commands = append(commands, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		return HistoryReadResult{State: state.FakeHistoryLoaded, Events: []history.FakeEvent{supportedCreateFileMutationEvent("event-mutation", version)}}
	})
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyRead)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteUndo, Kind: policy.CommandInputSlash}, controller.view)

	if content := readAppTestFile(t, filepath.Join(workspace, "notes.txt")); content != "approved write" {
		t.Fatalf("denied undo mutated file = %q", content)
	}
	if got.Recovery == nil || got.Recovery.Status != "failed" || got.Recovery.ErrorKind != "permission_denied" || got.Recovery.Decision == nil || got.Recovery.Decision.Allowed {
		t.Fatalf("denied undo recovery = %+v", got.Recovery)
	}
	if len(commands) != 2 || commands[1].Event.Recovery == nil || commands[1].Event.Recovery.Status != "failed" {
		t.Fatalf("denied undo history commands = %#v", commands)
	}
}

func TestSessionControllerAppendsFakeHistoryThroughStoreCommand(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), newInputRunnerWithDispatch(runtime.Dispatch), nil, storeHistoryPersistence(workspace))

	controller.submitPrompt("persist visible activity")

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	result, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read fake history: %v", err)
	}
	if result.State != state.FakeHistoryLoaded || len(result.Events) != 3 || len(result.Diagnostics) != 0 {
		t.Fatalf("fake history result = %#v, want three loaded events", result)
	}
	assertHistoryEvent(t, result.Events[0], history.EventKindPrompt, "prompt.submit", "user", "persist visible activity")
	assertHistoryEvent(t, result.Events[1], history.EventKindResponse, "runtime.response", "fake-runtime", "Fake Aila response: persist visible activity")
	assertHistoryEvent(t, result.Events[2], history.EventKindRuntime, "runtime.dispatch", "runtime.dispatch", "runtime idle: Fake Aila response: persist visible activity")
}

func TestSessionControllerSurfacesHistoryPersistenceFailureWithoutWorkflowMutation(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), nil, func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult {
		return HistoryPersistenceResult{Diagnostics: []diagnostic.Diagnostic{historyPersistenceDiagnostic(errors.New("write /tmp/secret/path/fake-events.jsonl failed because token=abc123"))}}
	})

	turn := controller.submitPrompt("history failure")

	if controller.view.Phase != view.Phase || controller.view.PhaseSource != view.PhaseSource {
		t.Fatalf("workflow display mutated from %q/%q to %q/%q", view.Phase, view.PhaseSource, controller.view.Phase, controller.view.PhaseSource)
	}
	if len(turn.Diagnostics) != 1 {
		t.Fatalf("turn diagnostics = %#v, want one deduplicated history diagnostic", turn.Diagnostics)
	}
	got := turn.Diagnostics[0]
	if got.Source != string(diagnostic.SourceStateHistory) || got.AffectedArtifact != string(diagnostic.ArtifactFakeHistory) || got.RecoveryAction != string(diagnostic.RecoveryInspect) || !got.UserInputNeeded {
		t.Fatalf("history diagnostic metadata = %+v", got)
	}
	if strings.Contains(got.BoundedMessage, "/tmp/secret") || strings.Contains(got.BoundedMessage, "abc123") || len(got.BoundedMessage) > diagnostic.MaxMessageBytes {
		t.Fatalf("history diagnostic was not bounded/redacted: %q", got.BoundedMessage)
	}
}

func supportedCreateFileMutationEvent(eventID string, version string) history.FakeEvent {
	event := history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          history.EventKindMutation,
		EventID:       eventID,
		RunID:         "run-1",
		SessionID:     "session-1",
		Source:        "mutation.tool",
		Provenance:    "mutation.result",
		DisplayText:   "mutation write completed notes.txt",
		Mutation: &history.MutationRecord{
			ToolName:              "write",
			Status:                "completed",
			CommandSource:         "approval-write",
			ChangedPaths:          []string{"notes.txt"},
			RequestedPath:         "notes.txt",
			ExpectedEffect:        "create notes",
			PreviousVersion:       "missing",
			NewVersion:            version,
			PreviousExists:        false,
			ResolvedPathAvailable: true,
		},
		Undo: &history.UndoMetadata{
			Available:       true,
			Action:          "delete_created_file",
			Paths:           []string{"notes.txt"},
			PreviousVersion: "missing",
			NewVersion:      version,
		},
	}
	return event
}

func completedUndoRecoveryEvent(eventID string, targetEventID string, path string, version string, redoContent string) history.FakeEvent {
	return history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          history.EventKindRecovery,
		EventID:       eventID,
		RunID:         "run-1",
		SessionID:     "session-1",
		Source:        "recovery.command",
		Provenance:    "recovery.undo",
		DisplayText:   "recovery undo completed " + path,
		Recovery: &history.RecoveryRecord{
			Command:         "undo",
			Status:          "completed",
			TargetEventID:   targetEventID,
			Action:          "delete_created_file",
			Paths:           []string{path},
			PreviousVersion: version,
			NewVersion:      "missing",
			RedoAvailable:   true,
			RedoAction:      "restore_created_file",
			RedoContent:     redoContent,
		},
	}
}

func assertHistoryEvent(t *testing.T, event history.FakeEvent, kind history.EventKind, provenance string, source string, displayText string) {
	t.Helper()
	if event.Kind != kind || event.Provenance != provenance || event.Source != source || event.DisplayText != displayText {
		t.Fatalf("history event = %#v, want kind=%q provenance=%q source=%q display=%q", event, kind, provenance, source, displayText)
	}
}
