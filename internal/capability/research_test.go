package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestResearchCapabilityEmitsCrossCuttingContextPayloadWithoutSuccessor(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "research-1",
		Input: "Research terminal agent context-folding patterns.",
		Phase: workflow.PhaseBuild,
		Metadata: map[string]string{
			ResearchMetadataTopic:          "terminal agent context-folding patterns",
			ResearchMetadataContext:        "M51 needs external-pattern evidence without workflow authority.",
			ResearchMetadataPatterns:       "Cross-cutting helpers return context evidence|Source refs travel with condensed claims",
			ResearchMetadataEvidence:       "docs/workflow-architecture keeps research cross-cutting|ARCHITECTURE requires FSM-owned transitions",
			ResearchMetadataConfidence:     "high",
			ResearchMetadataCaveats:        "live external fetching deferred|app-supplied evidence only",
			ResearchMetadataNextAction:     "Use the research as non-authoritative context for planning.",
			ResearchMetadataContextSummary: "research refs ready for context folding",
		},
		SourceRefs: []SourceRef{{ID: "workflow-research", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 264, LineEnd: 279, Excerpt: "research is cross-cutting"}},
	}

	payload, err := ResearchCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NameResearch || payload.Signal != ExitComplete || !payload.Attempted {
		t.Fatalf("research identity = %+v, want attempted complete research", payload)
	}
	if payload.RecommendedSuccessor != "" {
		t.Fatalf("RecommendedSuccessor = %q, want none for cross-cutting research", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 0 {
		t.Fatalf("ArtifactRefs = %+v, want no project artifact writes", payload.ArtifactRefs)
	}
	if payload.Research == nil {
		t.Fatal("Research output is nil")
	}
	if payload.Research.Topic != "terminal agent context-folding patterns" || payload.Research.CurrentPhase != workflow.PhaseBuild.String() || payload.Research.CrossCuttingStatus != "context_only" {
		t.Fatalf("research output identity = %+v", payload.Research)
	}
	if len(payload.Research.Patterns) != 2 || len(payload.Research.Evidence) != 2 || payload.Research.Confidence != "high" {
		t.Fatalf("research evidence = %+v", payload.Research)
	}
	if len(payload.Research.Caveats) != 2 || len(payload.Concerns) != 2 || payload.NextAction != "Use the research as non-authoritative context for planning." {
		t.Fatalf("caveats/next action = concerns:%+v next:%q", payload.Concerns, payload.NextAction)
	}
	if payload.Research.ContextSummary != "research refs ready for context folding" {
		t.Fatalf("ContextSummary = %q", payload.Research.ContextSummary)
	}
	if len(payload.SourceRefs) < 2 || payload.SourceRefs[0].ID != "workflow-research" {
		t.Fatalf("SourceRefs = %+v", payload.SourceRefs)
	}
	for _, forbidden := range []BoundaryKind{BoundaryModelCall, BoundaryToolExecution, BoundaryPermissionCheck, BoundaryArtifactAccess, BoundaryStateWrite} {
		if hasBoundaryKind(payload.BoundaryRequests, forbidden) {
			t.Fatalf("BoundaryRequests include forbidden boundary %s: %+v", forbidden, payload.BoundaryRequests)
		}
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryContextAccess, "current_context") || !hasBoundaryRequest(payload.BoundaryRequests, BoundaryStateAccess, "session.current") {
		t.Fatalf("BoundaryRequests = %+v, missing context/session boundaries", payload.BoundaryRequests)
	}
}

func TestResearchCapabilityWaitsForTopicWithoutInventingEvidence(t *testing.T) {
	t.Parallel()

	payload, err := ResearchCapability{}.Run(context.Background(), Request{ID: "research-empty", Phase: workflow.PhasePlan})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if payload.Research != nil || payload.Attempted || len(payload.Concerns) == 0 || strings.Contains(payload.Summary, "optimize") {
		t.Fatalf("waiting research invented evidence: %+v", payload)
	}
	if hasBoundaryKind(payload.BoundaryRequests, BoundaryModelCall) || hasBoundaryKind(payload.BoundaryRequests, BoundaryToolExecution) || hasBoundaryKind(payload.BoundaryRequests, BoundaryStateWrite) {
		t.Fatalf("waiting research requested execution or write: %+v", payload.BoundaryRequests)
	}
}

func TestRunBuiltInDispatchesResearchCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameResearch,
		Input:      "Research context-folding patterns.",
		Phase:      workflow.PhaseAudit,
	})
	if err != nil {
		t.Fatalf("RunBuiltIn returned error: %v", err)
	}
	if payload.Capability != NameResearch || payload.Research == nil || payload.Signal != ExitComplete || payload.RecommendedSuccessor != "" {
		t.Fatalf("built-in research payload = %+v", payload)
	}
}
