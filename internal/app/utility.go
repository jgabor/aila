package app

import (
	"strings"

	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/utility"
)

func defaultUtilityJobRequest(model string) utility.JobRequest {
	return utility.NormalizeJobRequest(utility.JobRequest{
		ID:    "status-stale-context-check",
		Kind:  utility.JobStaleContextCheck,
		Model: model,
		Source: utility.Source{
			Caller:      "app.status",
			RequestID:   "status-stale-context-check",
			Description: "idle-only utility stale-context check",
		},
		StaleContext: utility.StaleContextInput{
			SavedFingerprint:   "saved-context:utility-context-prep",
			CurrentFingerprint: "current-context:status-runtime",
			SavedLabel:         "saved context",
			CurrentLabel:       "current runtime status",
		},
	})
}

func (runner *inputRunner) proposeUtilityJob(request utility.JobRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.UtilityJobProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func utilityView(model runtime.Model) *tui.UtilityView {
	result := model.LastUtility
	if model.ActiveUtility.ID != "" {
		result = utility.RunningResult(model.ActiveUtility)
	}
	if result.Request.ID == "" && result.Status == "" && result.Summary == "" {
		return nil
	}
	request := utility.NormalizeJobRequest(result.Request)
	status := string(result.Status)
	if status == "" {
		status = string(utility.StatusIdle)
	}
	return &tui.UtilityView{
		Source:          defaultString(request.Source.Caller, "app.utility"),
		Status:          status,
		JobID:           request.ID,
		JobKind:         string(request.Kind),
		Model:           request.Model,
		Summary:         strings.TrimSpace(result.Summary),
		PreparedContext: utilityPreparedContextView(result.PreparedContext),
		StaleContext:    utilityStaleContextView(result.StaleContext),
		Suggestions:     utilitySuggestionViews(result.Suggestions),
		EvidenceRefs:    utilityEvidenceRefViews(result.EvidenceRefs),
		Caveats:         append([]string(nil), result.Caveats...),
		DeniedReason:    string(result.Denial.Reason),
		DeniedDetail:    result.Denial.Detail,
		ReadOnly:        true,
		Safety: tui.UtilitySafetyView{
			FileMutation:            result.Safety.FileMutation,
			GitMutation:             result.Safety.GitMutation,
			ProjectArtifactMutation: result.Safety.ProjectArtifactMutation,
			ApprovalGrant:           result.Safety.PermissionApproval,
			WorkflowPhaseTransition: result.Safety.WorkflowPhaseTransition,
			FinalJudgment:           result.Safety.FinalJudgment,
			ContextRefresh:          result.Safety.ContextRefresh,
			ContextCompaction:       result.Safety.ContextCompaction,
			ContextRewrite:          result.Safety.ContextRewrite,
		},
	}
}

func utilityPreparedContextView(prepared utility.PreparedContext) tui.UtilityPreparedContextView {
	return tui.UtilityPreparedContextView{
		Summary:          strings.TrimSpace(prepared.Summary),
		EvidenceRefIDs:   append([]string(nil), prepared.EvidenceRefIDs...),
		Caveats:          append([]string(nil), prepared.Caveats...),
		NonAuthoritative: prepared.NonAuthoritative,
	}
}

func utilityStaleContextView(stale utility.StaleContextCheck) tui.UtilityStaleContextView {
	return tui.UtilityStaleContextView{
		Status:              string(stale.Status),
		Summary:             strings.TrimSpace(stale.Summary),
		EvidenceRefIDs:      append([]string(nil), stale.EvidenceRefIDs...),
		Caveats:             append([]string(nil), stale.Caveats...),
		SuggestedNextAction: strings.TrimSpace(stale.SuggestedNextAction),
	}
}

func utilitySuggestionViews(suggestions []utility.Suggestion) []tui.UtilitySuggestionView {
	if len(suggestions) == 0 {
		return nil
	}
	views := make([]tui.UtilitySuggestionView, 0, len(suggestions))
	for _, suggestion := range suggestions {
		views = append(views, tui.UtilitySuggestionView{Text: suggestion.Text, EvidenceRefIDs: append([]string(nil), suggestion.EvidenceRefIDs...)})
	}
	return views
}

func utilityEvidenceRefViews(refs []utility.EvidenceRef) []tui.UtilityEvidenceRefView {
	if len(refs) == 0 {
		return nil
	}
	views := make([]tui.UtilityEvidenceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.UtilityEvidenceRefView{ID: ref.ID, Kind: ref.Kind, Source: ref.Source, Detail: ref.Detail})
	}
	return views
}
