package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
)

func TestBuildCommandExecutesOnePlanItemThroughSafetyHistoryAndState(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	view.FooterContext = "M48 build capability"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})
	controller.workspacePath = workspace

	planned := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRoutePlan, Kind: policy.CommandInputSlash}, controller.view)
	if planned.Plan == nil || len(planned.Plan.Items) < 2 {
		t.Fatalf("planned view = %+v", planned.Plan)
	}
	built := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteBuild, Kind: policy.CommandInputSlash}, planned)

	if built.Build == nil {
		t.Fatal("build view is nil")
	}
	if built.Build.PlanItem.ID != "implement" || built.Build.Step.Status != "completed" || built.Build.Operation.Status != "completed" || built.Build.Operation.Path != "docs/aila-build-output.md" {
		t.Fatalf("build view = %+v", built.Build)
	}
	if built.Build.Operation.DecisionSource != "autonomy_policy" || built.Build.Operation.DecisionAutonomy != string(permission.AutonomyWrite) || !built.Build.Operation.DecisionAllowed || built.Build.Operation.ApprovalRequired {
		t.Fatalf("build operation decision = %+v", built.Build.Operation)
	}
	if built.Build.RecommendedSuccessor != "audit" || !built.Build.SuccessorValid || built.Build.TransitionClaimed || !built.Build.DisplayOnly {
		t.Fatalf("build successor/display = %+v", built.Build)
	}
	if built.Mutation == nil || built.Mutation.Status != "completed" || built.Mutation.Path != "docs/aila-build-output.md" {
		t.Fatalf("mutation view = %+v", built.Mutation)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "docs", "aila-build-output.md"))
	if err != nil {
		t.Fatalf("read build output: %v", err)
	}
	if !strings.Contains(string(content), "Plan item: Implement only the scoped plan behavior") || !strings.Contains(string(content), "executed one bounded build step and held") {
		t.Fatalf("build output content = %q", content)
	}
	if runner.model.LastCapability.Build == nil || runner.model.LastCapability.RecommendedSuccessor != "audit" {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) < 2 || snapshots[len(snapshots)-1].Snapshot.Runtime.Result == "" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
	var sawMutation bool
	for _, event := range historyEvents {
		if event.Event.Kind == history.EventKindMutation && event.Event.Mutation != nil && event.Event.Mutation.RequestID == "build-implement" {
			sawMutation = true
			if event.Event.Mutation.Status != "completed" || len(event.Event.Mutation.ChangedPaths) != 1 || event.Event.Mutation.ChangedPaths[0] != "docs/aila-build-output.md" {
				t.Fatalf("mutation history = %+v", event.Event.Mutation)
			}
		}
	}
	if !sawMutation {
		t.Fatalf("history events missing build mutation: %#v", historyEvents)
	}
}

func TestBuildCommandFlagsDeniedWriteWithoutMutation(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyRead)
	view.FooterContext = "M48 build capability"
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		return SnapshotPersistenceResult{}
	}, func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult {
		return HistoryPersistenceResult{}
	})
	controller.workspacePath = workspace

	planned := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRoutePlan, Kind: policy.CommandInputSlash}, controller.view)
	built := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteBuild, Kind: policy.CommandInputSlash}, planned)

	if built.Build == nil || built.Build.Signal != string("flagged") || built.Build.Operation.Status != "denied" || built.Build.Operation.DecisionAllowed || !built.Build.Operation.ApprovalRequired || len(built.Build.Blockers) == 0 {
		t.Fatalf("denied build view = %+v", built.Build)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("denied build created output: %v", err)
	}
	if built.Mutation == nil || built.Mutation.Status != "denied" || runner.model.LastCapability.Build == nil || runner.model.LastCapability.RecommendedSuccessor != "build" {
		t.Fatalf("denied runtime state mutation=%+v capability=%+v", built.Mutation, runner.model.LastCapability)
	}
}

func TestBuildCommandWaitsWithoutActivePlanItem(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Autonomy = string(permission.AutonomyWrite)
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		return SnapshotPersistenceResult{}
	}, func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult {
		return HistoryPersistenceResult{}
	})
	controller.workspacePath = workspace

	built := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteBuild, Kind: policy.CommandInputSlash}, controller.view)

	if built.Build != nil || built.Mutation != nil || runner.model.LastCapability.Signal != "waiting" || runner.model.LastCapability.NeededInput == "" {
		t.Fatalf("waiting build state view=%+v capability=%+v", built.Build, runner.model.LastCapability)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("waiting build created output: %v", err)
	}
}
