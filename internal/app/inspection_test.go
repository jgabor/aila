package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

func TestStatusCommandBuildsAppOwnedRuntimeInspectionSurface(t *testing.T) {
	t.Parallel()

	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		t.Fatal("status inspection must not read fake history")
		return HistoryReadResult{}
	}, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("status inspection must not read diff state")
		return DiffReadResult{}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteStatus, Kind: policy.CommandInputSlash}, controller.view)

	if got.SurfaceTitle != "status" || got.CommandRoute != "status" || got.RouteSource != "policy.command" {
		t.Fatalf("status command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	lines := strings.Join(got.SurfaceLines, "\n")
	for _, want := range []string{
		"source: app.status",
		"read-only: true",
		"runtime source: runtime.dispatch",
		"runtime detail: utility worker status",
		"runtime result: fake command result: status",
		"last command: status",
		"project store: initialized (state.open; project store ready)",
		"utility worker: completed",
		"utility source: app.status",
		"utility job: context_prep status-context-prep",
		"utility summary: prepared context ready",
		"utility prepared context: Likely next context: roadmap M42 scope, current utility worker state, and recent status evidence. refs=context-prep-roadmap,context-prep-runtime",
		"utility prepared context non-authoritative: true",
		"utility prepared context caveat: prepared context is non-authoritative; foreground work must re-check source refs before acting",
		"utility suggestion: Use prepared context only as a starting point for the next foreground step. refs=context-prep-roadmap,context-prep-runtime",
		"utility evidence: context-prep-roadmap roadmap ROADMAP.md Milestone 42 requires visible non-authoritative utility context prep",
		"utility evidence: context-prep-runtime runtime_state app.status primary runtime idle; context prep allowed by utility scheduler",
		"utility caveat: prepared context is non-authoritative; foreground capability decides whether to use it",
		"utility file mutation: false",
		"utility git mutation: false",
		"utility artifact mutation: false",
		"utility permission approval: false",
		"utility workflow transition: false",
		"utility final judgment: false",
		"inspection: app-owned display data",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("status inspection missing %q in:\n%s", want, lines)
		}
	}
	for _, forbidden := range []string{"Deterministic placeholder status", "real status sources: deferred", "review", "provider review"} {
		if strings.Contains(lines, forbidden) {
			t.Fatalf("status inspection leaked forbidden marker %q in:\n%s", forbidden, lines)
		}
	}
	if len(dispatched) != 2 || len(dispatched[0]) != 1 || len(dispatched[1]) != 1 {
		t.Fatalf("status dispatches = %#v, want command effect then utility effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.FakeCommandEffect); !ok {
		t.Fatalf("status dispatch = %T, want runtime.FakeCommandEffect", dispatched[0][0])
	}
	if _, ok := dispatched[1][0].(runtime.UtilityJobEffect); !ok {
		t.Fatalf("utility dispatch = %T, want runtime.UtilityJobEffect", dispatched[1][0])
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result != "fake command result: status" || got.Utility == nil || got.Utility.Status != "completed" {
		t.Fatalf("status snapshots/view = %#v / %+v, want one snapshot with runtime result and completed utility view", snapshots, got.Utility)
	}
	if len(historyEvents) != 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("status history events = %#v, want command then runtime event", historyEvents)
	}
}

func TestReviewCommandBuildsReadOnlyDiffAndHistoryInspectionSurface(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.RuntimeStatus = "idle"
	view.RuntimeResult = "stable before review"
	var snapshots []SnapshotPersistenceCommand
	var historyReads int
	var diffReads int
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		t.Fatalf("review inspection dispatched runtime effects: %#v", effects)
		return nil
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		if command.Event.Kind != history.EventKindCommand {
			t.Fatalf("review persisted non-command history event: %#v", command.Event)
		}
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		historyReads++
		return HistoryReadResult{
			State: state.FakeHistoryLoaded,
			Events: []history.FakeEvent{{
				SchemaVersion: history.FakeEventSchemaVersion,
				Kind:          history.EventKindMutation,
				EventID:       "evt-review-1",
				RunID:         "run-review",
				SessionID:     "session-review",
				Source:        "mutation.tool",
				Provenance:    "mutation.result",
				DisplayText:   "mutation write completed docs/review.md",
				Mutation: &history.MutationRecord{
					ToolName:     "write",
					Status:       "completed",
					ChangedPaths: []string{"docs/review.md"},
				},
				Undo: &history.UndoMetadata{
					Available: true,
					Action:    "restore_file",
					Paths:     []string{"docs/review.md"},
				},
			}},
		}
	}, func(context.Context, DiffReadCommand) DiffReadResult {
		diffReads++
		return DiffReadResult{View: &tui.DiffView{Source: "test.diff", Status: "ready", Files: []tui.DiffFileView{
			{Path: "internal/demo.txt", Status: "modified"},
			{Path: "docs/review.md", Status: "added"},
		}}}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteReview, Kind: policy.CommandInputSlash}, controller.view)

	if historyReads != 1 || diffReads != 1 {
		t.Fatalf("review reads = history:%d diff:%d, want one each", historyReads, diffReads)
	}
	if got.SurfaceTitle != "review" || got.CommandRoute != "review" || got.RouteSource != "policy.command" {
		t.Fatalf("review command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.RuntimeResult != view.RuntimeResult || got.HistoryFocus || got.DiffFocus {
		t.Fatalf("review mutated unrelated display state: before=%+v after=%+v", view, got)
	}
	lines := strings.Join(got.SurfaceLines, "\n")
	for _, want := range []string{
		"source: app.review",
		"read-only: true",
		"model-assisted review: not invoked",
		"diff source: test.diff",
		"diff status: ready",
		"changed files: 2",
		"changed file: internal/demo.txt status=modified",
		"changed file: docs/review.md status=added",
		"history state: loaded",
		"history events: 1",
		"latest event: mutation mutation.tool mutation write completed docs/review.md",
		"latest mutation: write completed docs/review.md",
		"latest undo action: restore_file",
		"attention: inspect changed files before committing",
		"inspection: app-owned display data",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("review inspection missing %q in:\n%s", want, lines)
		}
	}
	if strings.Contains(lines, "provider") {
		t.Fatalf("review inspection claimed provider work:\n%s", lines)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result != view.RuntimeResult {
		t.Fatalf("review snapshots = %#v, want one snapshot preserving runtime state", snapshots)
	}
}
