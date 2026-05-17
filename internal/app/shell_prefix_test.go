package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/runtime"
)

func TestShellPrefixPromptRoutesThroughBashPermissionHistoryAndSnapshot(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "alpha\n")
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	turn := controller.submitPrompt("!ls -1")

	if turn.UserText != "!ls -1" || turn.Command == nil || turn.Command.Status != "completed" || !turn.Command.ReadOnly {
		t.Fatalf("shell prefix turn = %+v", turn)
	}
	if strings.Join(turn.Command.Argv, " ") != "ls -1" || !containsAnyString(turn.Command.StdoutLines, "notes.txt") {
		t.Fatalf("command view = %+v, want ls output", turn.Command)
	}
	if got := runner.model.LastBash; got.ToolName != "bash" || got.CommandFamily != "ls" || got.Source.Caller != shellPrefixSource {
		t.Fatalf("last bash = %+v, want shell-prefix bash result", got)
	}
	if len(historyEvents) != 1 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[0].Event.Source != shellPrefixSource || !strings.Contains(historyEvents[0].Event.DisplayText, "shell-prefix shell completed !ls -1") {
		t.Fatalf("history events = %+v", historyEvents)
	}
	if len(snapshots) == 0 || len(snapshots[len(snapshots)-1].Snapshot.Transcript) < 2 {
		t.Fatalf("snapshots = %+v, want persisted shell prefix transcript", snapshots)
	}
	lastTranscript := snapshots[len(snapshots)-1].Snapshot.Transcript
	if lastTranscript[0].Role != "user" || lastTranscript[0].Text != "!ls -1" || lastTranscript[1].Role != "assistant" {
		t.Fatalf("snapshot transcript = %+v", lastTranscript)
	}
}

func TestShellPrefixPromptSurfacesValidationFailureThroughBashResult(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, nil)

	turn := controller.submitPrompt("!git checkout main")

	if turn.Command == nil || turn.Command.Status != "failed" || turn.Command.ErrorKind != string(runtime.BashToolErrorUnsafeCommand) {
		t.Fatalf("shell prefix failure turn = %+v", turn)
	}
	if got := runner.model.LastBash.Error; got.Kind != runtime.BashToolErrorUnsafeCommand || !strings.Contains(got.Message, "git subcommand") {
		t.Fatalf("last bash error = %+v", got)
	}
}

func TestReservedSummarizedShellPrefixDefersWithoutBashEffect(t *testing.T) {
	t.Parallel()

	var dispatched []runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, effects...)
		return nil
	})
	var historyEvents []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), runner, nil, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	turn := controller.submitPrompt("!!git status --short")

	if len(dispatched) != 0 || runner.model.LastBash.ToolName != "" || len(runner.model.Transcript) != 0 {
		t.Fatalf("deferred summarized shell touched runtime: effects=%#v model=%+v", dispatched, runner.model)
	}
	if turn.Command == nil || turn.Command.Status != "deferred" || turn.Command.ErrorKind != "deferred" || !strings.Contains(turn.Command.ErrorMessage, "Milestone 39") {
		t.Fatalf("deferred command view = %+v", turn.Command)
	}
	if turn.RuntimeStatus != "idle" || turn.StatusSource != shellPrefixSource || !strings.Contains(turn.RuntimeResult, "Milestone 39") {
		t.Fatalf("deferred runtime surface = %+v", turn)
	}
	if len(historyEvents) != 1 || historyEvents[0].Event.Kind != history.EventKindCommand || !strings.Contains(historyEvents[0].Event.DisplayText, "shell-prefix summarized_shell deferred !!git status --short") {
		t.Fatalf("history events = %+v", historyEvents)
	}
}

func TestOrdinaryPromptBypassesShellPrefixRouting(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(runtime.Dispatch)
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, nil)

	turn := controller.submitPrompt("explain status")

	if turn.UserText != "explain status" || turn.Command != nil || runner.model.LastBash.ToolName != "" {
		t.Fatalf("ordinary prompt turn=%+v model=%+v", turn, runner.model)
	}
}
