package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestDesignCommandPersistsDesignArtifactThroughStateStore(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.RuntimeStatus = "idle"
	view.StatusSource = "test"
	view.RuntimeResult = "ready"
	controller := newController(context.Background(), workspace, view, newInputRunnerWithDispatch(runtime.Dispatch))
	designed := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteDesign, Kind: policy.CommandInputSlash}, controller.view)

	if designed.Design == nil {
		t.Fatalf("Design view missing: %+v", designed)
	}
	if designed.Design.Signal != "complete" || designed.Design.Goal.ID != "aila-terminal-design-system" || len(designed.Design.Decisions) != 3 || len(designed.Design.ReviewPrompts) != 2 {
		t.Fatalf("design view = %+v", designed.Design)
	}
	if designed.Design.RecommendedSuccessor != "audit" || !designed.Design.SuccessorValid || designed.Design.TransitionClaimed || !designed.Design.DisplayOnly {
		t.Fatalf("design successor/display = %+v", designed.Design)
	}
	if designed.Design.ArtifactStatus != "written" || len(designed.Design.ArtifactRefs) == 0 || len(designed.Design.BoundaryRequests) == 0 {
		t.Fatalf("design refs = %+v", designed.Design)
	}

	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "design.md"))
	if err != nil {
		t.Fatalf("read design artifact: %v", err)
	}
	for _, want := range []string{"# Design System", "phase-hierarchy", "Visual Review Prompts", "screenshots are review aids"} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("design artifact missing %q:\n%s", want, artifact)
		}
	}
	if controller.runner.model.LastCapability.Design == nil || controller.runner.model.LastCapability.RecommendedSuccessor != "audit" {
		t.Fatalf("last capability = %+v", controller.runner.model.LastCapability)
	}
}

func TestDesignViewMapsWaitingPayloadWithoutArtifactWrite(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	controller := newController(context.Background(), workspace, snapshotTestView(), newInputRunnerWithDispatch(runtime.Dispatch))
	request := capability.Request{ID: "design-waiting", Capability: capability.NameDesign}
	turn := controller.runner.proposeCapability(request)
	turn.Design = designView(controller.runner.model.LastCapability, request.Phase, designArtifactPersistence{})
	view := tui.ApplyTranscriptTurn(controller.view, turn)

	if view.Design == nil || view.Design.Signal != "waiting" || view.Design.NeededInput == "" || view.Design.ArtifactStatus != "available" {
		t.Fatalf("waiting design view = %+v", view.Design)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".aila", "artifacts", "design.md")); !os.IsNotExist(err) {
		t.Fatalf("design artifact stat error = %v, want missing", err)
	}
}
