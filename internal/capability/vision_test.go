package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestVisionCapabilityEmitsGoalPayloadArtifactAndValidSuccessor(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "vision-1",
		Input: "Aila should stay a focused terminal coding agent.",
		Phase: workflow.PhaseEnvision,
		Metadata: map[string]string{
			VisionMetadataNorthStar:       "Aila helps developers shape, plan, build, and audit focused code changes.",
			VisionMetadataPrinciples:      "Fixed product boundaries|Visible workflow evidence",
			VisionMetadataLongTermGoals:   "Use persisted vision before planning|Keep broad strategy out of build loops",
			VisionMetadataRecommendedNext: workflow.PhasePlan.String(),
			VisionMetadataNextAction:      "Turn the vision into a scoped plan.",
		},
		SourceRefs: []SourceRef{{ID: "roadmap-m50", Kind: "roadmap", Path: "ROADMAP.md", LineStart: 1913, LineEnd: 1935, Excerpt: "Milestone 50: Vision Capability"}},
	}

	payload, err := VisionCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NameVision || payload.Signal != ExitComplete || !payload.Attempted {
		t.Fatalf("vision identity = %+v, want attempted complete vision", payload)
	}
	if payload.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("RecommendedSuccessor = %q, want plan", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 1 || payload.ArtifactRefs[0].Path != defaultVisionArtifactPath {
		t.Fatalf("ArtifactRefs = %+v", payload.ArtifactRefs)
	}
	if payload.Vision == nil {
		t.Fatal("Vision output is nil")
	}
	if payload.Vision.NorthStar != "Aila helps developers shape, plan, build, and audit focused code changes." {
		t.Fatalf("NorthStar = %q", payload.Vision.NorthStar)
	}
	if len(payload.Vision.Principles) != 2 || payload.Vision.Principles[0] != "Fixed product boundaries" {
		t.Fatalf("Principles = %+v", payload.Vision.Principles)
	}
	if len(payload.Vision.LongTermGoals) != 2 || !strings.Contains(payload.Vision.Document, "## Long-term goals") {
		t.Fatalf("vision goals/document = %+v\n%s", payload.Vision.LongTermGoals, payload.Vision.Document)
	}
	if payload.NextAction != "Turn the vision into a scoped plan." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
	if len(payload.SourceRefs) < 1 || payload.SourceRefs[0].ID != "roadmap-m50" {
		t.Fatalf("SourceRefs = %+v", payload.SourceRefs)
	}
	for _, forbidden := range []BoundaryKind{BoundaryModelCall, BoundaryToolExecution, BoundaryPermissionCheck} {
		if hasBoundaryKind(payload.BoundaryRequests, forbidden) {
			t.Fatalf("BoundaryRequests include forbidden execution boundary %s: %+v", forbidden, payload.BoundaryRequests)
		}
	}
	for _, want := range []struct {
		kind   BoundaryKind
		target string
	}{
		{BoundaryStateAccess, "project.current"},
		{BoundaryStateAccess, "session.current"},
		{BoundaryArtifactAccess, "vision"},
		{BoundaryStateWrite, "vision"},
	} {
		if !hasBoundaryRequest(payload.BoundaryRequests, want.kind, want.target) {
			t.Fatalf("BoundaryRequests = %+v, missing %s %s", payload.BoundaryRequests, want.kind, want.target)
		}
	}
}

func TestVisionCapabilityWaitsForDirectionWithoutInventingStrategy(t *testing.T) {
	t.Parallel()

	payload, err := VisionCapability{}.Run(context.Background(), Request{ID: "vision-empty", Phase: workflow.PhaseEnvision})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if payload.Vision != nil || payload.Attempted || strings.Contains(payload.Summary, "research") {
		t.Fatalf("waiting vision invented strategy: %+v", payload)
	}
	if hasBoundaryKind(payload.BoundaryRequests, BoundaryModelCall) || hasBoundaryKind(payload.BoundaryRequests, BoundaryToolExecution) {
		t.Fatalf("waiting vision requested execution: %+v", payload.BoundaryRequests)
	}
}

func TestVisionCapabilityFlagsBlockersAndRoutesThroughValidSuccessor(t *testing.T) {
	t.Parallel()

	payload, err := VisionCapability{}.Run(context.Background(), Request{
		ID:    "vision-blocked",
		Input: "Clarify whether Aila should change product direction.",
		Phase: workflow.PhaseEnvision,
		Metadata: map[string]string{
			VisionMetadataBlockers:        "user confirmation required|decision affects roadmap scope",
			VisionMetadataRecommendedNext: workflow.PhaseDeliberate.String(),
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.RecommendedSuccessor != workflow.PhaseDeliberate || payload.Vision == nil {
		t.Fatalf("flagged payload = %+v", payload)
	}
	if len(payload.Concerns) != 2 || payload.NextAction != "Resolve the vision blockers or deliberate before planning." {
		t.Fatalf("concerns/next action = concerns:%+v next:%q", payload.Concerns, payload.NextAction)
	}
}

func TestVisionCapabilityOmitsInvalidSuccessorRecommendation(t *testing.T) {
	t.Parallel()

	payload, err := VisionCapability{}.Run(context.Background(), Request{
		ID:    "vision-invalid-successor",
		Input: "Shape the goal before building.",
		Phase: workflow.PhasePlan,
		Metadata: map[string]string{
			VisionMetadataRecommendedNext: workflow.PhaseAudit.String(),
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.RecommendedSuccessor != "" {
		t.Fatalf("RecommendedSuccessor = %q, want omitted invalid successor", payload.RecommendedSuccessor)
	}
}

func TestRunBuiltInDispatchesVisionCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameVision,
		Input:      "Shape the current project direction.",
		Phase:      workflow.PhaseEnvision,
	})
	if err != nil {
		t.Fatalf("RunBuiltIn returned error: %v", err)
	}
	if payload.Capability != NameVision || payload.Vision == nil || payload.Signal != ExitComplete {
		t.Fatalf("built-in vision payload = %+v", payload)
	}
}
