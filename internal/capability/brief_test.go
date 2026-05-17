package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestBriefCapabilityEmitsOrientationPayloadWithoutPhaseTransition(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "brief-1",
		Phase: workflow.PhaseBuild,
		Metadata: map[string]string{
			BriefMetadataRuntimeStatus:       "idle",
			BriefMetadataProjectStoreStatus:  "initialized",
			BriefMetadataHistoryState:        "loaded",
			BriefMetadataHistoryEvents:       "3",
			BriefMetadataContextStatus:       "current",
			BriefMetadataContextSummary:      "roadmap and runtime evidence ready",
			BriefMetadataHealthStatus:        "available",
			BriefMetadataSuggestedNextAction: "Continue the accepted build task.",
		},
		SourceRefs: []SourceRef{{ID: "runtime-status", Kind: "runtime_state", Excerpt: "idle"}},
	}

	payload, err := BriefCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NameBrief || payload.Signal != ExitComplete {
		t.Fatalf("brief identity = %+v, want brief complete", payload)
	}
	if payload.RecommendedSuccessor != "" {
		t.Fatalf("RecommendedSuccessor = %q, want no phase transition recommendation", payload.RecommendedSuccessor)
	}
	if payload.NextAction != "Continue the accepted build task." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
	if len(payload.Concerns) != 0 {
		t.Fatalf("Concerns = %#v, want none", payload.Concerns)
	}
	if !strings.Contains(payload.Summary, "phase build") || !strings.Contains(payload.Summary, "history loaded (3 events)") {
		t.Fatalf("Summary = %q", payload.Summary)
	}
}

func TestBriefCapabilityReportsKnownGapsWhenEvidenceIsMissing(t *testing.T) {
	t.Parallel()

	payload, err := BriefCapability{}.Run(context.Background(), Request{ID: "brief-missing"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := []string{
		"runtime status unavailable",
		"project store status unavailable",
		"history unavailable",
		"context unavailable",
		"health unavailable",
	}
	for _, gap := range want {
		if !containsString(payload.Concerns, gap) {
			t.Fatalf("Concerns = %#v, missing %q", payload.Concerns, gap)
		}
	}
	if payload.NextAction != "Review the missing evidence, then choose the next capability." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
}

func TestBriefCapabilityUsesBoundaryDescriptorsForStateHistoryContextAndHealth(t *testing.T) {
	t.Parallel()

	payload, err := BriefCapability{}.Run(context.Background(), Request{ID: "brief-boundaries"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := map[BoundaryKind]string{
		BoundaryStateAccess:    "runtime.current",
		BoundaryArtifactAccess: "current_session_snapshot",
		BoundaryContextAccess:  "current_context",
	}
	for kind, target := range want {
		if !hasBoundaryRequest(payload.BoundaryRequests, kind, target) {
			t.Fatalf("BoundaryRequests = %+v, missing %s %s", payload.BoundaryRequests, kind, target)
		}
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryArtifactAccess, "fake_history") {
		t.Fatalf("BoundaryRequests = %+v, missing fake history artifact boundary", payload.BoundaryRequests)
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryArtifactAccess, "health") {
		t.Fatalf("BoundaryRequests = %+v, missing health artifact boundary", payload.BoundaryRequests)
	}
}

func TestBuiltInRunnerRejectsUnsupportedCapabilities(t *testing.T) {
	t.Parallel()

	if _, err := RunBuiltIn(context.Background(), Request{Capability: NameAudit}); err == nil {
		t.Fatal("RunBuiltIn accepted unsupported audit capability")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasBoundaryRequest(requests []BoundaryRequest, kind BoundaryKind, target string) bool {
	for _, request := range requests {
		if request.Kind == kind && request.Target == target {
			return true
		}
	}
	return false
}
