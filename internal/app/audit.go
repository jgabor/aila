package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

func auditRequestFromReview(view tui.ViewState, diff *tui.DiffView, historyState string, events []history.FakeEvent) capability.Request {
	refs := auditSourceRefsFromReview(view, diff, historyState, events)
	metadata := map[string]string{
		capability.AuditMetadataEvidenceState: auditEvidenceState(view, diff, historyState, events),
		capability.AuditMetadataSummary:       auditSummaryFromReview(view, diff, events),
		capability.AuditMetadataCaveats:       "audit used app-supplied review evidence only",
	}
	finding := auditFindingFromReview(view, diff, events)
	if finding.title != "" || finding.message != "" {
		metadata[capability.AuditMetadataFindingID] = finding.id
		metadata[capability.AuditMetadataFindingSeverity] = finding.severity
		metadata[capability.AuditMetadataFindingTitle] = finding.title
		metadata[capability.AuditMetadataFindingMessage] = finding.message
		metadata[capability.AuditMetadataFindingSourceRefs] = strings.Join(finding.sourceRefIDs, "|")
		metadata[capability.AuditMetadataFindingNextActions] = strings.Join(finding.nextActions, "|")
		metadata[capability.AuditMetadataNextActions] = strings.Join(auditNextActionsFromFinding(finding), "|")
		metadata[capability.AuditMetadataRecommendedNext] = workflow.PhaseBuild.String()
	} else {
		metadata[capability.AuditMetadataNextActions] = "Continue with the next planned step after reviewing the empty audit."
		metadata[capability.AuditMetadataRecommendedNext] = workflow.PhaseBuild.String()
	}
	return capability.Request{
		ID:         "review-audit",
		Capability: capability.NameAudit,
		Input:      "review current work",
		Phase:      workflow.PhaseAudit,
		SourceRefs: refs,
		Metadata:   metadata,
	}
}

type reviewAuditFinding struct {
	id           string
	severity     string
	title        string
	message      string
	sourceRefIDs []string
	nextActions  []string
}

func auditFindingFromReview(view tui.ViewState, diff *tui.DiffView, events []history.FakeEvent) reviewAuditFinding {
	if len(view.Diagnostics) > 0 {
		return reviewAuditFinding{
			id:           "visible-diagnostics",
			severity:     "warning",
			title:        "Visible diagnostics need review",
			message:      fmt.Sprintf("%d visible diagnostic(s) should be resolved or acknowledged before continuing.", len(view.Diagnostics)),
			sourceRefIDs: []string{"review-diagnostics"},
			nextActions:  []string{"Route back to build after resolving diagnostics.", "Re-plan if diagnostics change scope."},
		}
	}
	if diff != nil && len(diff.Files) > 0 {
		return reviewAuditFinding{
			id:           "current-change-review",
			severity:     "warning",
			title:        "Review current changes before continuing",
			message:      fmt.Sprintf("%d changed file(s) need review before another build step.", len(diff.Files)),
			sourceRefIDs: []string{"review-diff"},
			nextActions:  []string{"Route back to build after reviewing changed files.", "Re-plan if findings change scope."},
		}
	}
	if len(events) > 0 {
		return reviewAuditFinding{
			id:           "recent-history-review",
			severity:     "info",
			title:        "Review recent history before continuing",
			message:      fmt.Sprintf("%d history event(s) are available as audit context.", len(events)),
			sourceRefIDs: []string{"review-history"},
			nextActions:  []string{"Continue after checking recent history."},
		}
	}
	return reviewAuditFinding{}
}

func auditNextActionsFromFinding(finding reviewAuditFinding) []string {
	if len(finding.nextActions) > 0 {
		return finding.nextActions
	}
	if finding.severity == "warning" || finding.severity == "error" || finding.severity == "critical" {
		return []string{"Route back to build after reviewing audit findings.", "Re-plan if findings change scope."}
	}
	return []string{"Continue after reviewing audit findings."}
}

