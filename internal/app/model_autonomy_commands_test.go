package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
)

func TestModelCommandBuildsSessionScopedReadinessSelection(t *testing.T) {
	t.Parallel()

	var snapshots []SnapshotPersistenceCommand
	var dispatched [][]runtime.Effect
	view := snapshotTestView()
	view.PrimaryModel = defaultPrimaryModel
	view.UtilityModel = defaultUtilityModel
	view.Autonomy = string(permission.AutonomyWrite)
	runner := newInputRunnerWithDispatchAndAgentConfig(t.Context(), func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	}, agent.DefaultFakeBuildRunner(), "fake", "fake-build", []string{"read", "write"})
	controller := newSessionControllerWithPersistence(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteModel, Kind: policy.CommandInputSlash, Target: policy.CommandTargetPrimaryModel}, controller.view)
	if got.ModelSwitch == nil || got.ModelSwitch.Target != string(policy.CommandTargetPrimaryModel) || got.ModelSwitch.Source != "app.model" || !got.ModelSwitch.Focus {
		t.Fatalf("model switch = %+v, want focused app-owned primary selector", got.ModelSwitch)
	}
	lines := strings.Join(got.SurfaceLines, "\n")
	for _, want := range []string{"source: app.model", "target: primary_model", "current primary: " + defaultPrimaryModel, "current utility: " + defaultUtilityModel, "credential_source=device-code", "status=degraded", "status=unavailable", "app-owned", "display-only"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("model switch missing %q in:\n%s", want, lines)
		}
	}
	if len(dispatched) != 0 {
		t.Fatalf("model selection opening dispatched runtime effects: %#v", dispatched)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Concerns[0].Source != "display.status" {
		t.Fatalf("model snapshots = %#v, want one current-session snapshot", snapshots)
	}

	got = controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteModel, Kind: policy.CommandInputSelection, Target: policy.CommandTargetPrimaryModel, Selection: "openai/o4-mini"}, controller.view)
	if got.PrimaryModel != "openai/o4-mini" || got.UtilityModel != defaultUtilityModel {
		t.Fatalf("model labels after selection = primary %q utility %q", got.PrimaryModel, got.UtilityModel)
	}
	if controller.runner.agent == nil || controller.runner.agent.provider != "openai" || controller.runner.agent.model != "o4-mini" {
		t.Fatalf("agent model metadata = %+v, want selected provider/model", controller.runner.agent)
	}
	if got.ModelSwitch == nil || got.ModelSwitch.CurrentPrimary != "openai/o4-mini" || !strings.Contains(strings.Join(got.SurfaceLines, "\n"), "config file unchanged") {
		t.Fatalf("selected model switch = %+v lines=%v", got.ModelSwitch, got.SurfaceLines)
	}
}

func TestUtilityModelCommandUpdatesOnlyUtilitySessionLabel(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.PrimaryModel = defaultPrimaryModel
	view.UtilityModel = defaultUtilityModel
	view.Autonomy = string(permission.AutonomyRead)
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		t.Fatalf("utility model command dispatched effects: %#v", effects)
		return nil
	})
	controller := newSessionControllerWithPersistence(context.Background(), view, runner, nil)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteModel, Kind: policy.CommandInputSelection, Target: policy.CommandTargetUtilityModel, Selection: "copilot/copilot-fast"}, controller.view)
	if got.PrimaryModel != defaultPrimaryModel || got.UtilityModel != "copilot/copilot-fast" {
		t.Fatalf("utility selection labels = primary %q utility %q", got.PrimaryModel, got.UtilityModel)
	}
	if got.ModelSwitch == nil || got.ModelSwitch.Target != string(policy.CommandTargetUtilityModel) || got.ModelSwitch.CurrentUtility != "copilot/copilot-fast" {
		t.Fatalf("utility model switch = %+v", got.ModelSwitch)
	}
}

func TestAutonomyCommandUpdatesSessionLabelAndPermissionDispatch(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.PrimaryModel = defaultPrimaryModel
	view.UtilityModel = defaultUtilityModel
	view.Autonomy = string(permission.AutonomyWrite)
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	controller := newSessionControllerWithPersistence(context.Background(), view, runner, nil)
	controller.workspacePath = workspace
	controller.autonomyLevel = string(permission.AutonomyWrite)

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteAuto, Kind: policy.CommandInputSelection, Target: policy.CommandTargetAutonomy, Selection: string(permission.AutonomyRead)}, controller.view)
	if got.Autonomy != string(permission.AutonomyRead) || controller.autonomyLevel != string(permission.AutonomyRead) {
		t.Fatalf("autonomy after selection = view %q controller %q", got.Autonomy, controller.autonomyLevel)
	}
	if got.AutonomySwitch == nil || got.AutonomySwitch.Current != string(permission.AutonomyRead) || !strings.Contains(strings.Join(got.SurfaceLines, "\n"), "config file unchanged") {
		t.Fatalf("autonomy switch after selection = %+v lines=%v", got.AutonomySwitch, got.SurfaceLines)
	}

	turn := controller.runner.proposeWriteTool(runtime.MutationToolRequest{Path: "notes.txt", TargetVersion: "missing", Content: "hello\n", ExpectedEffect: "create notes"})
	if turn.Mutation == nil || turn.Mutation.Status != "denied" || turn.Mutation.Decision == nil || turn.Mutation.Decision.Autonomy != string(permission.AutonomyRead) || !turn.Mutation.Decision.ApprovalRequired {
		t.Fatalf("write turn after autonomy switch = %+v mutation=%+v", turn, turn.Mutation)
	}
	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("read-autonomy write created file: %v", err)
	}
}
