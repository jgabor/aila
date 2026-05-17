package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestAuditCapabilityEmitsFindingsWithSeveritySourcesAndValidSuccessor(t *testing.T) {
	t.Parallel()

	payload, err := AuditCapability{}.Run(context.Background(), Request{
		ID:         "audit-ready",
		Capability: NameAudit,
		Phase:      workflow.PhaseAudit,
		SourceRefs: []SourceRef{{ID: "diff-ref", Kind: "diff", Path: "internal/app/inspection.go", Excerpt: "changed file: internal/app/inspection.go"}},
		Metadata: map[string]string{
			AuditMetadataFindingID:          "review-current-change",
			AuditMetadataFindingSeverity:    "warning",
			AuditMetadataFindingTitle:       "Review current changes before continuing",
			AuditMetadataFindingMessage:     "Current diff evidence needs review before another build step.",
			AuditMetadataFindingSourceRefs:  "diff-ref",
			AuditMetadataFindingNextActions: "Route back to build after reviewing the changed file.|Re-plan if the finding changes scope.",
			AuditMetadataNextActions:        "Route back to build after reviewing audit findings.|Re-plan if findings change scope.",
			AuditMetadataRecommendedNext:    "build",
			AuditMetadataEvidenceState:      "diff_available",
		},
	})
	if err != nil {
		t.Fatalf("Run audit capability: %v", err)
	}
	if payload.Capability != NameAudit || payload.Signal != ExitFlagged || !payload.Attempted || payload.Audit == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.RecommendedSuccessor != workflow.PhaseBuild {
		t.Fatalf("recommended successor = %q, want build", payload.RecommendedSuccessor)
	}
	if got := payload.Audit.Findings; len(got) != 1 || got[0].Severity != "warning" || got[0].SourceRefIDs[0] != "diff-ref" || len(got[0].NextActions) != 2 {
		t.Fatalf("findings = %+v", got)
	}
	if len(payload.BoundaryRequests) == 0 || len(payload.SourceRefs) == 0 || len(payload.Concerns) == 0 {
		t.Fatalf("missing audit evidence: refs=%+v concerns=%+v boundaries=%+v", payload.SourceRefs, payload.Concerns, payload.BoundaryRequests)
	}
}

func TestAuditCapabilityWaitsForEvidenceWithoutInventingReview(t *testing.T) {
	t.Parallel()

	payload, err := AuditCapability{}.Run(context.Background(), Request{ID: "audit-wait", Capability: NameAudit, Phase: workflow.PhaseAudit})
	if err != nil {
		t.Fatalf("Run audit capability: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.Attempted || payload.Audit != nil || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if !strings.Contains(payload.Summary, "review evidence") {
		t.Fatalf("waiting summary = %q", payload.Summary)
	}
}

func TestAuditCapabilityOmitsInvalidSuccessorRecommendation(t *testing.T) {
	t.Parallel()

	payload, err := AuditCapability{}.Run(context.Background(), Request{
		ID:         "audit-invalid-successor",
		Capability: NameAudit,
		Phase:      workflow.PhaseAudit,
		Metadata: map[string]string{
			AuditMetadataFindingSeverity: "info",
			AuditMetadataFindingTitle:    "No blocking findings",
			AuditMetadataFindingMessage:  "Supplied evidence was checked.",
			AuditMetadataRecommendedNext: "audit",
			AuditMetadataEvidenceState:   "history_available",
		},
	})
	if err != nil {
		t.Fatalf("Run audit capability: %v", err)
	}
	if payload.Signal != ExitComplete || payload.RecommendedSuccessor != "" {
		t.Fatalf("payload = %+v, want complete without invalid successor", payload)
	}
}

func TestRunBuiltInDispatchesAuditCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{Capability: NameAudit, Phase: workflow.PhaseAudit, Metadata: map[string]string{
		AuditMetadataFindingTitle:   "Review supplied evidence",
		AuditMetadataFindingMessage: "Diff evidence was checked.",
		AuditMetadataEvidenceState:  "diff_available",
	}})
	if err != nil {
		t.Fatalf("RunBuiltIn audit: %v", err)
	}
	if payload.Capability != NameAudit || payload.Audit == nil {
		t.Fatalf("RunBuiltIn payload = %+v", payload)
	}
}
