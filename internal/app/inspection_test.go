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
	var historyReads int
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
		historyReads++
		return HistoryReadResult{
			State: state.FakeHistoryLoaded,
			Events: []history.FakeEvent{{
				SchemaVersion: history.FakeEventSchemaVersion,
				Kind:          history.EventKindRuntime,
				EventID:       "event-status-1",
				RunID:         "run-status",
				SessionID:     "session-status",
				Source:        "runtime.dispatch",
				Provenance:    "runtime.dispatch",
				DisplayText:   "runtime idle before brief",
			}},
		}
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
		"runtime detail: brief capability status",
		"runtime result: Brief: phase idle, runtime idle, store initialized, history loaded (1 events), context unavailable (placeholder), health available.",
		"last command: status",
		"project store: initialized (state.open; project store ready)",
		"utility worker: completed",
		"utility source: app.status",
		"utility job: summary_refresh status-summary-refresh",
		"utility summary: summary refresh confidence low",
		"utility summary refresh: low_confidence",
		"utility original summary: Status output is available for the current runtime.",
		"utility refreshed summary: Status output is available for the current runtime. Important details: primary runtime remains idle; utility worker can refresh summaries without final judgment refs=summary-refresh-runtime,summary-refresh-roadmap",
		"utility summary refresh source refs: summary-refresh-runtime,summary-refresh-roadmap",
		"utility summary refresh confidence: low",
		"utility summary refresh detail: primary runtime remains idle",
		"utility summary refresh detail: utility worker can refresh summaries without final judgment",
		"utility summary refresh caveat: refresh confidence is low; foreground work must check source refs before using refreshed summary",
		"utility suggestion: Review preserved source refs before using the refreshed summary. refs=summary-refresh-source-1,summary-refresh-source-2,summary-refresh-detail-1,summary-refresh-detail-2",
		"utility evidence: summary-refresh-source-1 source_ref app.status source_ref=summary-refresh-runtime",
		"utility evidence: summary-refresh-detail-2 exact_detail app.status utility worker can refresh summaries without final judgment",
		"utility caveat: refresh confidence is low; foreground work must check source refs before using refreshed summary",
		"utility file mutation: false",
		"utility git mutation: false",
		"utility artifact mutation: false",
		"utility permission approval: false",
		"utility workflow transition: false",
		"utility final judgment: false",
		"utility context refresh: false",
		"utility context compaction: false",
		"utility context rewrite: false",
		"brief capability: brief complete",
		"brief source: app.brief",
		"brief current phase: idle",
		"brief runtime status: idle",
		"brief transition claimed: false",
		"brief display-only: true",
		"brief known gap: context unavailable",
		"brief suggested next action: Continue with the current roadmap task or choose the next capability.",
		"brief requested boundary: state_access operation=state.access target=runtime.current",
		"brief requested boundary: artifact_access operation=artifact.access target=fake_history",
		"brief source ref: brief-history kind=history excerpt=runtime runtime.dispatch runtime idle before brief",
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
	if historyReads != 1 {
		t.Fatalf("status history reads = %d, want one brief evidence read", historyReads)
	}
	if len(dispatched) != 3 || len(dispatched[0]) != 1 || len(dispatched[1]) != 1 || len(dispatched[2]) != 1 {
		t.Fatalf("status dispatches = %#v, want command effect, utility effect, then capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.FakeCommandEffect); !ok {
		t.Fatalf("status dispatch = %T, want runtime.FakeCommandEffect", dispatched[0][0])
	}
	if _, ok := dispatched[1][0].(runtime.UtilityJobEffect); !ok {
		t.Fatalf("utility dispatch = %T, want runtime.UtilityJobEffect", dispatched[1][0])
	}
	if _, ok := dispatched[2][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("capability dispatch = %T, want runtime.CapabilityEffect", dispatched[2][0])
	}
	if len(snapshots) != 1 || !strings.Contains(snapshots[0].Snapshot.Runtime.Result, "Brief: phase idle") || got.Utility == nil || got.Utility.Status != "completed" || got.Brief == nil || got.Brief.SuggestedNextAction == "" {
		t.Fatalf("status snapshots/view = %#v / utility=%+v brief=%+v, want one snapshot with brief result, completed utility view, and brief view", snapshots, got.Utility, got.Brief)
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
