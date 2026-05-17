package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	AuditMetadataFindingID          = "audit_finding_id"
	AuditMetadataFindingSeverity    = "audit_finding_severity"
	AuditMetadataFindingTitle       = "audit_finding_title"
	AuditMetadataFindingMessage     = "audit_finding_message"
	AuditMetadataFindingSourceRefs  = "audit_finding_source_refs"
	AuditMetadataFindingNextActions = "audit_finding_next_actions"
	AuditMetadataSummary            = "audit_summary"
	AuditMetadataNextActions        = "audit_next_actions"
	AuditMetadataCaveats            = "audit_caveats"
	AuditMetadataRecommendedNext    = "audit_recommended_successor"
	AuditMetadataEvidenceState      = "audit_evidence_state"
)

// AuditCapability summarizes read-only review evidence and recommends valid follow-up.
type AuditCapability struct{}

// AuditOutput is the typed audit data carried by an audit capability exit.
type AuditOutput struct {
	Findings      []AuditFinding
	Summary       string
	NextActions   []string
	Caveats       []string
	EvidenceState string
	SourceRefs    []SourceRef
}

// AuditFinding records one deterministic finding from app-supplied evidence.
type AuditFinding struct {
	ID           string
	Severity     string
	Title        string
	Message      string
	SourceRefIDs []string
	NextActions  []string
}

// Name returns the fixed capability identity.
func (AuditCapability) Name() Name {
	return NameAudit
}

// OwningPhase returns AUDIT because the capability checks work quality.
func (AuditCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseAudit
}

// Run emits one audit payload from app-supplied evidence without reading files or mutating state.
func (AuditCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run audit capability: %w", err)
	}
	request = normalizeAuditRequest(request)
	invocation := NewInvocation(request)
	output := auditOutput(request)
	if len(output.Findings) == 0 && strings.TrimSpace(request.Metadata[AuditMetadataEvidenceState]) == "" {
		payload := ExitPayload{
			Capability:       NameAudit,
			Signal:           ExitWaiting,
			Summary:          "Audit needs app-supplied review evidence before it can check work.",
			NeededInput:      "Open review with diff, history, or diagnostic evidence available.",
			NextAction:       "Run review after producing or selecting evidence to audit.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: auditBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	signal := ExitComplete
	if auditHasBlockingFinding(output.Findings) {
		signal = ExitFlagged
	}
	successor := auditRecommendedSuccessor(request)
	payload := ExitPayload{
		Capability:           NameAudit,
		Signal:               signal,
		Summary:              auditSummary(output, signal),
		Concerns:             auditConcerns(output.Findings),
		Attempted:            true,
		NextAction:           auditNextAction(output, signal),
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{
			{ID: "review-surface", Kind: "app_display", Path: "review"},
		},
		SourceRefs:       cloneSourceRefs(output.SourceRefs),
		BoundaryRequests: auditBoundaryRequests(request),
		Audit:            &output,
	}
	return invocation.Emit(payload)
}

func normalizeAuditRequest(request Request) Request {
	request.Capability = NameAudit
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseAudit
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func auditOutput(request Request) AuditOutput {
	finding := AuditFinding{
		ID:           auditMetadata(request, AuditMetadataFindingID, "audit-finding-1"),
		Severity:     auditMetadata(request, AuditMetadataFindingSeverity, "info"),
		Title:        auditMetadata(request, AuditMetadataFindingTitle, ""),
		Message:      auditMetadata(request, AuditMetadataFindingMessage, ""),
		SourceRefIDs: auditListMetadata(request, AuditMetadataFindingSourceRefs),
		NextActions:  auditListMetadata(request, AuditMetadataFindingNextActions),
	}
	var findings []AuditFinding
	if strings.TrimSpace(finding.Title) != "" || strings.TrimSpace(finding.Message) != "" {
		if len(finding.SourceRefIDs) == 0 && len(request.SourceRefs) > 0 {
			finding.SourceRefIDs = []string{request.SourceRefs[0].ID}
		}
		if len(finding.NextActions) == 0 {
			finding.NextActions = []string{"Review the finding before continuing."}
		}
		findings = append(findings, finding)
	}
	nextActions := auditListMetadata(request, AuditMetadataNextActions)
	if len(nextActions) == 0 {
		nextActions = auditDefaultNextActions(findings)
	}
	caveats := auditListMetadata(request, AuditMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"audit used app-supplied review evidence only"}
	}
	return AuditOutput{
		Findings:      findings,
		Summary:       auditMetadata(request, AuditMetadataSummary, ""),
		NextActions:   nextActions,
		Caveats:       caveats,
		EvidenceState: auditMetadata(request, AuditMetadataEvidenceState, "available"),
		SourceRefs:    auditSourceRefs(request),
	}
}

func auditRecommendedSuccessor(request Request) workflow.Phase {
	preferred := workflow.Phase(auditMetadata(request, AuditMetadataRecommendedNext, ""))
	if preferred == "" {
		preferred = workflow.PhaseBuild
	}
	if workflow.ValidateProtocolSuccessor(request.Phase, preferred) == nil {
		return preferred
	}
	return ""
}

func auditHasBlockingFinding(findings []AuditFinding) bool {
	for _, finding := range findings {
		switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
		case "critical", "error", "high", "warning":
			return true
		}
	}
	return false
}

func auditSummary(output AuditOutput, signal ExitSignal) string {
	if strings.TrimSpace(output.Summary) != "" {
		return output.Summary
	}
	if signal == ExitFlagged {
		return fmt.Sprintf("Audit found %d finding(s) that need follow-up.", len(output.Findings))
	}
	if len(output.Findings) == 0 {
		return "Audit checked supplied evidence and found no blocking findings."
	}
	return fmt.Sprintf("Audit checked supplied evidence and recorded %d informational finding(s).", len(output.Findings))
}

func auditNextAction(output AuditOutput, signal ExitSignal) string {
	for _, action := range output.NextActions {
		return action
	}
	if signal == ExitFlagged {
		return "Fix or re-plan the audit findings before continuing."
	}
	return "Continue after reviewing the audit findings."
}

func auditConcerns(findings []AuditFinding) []string {
	concerns := make([]string, 0, len(findings))
	for _, finding := range findings {
		if strings.TrimSpace(finding.Message) == "" {
			continue
		}
		concerns = append(concerns, finding.Severity+": "+finding.Message)
	}
	return concerns
}

func auditDefaultNextActions(findings []AuditFinding) []string {
	if auditHasBlockingFinding(findings) {
		return []string{"Route back to build to fix audit findings.", "Re-plan if findings change scope."}
	}
	return []string{"Continue with the next planned build or audit step."}
}

func auditBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("review.current", "audit uses app-owned review evidence"),
		request.RequestContextAccess("current_context", "audit uses supplied source refs and context evidence"),
		request.RequestArtifactAccess("history", "audit may reference recent app-owned history"),
	}
}

func auditSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "audit-evidence", Kind: "review_state", Excerpt: auditMetadata(request, AuditMetadataEvidenceState, "available")})
	}
	return refs
}

func auditMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func auditListMetadata(request Request, key string) []string {
	if request.Metadata == nil {
		return nil
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
