package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

const maxReviewChangedFiles = 8

func (controller *sessionController) openStatusView() {
	controller.view = tui.ApplyCommandSurface(controller.view, policy.CommandRouteStatus, "status", statusInspectionLines(controller.view, controller.runner.model))
}

func statusInspectionLines(view tui.ViewState, model runtime.Model) []string {
	status := view.RuntimeStatus
	if status == "" && model.Status != "" {
		status = string(model.Status)
	}
	if status == "" {
		status = "unknown"
	}

	lines := []string{
		"source: app.status",
		"read-only: true",
		"stage: " + valueOr(view.Phase, "unknown"),
		"phase source: " + valueOr(view.PhaseSource, "unknown"),
		"runtime status: " + status,
		"runtime active: " + boolText(view.RuntimeActive),
		"primary model: " + valueOr(view.PrimaryModel, "unknown"),
		"utility model: " + valueOr(view.UtilityModel, "unknown"),
		"autonomy: " + valueOr(view.Autonomy, "unknown"),
	}
	if view.StatusSource != "" {
		lines = append(lines, "runtime source: "+view.StatusSource)
	}
	if view.StatusDetail != "" {
		lines = append(lines, "runtime detail: "+view.StatusDetail)
	}
	if view.RuntimeResult != "" {
		lines = append(lines, "runtime result: "+view.RuntimeResult)
	} else if model.Result != "" {
		lines = append(lines, "runtime result: "+model.Result)
	}
	if model.LastCommand != "" {
		lines = append(lines, "last command: "+model.LastCommand)
	}
	if view.ProjectStoreStatus != "" {
		lines = append(lines, "project store: "+view.ProjectStoreStatus+" ("+valueOr(view.ProjectStoreSource, "unknown")+"; "+valueOr(view.ProjectStoreDetail, "no detail")+")")
	}
	if view.MemorySource != "" || view.MemorySessionID != "" {
		lines = append(lines, "memory source: "+valueOr(view.MemorySource, "unknown"), "memory session: "+valueOr(view.MemorySessionID, "unknown"))
	}
	if view.RunMemory != nil {
		lines = append(lines, "run memory: "+valueOr(view.RunMemory.Mode, "unknown")+" "+valueOr(view.RunMemory.Status, "unknown"))
		if view.RunMemory.StoredHistory {
			lines = append(lines, "run history: stored")
		}
	}
	if view.QueuedCount > 0 {
		lines = append(lines, fmt.Sprintf("queued messages: %d", view.QueuedCount))
	}
	lines = append(lines, subagentStatusLines(view.Subagents, model)...)
	lines = append(lines, utilityStatusLines(view.Utility, model)...)
	lines = append(lines, briefStatusLines(view.Brief)...)
	lines = append(lines, planStatusLines(view.Plan)...)
	lines = append(lines, fmt.Sprintf("diagnostics: %d", len(view.Diagnostics)))
	lines = append(lines, "git: "+valueOr(view.FooterGit, "unknown"), "context: "+valueOr(view.FooterContext, "unknown"))
	lines = append(lines, "inspection: app-owned display data")
	return lines
}

func (controller *sessionController) openReviewView() []diagnostic.Diagnostic {
	diff := emptyDiffView("app.review.diff")
	if controller.readDiff != nil {
		diff = controller.readDiff(controller.ctx, DiffReadCommand{}).View
	}

	historyState := "unavailable"
	var events []history.FakeEvent
	var diagnostics []diagnostic.Diagnostic
	if controller.readHistory != nil {
		result := controller.readHistory(controller.ctx, HistoryReadCommand{})
		historyState = string(result.State)
		events = result.Events
		diagnostics = append(diagnostics, result.Diagnostics...)
	}

	auditRequest := auditRequestFromReview(controller.view, diff, historyState, events)
	turn := controller.runner.proposeCapability(auditRequest)
	controller.view = tui.ApplyCommandSurface(controller.view, policy.CommandRouteReview, "review", reviewInspectionLines(controller.view, diff, historyState, events))
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return diagnostics
}

