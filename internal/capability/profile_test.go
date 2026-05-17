package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestProfileCapabilityEmitsCrossCuttingProfilePayloadWithDurableArtifact(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "profile-1",
		Input: "Profile Aila decision patterns.",
		Phase: workflow.PhasePlan,
		Metadata: map[string]string{
			ProfileMetadataSubject:           "Aila decision profile",
			ProfileMetadataContext:           "M51A needs decision-profile context without workflow authority.",
			ProfileMetadataDecisionSignals:   "Prefer bounded roadmap slices before broad refactors|Keep explicit validation evidence with closeout",
			ProfileMetadataUpdateSuggestions: "Remember to verify behavior in the real command path|Prefer behavior-named tests over milestone names",
			ProfileMetadataEvidence:          "Prior roadmap cycles used planera before implementation|Recent validation caught stale milestone-numbered test names",
			ProfileMetadataConfidence:        "medium",
			ProfileMetadataCaveats:           "profile uses app-supplied session evidence only|provider-backed corpus analysis deferred",
			ProfileMetadataNextAction:        "Use the profile as non-authoritative context for the current phase.",
			ProfileMetadataContextSummary:    "profile refs ready for context folding",
			ProfileMetadataDurable:           "true",
		},
		SourceRefs: []SourceRef{{ID: "profile-session", Kind: "session", Excerpt: "planera before implementation"}},
	}

	payload, err := ProfileCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NameProfile || payload.Signal != ExitComplete || !payload.Attempted {
		t.Fatalf("profile identity = %+v, want attempted complete profile", payload)
	}
	if payload.RecommendedSuccessor != "" {
		t.Fatalf("RecommendedSuccessor = %q, want none for cross-cutting profile", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 1 || payload.ArtifactRefs[0].Path != defaultProfileArtifactPath {
		t.Fatalf("ArtifactRefs = %+v", payload.ArtifactRefs)
	}
	if payload.Profile == nil {
		t.Fatal("Profile output is nil")
	}
	if payload.Profile.Subject != "Aila decision profile" || payload.Profile.CurrentPhase != workflow.PhasePlan.String() || payload.Profile.CrossCuttingStatus != "context_only" {
		t.Fatalf("profile output identity = %+v", payload.Profile)
	}
	if len(payload.Profile.DecisionSignals) != 2 || len(payload.Profile.UpdateSuggestions) != 2 || len(payload.Profile.Evidence) != 2 || payload.Profile.Confidence != "medium" {
		t.Fatalf("profile evidence = %+v", payload.Profile)
	}
	if len(payload.Profile.Caveats) != 2 || len(payload.Concerns) != 2 || payload.NextAction != "Use the profile as non-authoritative context for the current phase." {
		t.Fatalf("caveats/next action = concerns:%+v next:%q", payload.Concerns, payload.NextAction)
	}
	if !strings.Contains(payload.Profile.Document, "# Profile") || !strings.Contains(payload.Profile.Document, "Prefer bounded roadmap slices") {
		t.Fatalf("profile document missing expected content:\n%s", payload.Profile.Document)
	}
	for _, want := range []struct {
		kind   BoundaryKind
		target string
	}{
		{BoundaryContextAccess, "current_context"},
		{BoundaryStateAccess, "session.current"},
		{BoundaryArtifactAccess, "profile"},
		{BoundaryStateWrite, "profile"},
	} {
		if !hasBoundaryRequest(payload.BoundaryRequests, want.kind, want.target) {
			t.Fatalf("BoundaryRequests = %+v, missing %s %s", payload.BoundaryRequests, want.kind, want.target)
		}
	}
	for _, forbidden := range []BoundaryKind{BoundaryModelCall, BoundaryToolExecution, BoundaryPermissionCheck} {
		if hasBoundaryKind(payload.BoundaryRequests, forbidden) {
			t.Fatalf("BoundaryRequests include forbidden boundary %s: %+v", forbidden, payload.BoundaryRequests)
		}
	}
}

func TestProfileCapabilityWaitsForEvidenceWithoutInventingProfile(t *testing.T) {
	t.Parallel()

	payload, err := ProfileCapability{}.Run(context.Background(), Request{ID: "profile-empty", Phase: workflow.PhaseBuild, Input: "Profile Aila."})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if payload.Profile != nil || payload.Attempted || len(payload.Concerns) == 0 || strings.Contains(payload.Summary, "research") {
		t.Fatalf("waiting profile invented evidence: %+v", payload)
	}
	if hasBoundaryKind(payload.BoundaryRequests, BoundaryModelCall) || hasBoundaryKind(payload.BoundaryRequests, BoundaryToolExecution) || hasBoundaryKind(payload.BoundaryRequests, BoundaryStateWrite) {
		t.Fatalf("waiting profile requested execution or write: %+v", payload.BoundaryRequests)
	}
}

func TestRunBuiltInDispatchesProfileCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameProfile,
		Input:      "Profile command habits.",
		Phase:      workflow.PhaseAudit,
		Metadata: map[string]string{
			ProfileMetadataSubject:           "command habit profile",
			ProfileMetadataDecisionSignals:   "Prefer exact command proof",
			ProfileMetadataUpdateSuggestions: "Keep CLI evidence close to closeout",
			ProfileMetadataEvidence:          "Recent runs used direct CLI smokes",
		},
	})
	if err != nil {
		t.Fatalf("RunBuiltIn returned error: %v", err)
	}
	if payload.Capability != NameProfile || payload.Profile == nil || payload.Signal != ExitComplete || payload.RecommendedSuccessor != "" {
		t.Fatalf("built-in profile payload = %+v", payload)
	}
}
