package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestBuildCapabilityEmitsOneBoundedStepPayloadWithSuccessorEvidence(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "build-ready",
		Capability: NameBuild,
		Input:      "Implement only the selected plan item",
		Phase:      workflow.PhaseBuild,
		SourceRefs: []SourceRef{{ID: "plan-ref", Kind: "plan_item", Excerpt: "Implement only the selected plan item"}},
		Metadata: map[string]string{
			BuildMetadataPlanItemID:       "implement",
			BuildMetadataPlanItemText:     "Implement only the selected plan item",
			BuildMetadataStepID:           "write-output",
			BuildMetadataStepText:         "Write bounded build output",
			BuildMetadataToolName:         "write",
			BuildMetadataToolStatus:       "completed",
			BuildMetadataTargetPath:       "docs/aila-build-output.md",
			BuildMetadataExpectedEffect:   "create bounded build output",
			BuildMetadataDecisionSource:   "autonomy_policy",
			BuildMetadataDecisionAutonomy: "write",
			BuildMetadataDecisionAllowed:  "true",
			BuildMetadataApprovalRequired: "false",
			BuildMetadataBytesWritten:     "42",
		},
	}

	payload, err := BuildCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run build capability: %v", err)
	}
	if payload.Capability != NameBuild || payload.Signal != ExitComplete || !payload.Attempted || payload.Build == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Build.PlanItem.ID != "implement" || payload.Build.Tool.Status != "completed" || payload.Build.Tool.Path != "docs/aila-build-output.md" || payload.Build.Tool.DecisionSource != "autonomy_policy" || !payload.Build.Tool.DecisionAllowed || payload.Build.Tool.ApprovalRequired {
		t.Fatalf("build output = %+v", payload.Build)
	}
	if len(payload.Build.ChangedPaths) != 1 || payload.Build.ChangedPaths[0] != "docs/aila-build-output.md" {
		t.Fatalf("changed paths = %+v", payload.Build.ChangedPaths)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit {
		t.Fatalf("recommended successor = %q, want audit", payload.RecommendedSuccessor)
	}
	if len(payload.BoundaryRequests) == 0 || len(payload.SourceRefs) == 0 || len(payload.ArtifactRefs) == 0 {
		t.Fatalf("missing refs or boundaries: refs=%+v artifacts=%+v boundaries=%+v", payload.SourceRefs, payload.ArtifactRefs, payload.BoundaryRequests)
	}
}

func TestBuildCapabilityWaitsForPlanItemWithoutInventingWork(t *testing.T) {
	t.Parallel()

	payload, err := BuildCapability{}.Run(context.Background(), Request{ID: "build-wait", Capability: NameBuild, Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("Run build capability: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.Attempted || payload.Build != nil || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if !strings.Contains(payload.Summary, "active plan item") {
		t.Fatalf("waiting summary = %q", payload.Summary)
	}
}

func TestBuildCapabilityFlagsDeniedToolResultAndHoldsInBuild(t *testing.T) {
	t.Parallel()

	payload, err := BuildCapability{}.Run(context.Background(), Request{
		ID:         "build-denied",
		Capability: NameBuild,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			BuildMetadataPlanItemID:       "implement",
			BuildMetadataPlanItemText:     "Implement only the selected plan item",
			BuildMetadataToolName:         "write",
			BuildMetadataToolStatus:       "denied",
			BuildMetadataTargetPath:       "docs/aila-build-output.md",
			BuildMetadataExpectedEffect:   "create bounded build output",
			BuildMetadataDecisionSource:   "autonomy_policy",
			BuildMetadataDecisionAutonomy: "read",
			BuildMetadataDecisionAllowed:  "false",
			BuildMetadataApprovalRequired: "true",
			BuildMetadataErrorKind:        "permission",
			BuildMetadataErrorMessage:     "read autonomy requires approval for write-shaped operation",
		},
	})
	if err != nil {
		t.Fatalf("Run build capability: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.Build == nil || payload.RecommendedSuccessor != workflow.PhaseBuild {
		t.Fatalf("flagged payload = %+v", payload)
	}
	if len(payload.Build.ChangedPaths) != 0 || len(payload.Build.Blockers) == 0 || !strings.Contains(payload.NextAction, "blockers") {
		t.Fatalf("denied build output = %+v next=%q", payload.Build, payload.NextAction)
	}
}

func TestRunBuiltInDispatchesBuildCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{Capability: NameBuild, Phase: workflow.PhaseBuild, Metadata: map[string]string{
		BuildMetadataPlanItemText: "Run one bounded build step",
		BuildMetadataToolStatus:   "completed",
		BuildMetadataTargetPath:   "docs/aila-build-output.md",
	}})
	if err != nil {
		t.Fatalf("RunBuiltIn build: %v", err)
	}
	if payload.Capability != NameBuild || payload.Build == nil {
		t.Fatalf("RunBuiltIn payload = %+v", payload)
	}
}