func reviewInspectionLines(view tui.ViewState, diff *tui.DiffView, historyState string, events []history.FakeEvent) []string {
	lines := []string{
		"source: app.review",
		"read-only: true",
		"model-assisted review: not invoked",
		"runtime status: " + valueOr(view.RuntimeStatus, "unknown"),
	}
	if diff == nil {
		lines = append(lines, "diff status: unavailable", "changed files: unknown")
	} else {
		lines = append(lines, "diff source: "+valueOr(diff.Source, "unknown"), "diff status: "+valueOr(diff.Status, "unknown"))
		lines = append(lines, fmt.Sprintf("changed files: %d", len(diff.Files)))
		if diff.Empty || len(diff.Files) == 0 {
			lines = append(lines, "changed file: none")
		}
		for i, file := range diff.Files {
			if i >= maxReviewChangedFiles {
				lines = append(lines, fmt.Sprintf("changed files omitted: %d", len(diff.Files)-maxReviewChangedFiles))
				break
			}
			lines = append(lines, "changed file: "+valueOr(file.Path, "unknown")+" status="+valueOr(file.Status, "unknown"))
		}
		if diff.ErrorMessage != "" {
			lines = append(lines, "diff error: "+diff.ErrorMessage)
		}
	}

	lines = append(lines, "history state: "+valueOr(historyState, "unknown"), fmt.Sprintf("history events: %d", len(events)))
	if last, ok := latestHistoryEvent(events); ok {
		lines = append(lines, "latest event: "+strings.Join(nonEmptyParts(string(last.Kind), last.Source, last.DisplayText), " "))
	}
	if lastMutation, ok := latestHistoryMutationEvent(events); ok {
		if lastMutation.Mutation != nil {
			lines = append(lines, "latest mutation: "+lastMutation.Mutation.ToolName+" "+lastMutation.Mutation.Status+" "+strings.Join(lastMutation.Mutation.ChangedPaths, ", "))
		}
		if lastMutation.Undo != nil {
			lines = append(lines, "latest undo action: "+lastMutation.Undo.Action)
		}
	}
	if len(view.Diagnostics) > 0 {
		lines = append(lines, fmt.Sprintf("attention: %d diagnostics visible", len(view.Diagnostics)))
	} else if diff != nil && len(diff.Files) > 0 {
		lines = append(lines, "attention: inspect changed files before committing")
	} else if len(events) > 0 {
		lines = append(lines, "attention: inspect recent history before committing")
	} else {
		lines = append(lines, "attention: no current changes or history found")
	}
	lines = append(lines, "inspection: app-owned display data")
	return lines
}

func latestHistoryEvent(events []history.FakeEvent) (history.FakeEvent, bool) {
	if len(events) == 0 {
		return history.FakeEvent{}, false
	}
	return events[len(events)-1], true
}

func latestHistoryMutationEvent(events []history.FakeEvent) (history.FakeEvent, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Mutation != nil || events[index].Undo != nil {
			return events[index], true
		}
	}
	return history.FakeEvent{}, false
}

func nonEmptyParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return parts
}

func valueOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func subagentStatusLines(views []tui.SubagentView, model runtime.Model) []string {
	if len(views) == 0 {
		views = subagentViews(model)
	}
	if len(views) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("subagents: %d", len(views))}
	for _, view := range views {
		line := "subagent: " + valueOr(view.ID, "unknown") + " parent=" + valueOr(view.ParentRunID, "unknown") + " status=" + valueOr(view.Status, "unknown") + " purpose=" + valueOr(view.Purpose, "unknown")
		lines = append(lines, line)
		if view.Summary != "" {
			lines = append(lines, "subagent summary: "+view.ID+" "+view.Summary)
		}
		for _, evidence := range view.EvidenceLinks {
			evidenceLine := "subagent evidence: " + view.ID + " " + valueOr(evidence.ID, "unknown") + " kind=" + valueOr(evidence.Kind, "unknown")
			if evidence.Path != "" {
				evidenceLine += " path=" + evidence.Path
			}
			if evidence.Command != "" {
				evidenceLine += " command=" + evidence.Command
			}
			lines = append(lines, evidenceLine)
		}
	}
	return lines
}

