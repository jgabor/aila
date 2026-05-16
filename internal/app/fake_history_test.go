package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
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

func assertHistoryEvent(t *testing.T, event history.FakeEvent, kind history.EventKind, provenance string, source string, displayText string) {
	t.Helper()
	if event.Kind != kind || event.Provenance != provenance || event.Source != source || event.DisplayText != displayText {
		t.Fatalf("history event = %#v, want kind=%q provenance=%q source=%q display=%q", event, kind, provenance, source, displayText)
	}
}
