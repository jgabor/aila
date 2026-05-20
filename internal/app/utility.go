package app

import (
	"strings"

	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/utility"
)

func defaultUtilityJobRequest(model string) utility.JobRequest {
	return utility.NormalizeJobRequest(utility.JobRequest{
		ID:    "status-summary-refresh",
		Kind:  utility.JobSummaryRefresh,
		Model: model,
		Source: utility.Source{
			Caller:      "app.status",
			RequestID:   "status-summary-refresh",
			Description: "idle-only utility summary refresh",
		},
		SummaryRefresh: utility.SummaryRefreshInput{
			OriginalSummary: "Status output is available for the current runtime.",
			RequiredDetails: []string{
				"primary runtime remains idle",
				"utility worker can refresh summaries without final judgment",
			},
			SourceRefIDs:   []string{"summary-refresh-runtime", "summary-refresh-roadmap"},
			ConfidenceHint: "low",
		},
	})
}

func defaultIdleUtilitySchedule(model string) utility.IdleScheduleInput {
	return utility.IdleScheduleInput{
		IDPrefix:    "idle-utility",
		Model:       model,
		Caller:      "app.idle",
		RequestID:   "idle-utility",
		Description: "primary runtime idle utility schedule",
		StaleContext: utility.StaleContextInput{
			SavedLabel:   ".aila saved context",
			CurrentLabel: "workspace state",
		},
		SummaryRefresh: utility.SummaryRefreshInput{
			OriginalSummary: "Primary runtime is idle and ready for the next user action.",
			RequiredDetails: []string{
				"context prep is non-authoritative",
				"stale context checks do not rewrite saved context",
				"next-action suggestions require foreground confirmation",
			},
			SourceRefIDs:   []string{"idle-runtime", "idle-utility-boundary", "idle-next-action"},
			ConfidenceHint: "medium",
		},
	}
}

func (runner *inputRunner) scheduleIdleUtilityWork(model string) (tui.TranscriptTurn, bool) {
	requests, decision := utility.ScheduleIdleWork(runner.utilityActivity(), defaultIdleUtilitySchedule(model))
	if !decision.Allowed || len(requests) == 0 {
		return tui.TranscriptTurn{}, false
	}
	before := len(runner.model.Transcript)
	results := make([]utility.JobResult, 0, len(requests))
	for _, request := range requests {
		runner.apply(runtime.UtilityJobProposed{Request: request})
		results = append(results, runner.model.LastUtility)
	}
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	if scheduled := scheduledUtilityView(results); scheduled != nil {
		turn.Utility = scheduled
		turn.StatusDetail = "utility worker status"
	}
	return turn, true
}

func (runner *inputRunner) utilityActivity() utility.Activity {
	return utility.Activity{
		PrimaryStatus:       string(runner.model.Status),
		ActiveOperationKind: string(runner.model.ActiveOperation.Kind),
		ApprovalPending:     runner.model.PendingApproval.ID != "",
		QueuedCount:         len(runner.model.Queued),
	}
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
		SummaryRefresh:  utilitySummaryRefreshView(result.SummaryRefresh),
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

func utilitySummaryRefreshView(refresh utility.SummaryRefresh) tui.UtilitySummaryRefreshView {
	return tui.UtilitySummaryRefreshView{
		Status:           string(refresh.Status),
		OriginalSummary:  strings.TrimSpace(refresh.OriginalSummary),
		RefreshedSummary: strings.TrimSpace(refresh.RefreshedSummary),
		SourceRefIDs:     append([]string(nil), refresh.SourceRefIDs...),
		ExactDetails:     append([]string(nil), refresh.ExactDetails...),
		Confidence:       strings.TrimSpace(refresh.Confidence),
		Caveats:          append([]string(nil), refresh.Caveats...),
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

func scheduledUtilityView(results []utility.JobResult) *tui.UtilityView {
	if len(results) == 0 {
		return nil
	}
	var aggregate tui.UtilityView
	aggregate.Status = string(utility.StatusCompleted)
	aggregate.JobID = "idle-utility"
	aggregate.JobKind = "idle_schedule"
	aggregate.Summary = "idle utility hints ready"
	aggregate.ReadOnly = true
	seenEvidence := map[string]struct{}{}
	seenCaveat := map[string]struct{}{}
	for _, result := range results {
		request := utility.NormalizeJobRequest(result.Request)
		if aggregate.Source == "" {
			aggregate.Source = defaultString(request.Source.Caller, "app.utility")
		}
		if aggregate.Model == "" {
			aggregate.Model = request.Model
		}
		if aggregate.PreparedContext.Summary == "" {
			aggregate.PreparedContext = utilityPreparedContextView(result.PreparedContext)
		}
		if aggregate.StaleContext.Status == "" && aggregate.StaleContext.Summary == "" {
			aggregate.StaleContext = utilityStaleContextView(result.StaleContext)
		}
		if aggregate.SummaryRefresh.Status == "" && aggregate.SummaryRefresh.RefreshedSummary == "" {
			aggregate.SummaryRefresh = utilitySummaryRefreshView(result.SummaryRefresh)
		}
		aggregate.Suggestions = append(aggregate.Suggestions, utilitySuggestionViews(result.Suggestions)...)
		for _, ref := range utilityEvidenceRefViews(result.EvidenceRefs) {
			key := ref.ID + "\x00" + ref.Kind + "\x00" + ref.Source + "\x00" + ref.Detail
			if _, ok := seenEvidence[key]; ok {
				continue
			}
			seenEvidence[key] = struct{}{}
			aggregate.EvidenceRefs = append(aggregate.EvidenceRefs, ref)
		}
		for _, caveat := range result.Caveats {
			if _, ok := seenCaveat[caveat]; ok {
				continue
			}
			seenCaveat[caveat] = struct{}{}
			aggregate.Caveats = append(aggregate.Caveats, caveat)
		}
		aggregate.Safety.FileMutation = aggregate.Safety.FileMutation || result.Safety.FileMutation
		aggregate.Safety.GitMutation = aggregate.Safety.GitMutation || result.Safety.GitMutation
		aggregate.Safety.ProjectArtifactMutation = aggregate.Safety.ProjectArtifactMutation || result.Safety.ProjectArtifactMutation
		aggregate.Safety.ApprovalGrant = aggregate.Safety.ApprovalGrant || result.Safety.PermissionApproval
		aggregate.Safety.WorkflowPhaseTransition = aggregate.Safety.WorkflowPhaseTransition || result.Safety.WorkflowPhaseTransition
		aggregate.Safety.FinalJudgment = aggregate.Safety.FinalJudgment || result.Safety.FinalJudgment
		aggregate.Safety.ContextRefresh = aggregate.Safety.ContextRefresh || result.Safety.ContextRefresh
		aggregate.Safety.ContextCompaction = aggregate.Safety.ContextCompaction || result.Safety.ContextCompaction
		aggregate.Safety.ContextRewrite = aggregate.Safety.ContextRewrite || result.Safety.ContextRewrite
	}
	return &aggregate
}
