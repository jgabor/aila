package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestDesignCapabilityEmitsDesignPayloadWithDecisionsReviewPromptsAndArtifact(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "design-1",
		Capability: NameDesign,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			DesignMetadataGoalID:        "aila-terminal-system",
			DesignMetadataGoalSummary:   "Keep the terminal design system durable for agents and humans.",
			DesignMetadataSurface:       "terminal-ui",
			DesignMetadataDecisions:     "phase-hierarchy::information architecture::Keep active phase and capability visible before detailed evidence.::Users need orientation before inspecting dense output.|review-prompts::visual review::Record explicit review prompts beside decisions.::Screenshots are useful review aids but not correctness contracts.",
			DesignMetadataReviewPrompts: "desktop-rhythm::Does the wide layout preserve content and session hierarchy?::docs/mockup-desktop.png|narrow-clarity::Can 80x24 still show phase, prompt, and caveats?::docs/mockup-mobile.png",
			DesignMetadataCaveats:       "deterministic app-supplied design evidence only|no screenshot correctness contract",
			DesignMetadataNextAction:    "Audit the design-system artifact before continuing.",
			DesignMetadataDurable:       "true",
		},
		SourceRefs: []SourceRef{{ID: "design-command", Kind: "command", Command: "/design", Excerpt: "app-owned design command"}},
	}

	payload, err := DesignCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run design capability: %v", err)
	}
	if payload.Capability != NameDesign || payload.Signal != ExitComplete || payload.Design == nil {
		t.Fatalf("design identity = %+v", payload)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit {
		t.Fatalf("RecommendedSuccessor = %q, want audit", payload.RecommendedSuccessor)
	}
	if len(payload.Design.Decisions) != 2 || len(payload.Design.ReviewPrompts) != 2 || len(payload.Design.Caveats) != 2 {
		t.Fatalf("design output = %+v", payload.Design)
	}
	if payload.Design.DesignArtifactPath != defaultDesignArtifactPath || !strings.Contains(payload.Design.DesignArtifact, "# Design System") || !strings.Contains(payload.Design.DesignArtifact, "no screenshot correctness contract") {
		t.Fatalf("artifact path/content = %q\n%s", payload.Design.DesignArtifactPath, payload.Design.DesignArtifact)
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryStateWrite, "design") || len(payload.ArtifactRefs) == 0 || payload.ArtifactRefs[0].Kind != "design" {
		t.Fatalf("artifact refs/boundaries = refs:%+v boundaries:%+v", payload.ArtifactRefs, payload.BoundaryRequests)
	}
}

func TestDesignCapabilityWaitsForGoalAndDecisionEvidence(t *testing.T) {
	t.Parallel()

	payload, err := DesignCapability{}.Run(context.Background(), Request{ID: "design-waiting", Capability: NameDesign})
	if err != nil {
		t.Fatalf("Run design capability: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.Design != nil || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryArtifactAccess, "design") {
		t.Fatalf("BoundaryRequests = %+v, missing design artifact boundary", payload.BoundaryRequests)
	}
}

func TestDesignCapabilityNormalizesBuildOwnershipAndSinglePayload(t *testing.T) {
	t.Parallel()

	payload, err := DesignCapability{}.Run(context.Background(), Request{
		ID:         "design-normalized",
		Capability: NameDesign,
		Metadata: map[string]string{
			DesignMetadataGoalSummary: "Make design evidence durable.",
			DesignMetadataDecisions:   "durable-record::artifact::Persist design decisions through state.Store.::The TUI should display evidence but not own persistence.",
			DesignMetadataDurable:     "true",
		},
	})
	if err != nil {
		t.Fatalf("Run design capability: %v", err)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit || payload.Design == nil || len(payload.Design.Decisions) != 1 {
		t.Fatalf("normalized payload = %+v", payload)
	}

	invocation := NewInvocation(Request{ID: "single", Capability: NameDesign})
	if _, err := invocation.Emit(payload); err != nil {
		t.Fatalf("first Emit: %v", err)
	}
	if _, err := invocation.Emit(payload); err == nil {
		t.Fatal("second Emit returned nil error")
	}
}

func TestRunBuiltInDispatchesDesignCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameDesign,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			DesignMetadataGoalSummary: "Make design evidence durable.",
			DesignMetadataDecisions:   "review-prompt::visual review::Keep review prompts explicit.::Automated snapshots are regression evidence only.",
		},
	})
	if err != nil {
		t.Fatalf("RunBuiltIn design: %v", err)
	}
	if payload.Capability != NameDesign || payload.Design == nil {
		t.Fatalf("RunBuiltIn design payload = %+v", payload)
	}
}
