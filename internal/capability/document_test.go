package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestDocumentCapabilityEmitsDocPlanDiffAndMutationPayload(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "document-ready",
		Capability: NameDocument,
		Input:      "Document the /document command mutation path.",
		Phase:      workflow.PhaseBuild,
		SourceRefs: []SourceRef{{ID: "workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", Excerpt: "document is BUILD-owned"}},
		Metadata: map[string]string{
			DocumentMetadataTargetPath:       "docs/aila-documentation-output.md",
			DocumentMetadataTargetTitle:      "Aila documentation alignment",
			DocumentMetadataSourceBehavior:   "/document routes documentation writes through mutation safety",
			DocumentMetadataPlanID:           "document-safety",
			DocumentMetadataPlanSummary:      "Record the document command safety path.",
			DocumentMetadataPlanSteps:        "write doc output through mutation tool|persist documentation artifact through state store",
			DocumentMetadataOutputSummary:    "Documented the /document mutation safety path.",
			DocumentMetadataDiffLines:        "+ # Aila Documentation Alignment|+ Capability: document",
			DocumentMetadataCaveats:          "deterministic app-supplied documentation evidence only",
			DocumentMetadataToolStatus:       "completed",
			DocumentMetadataDecisionSource:   "autonomy_policy",
			DocumentMetadataDecisionAutonomy: "write",
			DocumentMetadataDecisionAllowed:  "true",
			DocumentMetadataBytesWritten:     "128",
			DocumentMetadataDurable:          "true",
		},
	}

	payload, err := DocumentCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run document capability: %v", err)
	}
	if payload.Capability != NameDocument || payload.Signal != ExitComplete || !payload.Attempted || payload.Document == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Document.Target.Path != "docs/aila-documentation-output.md" || payload.Document.Plan.ID != "document-safety" || len(payload.Document.Plan.Steps) != 2 || len(payload.Document.DiffLines) != 2 || payload.Document.Mutation.Status != "completed" || !payload.Document.Mutation.DecisionAllowed {
		t.Fatalf("document output = %+v", payload.Document)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit {
		t.Fatalf("recommended successor = %q, want audit", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 2 || len(payload.SourceRefs) == 0 || len(payload.BoundaryRequests) == 0 || payload.Document.DocumentArtifact == "" {
		t.Fatalf("missing refs, boundaries, or artifact: refs=%+v artifacts=%+v boundaries=%+v document=%+v", payload.SourceRefs, payload.ArtifactRefs, payload.BoundaryRequests, payload.Document)
	}
}

func TestDocumentCapabilityWaitsForTargetBehaviorAndPlanWithoutInventingDocs(t *testing.T) {
	t.Parallel()

	payload, err := DocumentCapability{}.Run(context.Background(), Request{ID: "document-wait", Capability: NameDocument, Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("Run document capability: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.Attempted || payload.Document != nil || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if !strings.Contains(payload.Summary, "source behavior") {
		t.Fatalf("waiting summary = %q", payload.Summary)
	}
}

func TestDocumentCapabilityFlagsDeniedMutationAndHoldsInBuild(t *testing.T) {
	t.Parallel()

	payload, err := DocumentCapability{}.Run(context.Background(), Request{
		ID:         "document-denied",
		Capability: NameDocument,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			DocumentMetadataTargetPath:       "docs/aila-documentation-output.md",
			DocumentMetadataSourceBehavior:   "document command write safety",
			DocumentMetadataPlanSummary:      "Record the doc write safety path.",
			DocumentMetadataToolStatus:       "denied",
			DocumentMetadataDecisionAllowed:  "false",
			DocumentMetadataApprovalRequired: "true",
			DocumentMetadataErrorMessage:     "read autonomy blocked documentation write",
		},
	})
	if err != nil {
		t.Fatalf("Run document capability: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.Document == nil || payload.RecommendedSuccessor != workflow.PhaseBuild {
		t.Fatalf("flagged payload = %+v", payload)
	}
	if payload.Document.Mutation.DecisionAllowed || len(payload.Document.Caveats) == 0 || !strings.Contains(payload.NextAction, "Review") {
		t.Fatalf("flagged document output = %+v next=%q", payload.Document, payload.NextAction)
	}
}

func TestRunBuiltInDispatchesDocumentCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{Capability: NameDocument, Phase: workflow.PhaseBuild, Metadata: map[string]string{
		DocumentMetadataTargetPath:     "docs/aila-documentation-output.md",
		DocumentMetadataSourceBehavior: "document command write safety",
		DocumentMetadataPlanSummary:    "Record the doc write safety path.",
	}})
	if err != nil {
		t.Fatalf("RunBuiltIn document: %v", err)
	}
	if payload.Capability != NameDocument || payload.Document == nil {
		t.Fatalf("RunBuiltIn payload = %+v", payload)
	}
}
