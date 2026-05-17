package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestDiscussCapabilityEmitsDecisionPayloadArtifactAndValidSuccessor(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "discuss-1",
		Input: "Should Aila plan or build next?",
		Phase: workflow.PhaseDeliberate,
		Metadata: map[string]string{
			DiscussMetadataQuestion:        "Should Aila plan or build next?",
			DiscussMetadataContext:         "M50A needs a bounded discussion record before later workflow work.",
			DiscussMetadataOptions:         "Plan the scoped next step|Proceed directly to build|Revisit vision",
			DiscussMetadataSelected:        "Plan the scoped next step",
			DiscussMetadataReasoning:       "Planning preserves workflow authority and keeps build work scoped.",
			DiscussMetadataConfidence:      "high",
			DiscussMetadataRecommendedNext: workflow.PhasePlan.String(),
			DiscussMetadataNextAction:      "Turn this decision into a scoped plan.",
		},
		SourceRefs: []SourceRef{{ID: "roadmap-m50a", Kind: "roadmap", Path: "ROADMAP.md", LineStart: 1938, LineEnd: 1963, Excerpt: "Milestone 50A: Discuss Capability"}},
	}

	payload, err := DiscussCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NameDiscuss || payload.Signal != ExitComplete || !payload.Attempted {
		t.Fatalf("discuss identity = %+v, want attempted complete discuss", payload)
	}
	if payload.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("RecommendedSuccessor = %q, want plan", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 1 || payload.ArtifactRefs[0].Path != defaultDiscussArtifactPath {
		t.Fatalf("ArtifactRefs = %+v", payload.ArtifactRefs)
	}
	if payload.Discuss == nil {
		t.Fatal("Discuss output is nil")
	}
	if payload.Discuss.Question != "Should Aila plan or build next?" || payload.Discuss.Selected != "Plan the scoped next step" {
		t.Fatalf("decision = %+v", payload.Discuss)
	}
	if len(payload.Discuss.Options) != 3 || !payload.Discuss.Options[0].Selected {
		t.Fatalf("Options = %+v", payload.Discuss.Options)
	}
	if payload.Discuss.Confidence != "high" || !strings.Contains(payload.Discuss.Document, "## Options") || !strings.Contains(payload.Discuss.Document, "Choice: Plan the scoped next step") {
		t.Fatalf("confidence/document = %q\n%s", payload.Discuss.Confidence, payload.Discuss.Document)
	}
	if payload.NextAction != "Turn this decision into a scoped plan." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
	if len(payload.SourceRefs) < 1 || payload.SourceRefs[0].ID != "roadmap-m50a" {
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
		{BoundaryArtifactAccess, "decisions"},
		{BoundaryStateWrite, "decisions"},
	} {
		if !hasBoundaryRequest(payload.BoundaryRequests, want.kind, want.target) {
			t.Fatalf("BoundaryRequests = %+v, missing %s %s", payload.BoundaryRequests, want.kind, want.target)
		}
	}
}

func TestDiscussCapabilityWaitsForDecisionWithoutInventingChoice(t *testing.T) {
	t.Parallel()

	payload, err := DiscussCapability{}.Run(context.Background(), Request{ID: "discuss-empty", Phase: workflow.PhaseDeliberate})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if payload.Discuss != nil || payload.Attempted || strings.Contains(payload.Summary, "research") {
		t.Fatalf("waiting discuss invented decision: %+v", payload)
	}
	if hasBoundaryKind(payload.BoundaryRequests, BoundaryModelCall) || hasBoundaryKind(payload.BoundaryRequests, BoundaryToolExecution) {
		t.Fatalf("waiting discuss requested execution: %+v", payload.BoundaryRequests)
	}
}

func TestDiscussCapabilityFlagsBlockersAndRoutesThroughValidSuccessor(t *testing.T) {
	t.Parallel()

	payload, err := DiscussCapability{}.Run(context.Background(), Request{
		ID:    "discuss-blocked",
		Input: "Should Aila continue planning before user confirmation?",
		Phase: workflow.PhaseDeliberate,
		Metadata: map[string]string{
			DiscussMetadataBlockers:        "user confirmation required|decision affects roadmap scope",
			DiscussMetadataRecommendedNext: workflow.PhaseEnvision.String(),
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.RecommendedSuccessor != workflow.PhaseEnvision || payload.Discuss == nil {
		t.Fatalf("flagged payload = %+v", payload)
	}
	if len(payload.Concerns) != 2 || payload.NextAction != "Resolve the decision blockers before changing workflow direction." {
		t.Fatalf("concerns/next action = concerns:%+v next:%q", payload.Concerns, payload.NextAction)
	}
}

func TestDiscussCapabilityOmitsInvalidSuccessorRecommendation(t *testing.T) {
	t.Parallel()

	payload, err := DiscussCapability{}.Run(context.Background(), Request{
		ID:    "discuss-invalid-successor",
		Input: "Should Aila jump to audit?",
		Phase: workflow.PhaseDeliberate,
		Metadata: map[string]string{
			DiscussMetadataRecommendedNext: workflow.PhaseAudit.String(),
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.RecommendedSuccessor != "" {
		t.Fatalf("RecommendedSuccessor = %q, want omitted invalid successor", payload.RecommendedSuccessor)
	}
}

func TestRunBuiltInDispatchesDiscussCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameDiscuss,
		Input:      "Decide whether to plan next.",
		Phase:      workflow.PhaseDeliberate,
	})
	if err != nil {
		t.Fatalf("RunBuiltIn returned error: %v", err)
	}
	if payload.Capability != NameDiscuss || payload.Discuss == nil || payload.Signal != ExitComplete {
		t.Fatalf("built-in discuss payload = %+v", payload)
	}
}
