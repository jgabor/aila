package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/tui"
)

func TestCompactCommandRoutesThroughExplicitEffectHistorySnapshotAndContext(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.Context = &tui.ContextView{
		Source: "app.context",
		Status: "ready",
		Meter:  "1 blocks / 2 refs / 42 bytes",
		Blocks: []tui.ContextBlockView{{
			ID:           "block-1",
			Kind:         "command_output",
			Title:        "Summarized shell output",
			Text:         "command git status --short completed exit 0\nstdout: M internal/context/context.go",
			SourceRefIDs: []string{"command-1", "command-1-stdout-1"},
		}},
		Claims: []tui.ContextClaimView{{Text: "command git status --short completed exit 0", SourceRefIDs: []string{"command-1-stdout-1"}}},
		SourceRefs: []tui.ContextSourceRefView{
			{ID: "command-1", Kind: "command", Command: "git status --short", Excerpt: "git status --short"},
			{ID: "command-1-stdout-1", Kind: "command_stdout", Command: "git status --short", Stream: "stdout", Excerpt: "M internal/context/context.go"},
		},
	}
	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteCompact, Kind: policy.CommandInputSlash}, view)

	if got.Compact == nil || got.Compact.Status != "completed" || !strings.Contains(got.Compact.Summary, "manual compaction preserved 2 source refs") {
		t.Fatalf("compact view = %+v", got.Compact)
	}
	if got.Context == nil || !strings.Contains(got.Context.Meter, "2 refs") || !compactContextHasRef(got.Context, "command-1-stdout-1", "M internal/context/context.go") {
		t.Fatalf("compacted context = %+v", got.Context)
	}
	if runner.model.LastCommand != "compact" || runner.model.LastCompact.Status != "completed" || runner.model.ActiveOperation.ID != "" {
		t.Fatalf("runtime compact model = last command %q compact=%+v active=%+v", runner.model.LastCommand, runner.model.LastCompact, runner.model.ActiveOperation)
	}
	if len(snapshots) == 0 {
		t.Fatalf("snapshots = %#v, want persisted compact command snapshot", snapshots)
	}
	snapshot := snapshots[len(snapshots)-1].Snapshot
	if !strings.Contains(snapshot.Runtime.Result, "manual compaction preserved 2 source refs") {
		t.Fatalf("snapshot = %#v, want persisted compact result", snapshot)
	}
	if !compactHistoryContains(historyEvents, history.EventKindCommand, "compact") {
		t.Fatalf("history events = %#v, want compact command history", historyEvents)
	}
}

func TestCompactCommandReportsVisibleCaveatWithoutContext(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistence(context.Background(), view, runner, nil)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteCompact, Kind: policy.CommandInputShortcut}, view)

	if got.Compact == nil || got.Compact.Status != "flagged" || len(got.Compact.Caveats) != 1 || !strings.Contains(got.Compact.Caveats[0], "no context") {
		t.Fatalf("empty compact view = %+v", got.Compact)
	}
	if got.Context == nil || len(got.Context.Warnings) == 0 || !strings.Contains(got.RuntimeResult, "caveat") {
		t.Fatalf("empty compact context/result = context:%+v result:%q", got.Context, got.RuntimeResult)
	}
}

func compactContextHasRef(contextView *tui.ContextView, id string, excerpt string) bool {
	for _, ref := range contextView.SourceRefs {
		if ref.ID == id && strings.Contains(ref.Excerpt, excerpt) {
			return true
		}
	}
	return false
}

func compactHistoryContains(events []HistoryPersistenceCommand, kind history.EventKind, text string) bool {
	for _, event := range events {
		if event.Event.Kind == kind && strings.Contains(event.Event.DisplayText, text) {
			return true
		}
	}
	return false
}