func utilityStatusLines(view *tui.UtilityView, model runtime.Model) []string {
	if view == nil {
		view = utilityView(model)
	}
	if view == nil {
		return nil
	}
	lines := []string{
		"utility worker: " + valueOr(view.Status, "idle"),
		"utility source: " + valueOr(view.Source, "unknown"),
		"utility job: " + strings.Join(nonEmptyParts(view.JobKind, view.JobID), " "),
		"utility job model: " + valueOr(view.Model, "unknown"),
		"utility read-only: " + boolText(view.ReadOnly),
	}
	if view.Summary != "" {
		lines = append(lines, "utility summary: "+view.Summary)
	}
	if view.PreparedContext.Summary != "" {
		line := "utility prepared context: " + view.PreparedContext.Summary
		if len(view.PreparedContext.EvidenceRefIDs) > 0 {
			line += " refs=" + strings.Join(view.PreparedContext.EvidenceRefIDs, ",")
		}
		lines = append(lines, line, "utility prepared context non-authoritative: "+boolText(view.PreparedContext.NonAuthoritative))
		for _, caveat := range view.PreparedContext.Caveats {
			lines = append(lines, "utility prepared context caveat: "+caveat)
		}
	}
	if view.StaleContext.Status != "" || view.StaleContext.Summary != "" {
		if view.StaleContext.Status != "" {
			lines = append(lines, "utility stale context: "+view.StaleContext.Status)
		}
		if view.StaleContext.Summary != "" {
			line := "utility stale context summary: " + view.StaleContext.Summary
			if len(view.StaleContext.EvidenceRefIDs) > 0 {
				line += " refs=" + strings.Join(view.StaleContext.EvidenceRefIDs, ",")
			}
			lines = append(lines, line)
		}
		for _, caveat := range view.StaleContext.Caveats {
			lines = append(lines, "utility stale context caveat: "+caveat)
		}
		if view.StaleContext.SuggestedNextAction != "" {
			lines = append(lines, "utility suggested next action: "+view.StaleContext.SuggestedNextAction)
		}
	}
	if view.SummaryRefresh.Status != "" || view.SummaryRefresh.RefreshedSummary != "" {
		if view.SummaryRefresh.Status != "" {
			lines = append(lines, "utility summary refresh: "+view.SummaryRefresh.Status)
		}
		if view.SummaryRefresh.OriginalSummary != "" {
			lines = append(lines, "utility original summary: "+view.SummaryRefresh.OriginalSummary)
		}
		if view.SummaryRefresh.RefreshedSummary != "" {
			line := "utility refreshed summary: " + view.SummaryRefresh.RefreshedSummary
			if len(view.SummaryRefresh.SourceRefIDs) > 0 {
				line += " refs=" + strings.Join(view.SummaryRefresh.SourceRefIDs, ",")
			}
			lines = append(lines, line)
		}
		if len(view.SummaryRefresh.SourceRefIDs) > 0 {
			lines = append(lines, "utility summary refresh source refs: "+strings.Join(view.SummaryRefresh.SourceRefIDs, ","))
		}
		if view.SummaryRefresh.Confidence != "" {
			lines = append(lines, "utility summary refresh confidence: "+view.SummaryRefresh.Confidence)
		}
		for _, detail := range view.SummaryRefresh.ExactDetails {
			lines = append(lines, "utility summary refresh detail: "+detail)
		}
		for _, caveat := range view.SummaryRefresh.Caveats {
			lines = append(lines, "utility summary refresh caveat: "+caveat)
		}
	}
	for _, suggestion := range view.Suggestions {
		line := "utility suggestion: " + suggestion.Text
		if len(suggestion.EvidenceRefIDs) > 0 {
			line += " refs=" + strings.Join(suggestion.EvidenceRefIDs, ",")
		}
		lines = append(lines, line)
	}
	for _, ref := range view.EvidenceRefs {
		lines = append(lines, "utility evidence: "+strings.Join(nonEmptyParts(ref.ID, ref.Kind, ref.Source, ref.Detail), " "))
	}
	for _, caveat := range view.Caveats {
		lines = append(lines, "utility caveat: "+caveat)
	}
	if view.DeniedReason != "" {
		lines = append(lines, "utility denied: "+strings.Join(nonEmptyParts(view.DeniedReason, view.DeniedDetail), " "))
	}
	lines = append(lines,
		"utility file mutation: "+boolText(view.Safety.FileMutation),
		"utility git mutation: "+boolText(view.Safety.GitMutation),
		"utility artifact mutation: "+boolText(view.Safety.ProjectArtifactMutation),
		"utility permission approval: "+boolText(view.Safety.ApprovalGrant),
		"utility workflow transition: "+boolText(view.Safety.WorkflowPhaseTransition),
		"utility final judgment: "+boolText(view.Safety.FinalJudgment),
		"utility context refresh: "+boolText(view.Safety.ContextRefresh),
		"utility context compaction: "+boolText(view.Safety.ContextCompaction),
		"utility context rewrite: "+boolText(view.Safety.ContextRewrite),
	)
	return lines
}