func auditEvidenceState(view tui.ViewState, diff *tui.DiffView, historyState string, events []history.FakeEvent) string {
	switch {
	case len(view.Diagnostics) > 0:
		return "diagnostics_available"
	case diff != nil && len(diff.Files) > 0:
		return "diff_available"
	case len(events) > 0:
		return "history_available"
	case diff != nil && diff.Status != "":
		return "review_checked"
	case strings.TrimSpace(historyState) != "":
		return "history_checked"
	default:
		return "empty"
	}
}

func auditSummaryFromReview(view tui.ViewState, diff *tui.DiffView, events []history.FakeEvent) string {
	if len(view.Diagnostics) > 0 {
		return fmt.Sprintf("Audit found %d visible diagnostic(s) needing follow-up.", len(view.Diagnostics))
	}
	if diff != nil && len(diff.Files) > 0 {
		return fmt.Sprintf("Audit found %d changed file(s) needing review.", len(diff.Files))
	}
	if len(events) > 0 {
		return fmt.Sprintf("Audit checked %d recent history event(s) with no blocking findings.", len(events))
	}
	return "Audit checked review evidence and found no blocking findings."
}

func auditSourceRefsFromReview(view tui.ViewState, diff *tui.DiffView, historyState string, events []history.FakeEvent) []capability.SourceRef {
	refs := []capability.SourceRef{{ID: "review-command", Kind: "command", Command: "/review", Excerpt: "app-owned read-only review command"}}
	if len(view.Diagnostics) > 0 {
		refs = append(refs, capability.SourceRef{ID: "review-diagnostics", Kind: "diagnostics", Excerpt: fmt.Sprintf("visible diagnostics=%d", len(view.Diagnostics))})
	}
	if diff != nil {
		path := ""
		if len(diff.Files) > 0 {
			path = diff.Files[0].Path
		}
		refs = append(refs, capability.SourceRef{ID: "review-diff", Kind: "diff", Path: path, Excerpt: fmt.Sprintf("status=%s changed_files=%d", valueOr(diff.Status, "unknown"), len(diff.Files))})
	}
	latest := "none"
	if event, ok := latestHistoryEvent(events); ok {
		latest = latestHistoryExcerpt(event)
	}
	refs = append(refs, capability.SourceRef{ID: "review-history", Kind: "history", Excerpt: fmt.Sprintf("state=%s events=%d latest=%s", valueOr(historyState, "unknown"), len(events), latest)})
	return refs
}

func auditView(payload capability.ExitPayload, current workflow.Phase) *tui.AuditView {
	if payload.Capability != capability.NameAudit || payload.Audit == nil {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	audit := payload.Audit
	return &tui.AuditView{
		Source:               "app.audit",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		Summary:              payload.Summary,
		EvidenceState:        audit.EvidenceState,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		SuccessorRejected:    recommendation.SuccessorRejected,
		SuccessorReason:      recommendation.SuccessorReason,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Findings:             auditFindingViews(audit.Findings),
		NextActions:          append([]string(nil), audit.NextActions...),
		Caveats:              append([]string(nil), audit.Caveats...),
		ArtifactRefs:         auditArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           auditSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     auditBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func auditFindingViews(findings []capability.AuditFinding) []tui.AuditFindingView {
	views := make([]tui.AuditFindingView, 0, len(findings))
	for _, finding := range findings {
		views = append(views, tui.AuditFindingView{
			ID:           finding.ID,
			Severity:     finding.Severity,
			Title:        finding.Title,
			Message:      finding.Message,
			SourceRefIDs: append([]string(nil), finding.SourceRefIDs...),
			NextActions:  append([]string(nil), finding.NextActions...),
		})
	}
	return views
}

func auditArtifactRefViews(refs []capability.ArtifactRef) []tui.AuditArtifactRefView {
	views := make([]tui.AuditArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.AuditArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func auditSourceRefViews(refs []capability.SourceRef) []tui.AuditSourceRefView {
	views := make([]tui.AuditSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.AuditSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func auditBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.AuditBoundaryRequestView {
	views := make([]tui.AuditBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.AuditBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
