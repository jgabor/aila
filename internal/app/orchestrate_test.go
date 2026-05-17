package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestOrchestrateCommandCoordinatesCyclesRetriesAndSupervisedChildren(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.RuntimeStatus = "idle"
	view.Plan = &tui.PlanView{Items: []tui.PlanItemView{{ID: "task-2", Text: "Add bounded orchestrate capability output", Done: false}}}
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	runner := newInputRunnerWithDispatch(runtime.Dispatch)
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	orchestrated := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteOrchestrate, Kind: policy.CommandInputSlash}, controller.view)

	if orchestrated.Orchestrate == nil {
		t.Fatalf("orchestrate view missing: %+v", orchestrated)
	}
	got := orchestrated.Orchestrate
	if got.Signal != "complete" || got.Goal.ID != "task-2" || got.ActiveCycle != "cycle-2" || len(got.Cycles) != 2 || len(got.ChildWork) != 3 || len(got.Decisions) != 2 || len(got.Evidence) != 2 {
		t.Fatalf("orchestrate view = %+v", got)
	}
	if got.RetryBudget.MaxAttempts != 1 || got.RetryBudget.Used != 1 || got.RetryBudget.Remaining != 0 || got.RecommendedSuccessor != "audit" || !got.SuccessorValid || got.TransitionClaimed || !got.DisplayOnly {
		t.Fatalf("orchestrate retry/successor/display = %+v", got)
	}
	if len(orchestrated.Subagents) != 3 || !containsSubagentViewStatus(orchestrated.Subagents, "failed") || !containsSubagentViewStatus(orchestrated.Subagents, "completed") {
		t.Fatalf("orchestrated subagents = %+v", orchestrated.Subagents)
	}
	if runner.model.LastCapability.Orchestrate == nil || runner.model.LastCapability.RecommendedSuccessor != "audit" {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) == 0 || snapshots[len(snapshots)-1].Snapshot.Runtime.Result == "" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
	if len(historyEvents) == 0 {
		t.Fatal("orchestrate command did not record command/runtime history")
	}
}

func TestOrchestrateHeldRunCanBeCanceledWithoutLowerLayerClaims(t *testing.T) {
	t.Setenv("AILA_FAKE_ORCHESTRATE_HOLD_ACTIVE", "1")

	ctx := context.Background()
	runner := newInputRunnerHoldingFakeWorkWithSecondInterruptResolutionContext(ctx)
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	controller := newSessionControllerWithPersistence(ctx, view, runner, nil)

	active := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteOrchestrate, Kind: policy.CommandInputSlash}, controller.view)
	if active.Orchestrate == nil || active.Orchestrate.Status != "running" || active.RuntimeStatus != "active" || !active.RuntimeActive {
		t.Fatalf("active orchestrate view = %+v", active.Orchestrate)
	}

	turn := controller.requestInterrupt("test cancellation")
	canceling := tui.ApplyTranscriptTurn(active, turn)
	if canceling.RuntimeStatus != "canceling" || !canceling.RuntimeActive || canceling.RuntimeResult != "" {
		t.Fatalf("canceling view status=%q active=%v result=%q", canceling.RuntimeStatus, canceling.RuntimeActive, canceling.RuntimeResult)
	}
	turn = controller.requestInterrupt("resolve fake cancellation")
	canceled := tui.ApplyTranscriptTurn(canceling, turn)
	if canceled.RuntimeStatus != "canceled" || canceled.RuntimeActive || !strings.Contains(canceled.RuntimeResult, "fake work canceled") {
		t.Fatalf("canceled view status=%q active=%v result=%q", canceled.RuntimeStatus, canceled.RuntimeActive, canceled.RuntimeResult)
	}
	if strings.Contains(strings.Join(canceled.SurfaceLines, "\n"), "lower-layer cancellation executed: true") {
		t.Fatalf("surface claimed lower-layer cancellation: %+v", canceled.SurfaceLines)
	}
}

func containsSubagentViewStatus(views []tui.SubagentView, status string) bool {
	for _, view := range views {
		if view.Status == status {
			return true
		}
	}
	return false
}
