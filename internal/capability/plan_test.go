package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestPlanCapabilityEmitsScopedPayloadWithArtifactAndValidSuccessor(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:    "plan-1",
		Input: "Milestone 47 plan capability",
		Phase: workflow.PhaseBuild,
		Metadata: map[string]string{
			PlanMetadataTitle:        "Plan Capability Slice",
			PlanMetadataProjectState: "project store initialized",
			PlanMetadataSessionState: "runtime idle with roadmap context",
			PlanMetadataNextAction:   "Start with the first pending implementation item.",
		},
		SourceRefs: []SourceRef{{ID: "roadmap-m47", Kind: "roadmap", Path: "ROADMAP.md", LineStart: 1842, LineEnd: 1860, Excerpt: "Milestone 47: Plan Capability"}},
	}

	payload, err := PlanCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Capability != NamePlan || payload.Signal != ExitComplete || !payload.Attempted {
		t.Fatalf("plan identity = %+v, want attempted plan complete", payload)
	}
	if payload.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("RecommendedSuccessor = %q, want plan", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 1 || payload.ArtifactRefs[0].Path != defaultPlanArtifactPath {
		t.Fatalf("ArtifactRefs = %+v", payload.ArtifactRefs)
	}
	if payload.Plan == nil {
		t.Fatal("Plan output is nil")
	}
	if payload.Plan.Title != "Plan Capability Slice" || payload.Plan.Scope != "Milestone 47 plan capability" {
		t.Fatalf("Plan title/scope = %+v", payload.Plan)
	}
	if len(payload.Plan.Items) != 3 || !payload.Plan.Items[0].Done || payload.Plan.Items[1].Done {
		t.Fatalf("Plan items = %+v", payload.Plan.Items)
	}
	if !strings.Contains(payload.Plan.Document, "GIVEN implementation starts WHEN code changes are made") {
		t.Fatalf("plan document missing behavioral acceptance criteria:\n%s", payload.Plan.Document)
	}
	if payload.NextAction != "Start with the first pending implementation item." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
	if len(payload.SourceRefs) < 3 || payload.SourceRefs[0].ID != "roadmap-m47" {
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
		{BoundaryArtifactAccess, "plan"},
		{BoundaryStateWrite, "plan"},
	} {
		if !hasBoundaryRequest(payload.BoundaryRequests, want.kind, want.target) {
			t.Fatalf("BoundaryRequests = %+v, missing %s %s", payload.BoundaryRequests, want.kind, want.target)
		}
	}
}

func TestPlanCapabilityFlagsMissingEvidenceWithoutBuildExecution(t *testing.T) {
	t.Parallel()

	payload, err := PlanCapability{}.Run(context.Background(), Request{ID: "plan-missing", Input: "prepare the next slice", Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.RecommendedSuccessor != "" {
		t.Fatalf("flagged payload = %+v, want no successor", payload)
	}
	for _, blocker := range []string{"project state evidence missing", "session state evidence missing"} {
		if !containsString(payload.Concerns, blocker) || payload.Plan == nil || !containsString(payload.Plan.Blockers, blocker) {
			t.Fatalf("missing blocker %q in payload=%+v plan=%+v", blocker, payload, payload.Plan)
		}
	}
	if payload.NextAction != "Provide missing project/session evidence before executing the plan." {
		t.Fatalf("NextAction = %q", payload.NextAction)
	}
	if hasBoundaryKind(payload.BoundaryRequests, BoundaryToolExecution) || strings.Contains(payload.Summary, "build executed") {
		t.Fatalf("plan invented execution work: summary=%q boundaries=%+v", payload.Summary, payload.BoundaryRequests)
	}
}

func TestPlanCapabilityWaitsForScope(t *testing.T) {
	t.Parallel()

	payload, err := PlanCapability{}.Run(context.Background(), Request{ID: "plan-empty", Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if payload.Plan != nil {
		t.Fatalf("waiting plan output = %+v, want none", payload.Plan)
	}
}

func TestBuiltInRunnerRunsPlanCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NamePlan,
		Input:      "create a scoped plan",
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			PlanMetadataProjectState: "project ready",
			PlanMetadataSessionState: "session ready",
		},
	})
	if err != nil {
		t.Fatalf("RunBuiltIn returned error: %v", err)
	}
	if payload.Capability != NamePlan || payload.Plan == nil || payload.Signal != ExitComplete {
		t.Fatalf("built-in plan payload = %+v", payload)
	}
}

func hasBoundaryKind(requests []BoundaryRequest, kind BoundaryKind) bool {
	for _, request := range requests {
		if request.Kind == kind {
			return true
		}
	}
	return false
}
