package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
)

func TestOptimizeCommandPersistsMeasuredObjectiveAndExperimentThroughStateStore(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.FooterContext = "M52 optimize capability"
	var snapshots []SnapshotPersistenceCommand
	runner := newInputRunnerWithDispatch(runtime.Dispatch)
	controller := newSessionControllerWithPersistence(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	})
	controller.workspacePath = workspace

	optimized := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteOptimize, Kind: policy.CommandInputSlash}, controller.view)

	if optimized.Optimize == nil {
		t.Fatal("optimize view is nil")
	}
	if optimized.Optimize.Signal != "complete" || optimized.Optimize.Objective.ID != "current-metric-objective" || optimized.Optimize.Experiment.Status != "improved" || !optimized.Optimize.Harness.Locked || optimized.Optimize.Metric.Name != "render_evidence_seconds" {
		t.Fatalf("optimize view = %+v", optimized.Optimize)
	}
	if optimized.Optimize.RecommendedSuccessor != "audit" || !optimized.Optimize.SuccessorValid || optimized.Optimize.TransitionClaimed || !optimized.Optimize.DisplayOnly {
		t.Fatalf("optimize successor/display = %+v", optimized.Optimize)
	}
	if optimized.Optimize.ArtifactStatus != "written" || len(optimized.Optimize.ArtifactRefs) != 2 || len(optimized.Optimize.BoundaryRequests) == 0 || len(optimized.Optimize.Evidence) == 0 {
		t.Fatalf("optimize refs/evidence = %+v", optimized.Optimize)
	}
	objectiveContent, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "objective.md"))
	if err != nil {
		t.Fatalf("read objective artifact: %v", err)
	}
	if !strings.Contains(string(objectiveContent), "Reduce evidence rendering latency") {
		t.Fatalf("objective artifact content = %q", objectiveContent)
	}
	experimentContent, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "experiments.md"))
	if err != nil {
		t.Fatalf("read experiments artifact: %v", err)
	}
	if !strings.Contains(string(experimentContent), "render_evidence_seconds") || !strings.Contains(string(experimentContent), "1.50s -> 1.20s") {
		t.Fatalf("experiment artifact content = %q", experimentContent)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("optimize created build output: %v", err)
	}
	if optimized.Mutation != nil {
		t.Fatalf("optimize exposed mutation view: %+v", optimized.Mutation)
	}
	if runner.model.LastCapability.Optimize == nil || runner.model.LastCapability.RecommendedSuccessor != "audit" {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) == 0 || snapshots[len(snapshots)-1].Snapshot.Runtime.Result == "" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
}
