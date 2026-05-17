package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

func (controller *sessionController) briefRequestFromStatus() capability.Request {
	metadata := map[string]string{
		capability.BriefMetadataRuntimeStatus:      statusRuntimeLabel(controller.view, controller.runner.model),
		capability.BriefMetadataProjectStoreStatus: valueOr(controller.view.ProjectStoreStatus, "unknown"),
		capability.BriefMetadataContextStatus:      contextStatusLabel(controller.view),
		capability.BriefMetadataContextSummary:     valueOr(controller.view.FooterContext, "unknown"),
		capability.BriefMetadataHealthStatus:       healthStatusLabel(controller.view),
	}
	refs := []capability.SourceRef{
		{ID: "brief-runtime", Kind: "runtime_state", Excerpt: "phase=" + controller.view.PhaseSource + " status=" + metadata[capability.BriefMetadataRuntimeStatus]},
		{ID: "brief-store", Kind: "project_store", Excerpt: valueOr(controller.view.ProjectStoreStatus, "unknown") + " " + valueOr(controller.view.ProjectStoreDetail, "no detail")},
		{ID: "brief-context", Kind: "context", Excerpt: metadata[capability.BriefMetadataContextSummary]},
		{ID: "brief-health", Kind: "health", Excerpt: fmt.Sprintf("visible diagnostics=%d", len(controller.view.Diagnostics))},
	}
	if controller.readHistory == nil {
		metadata[capability.BriefMetadataHistoryState] = "unavailable"
	} else {
		result := controller.readHistory(controller.ctx, HistoryReadCommand{})
		metadata[capability.BriefMetadataHistoryState] = string(result.State)
		metadata[capability.BriefMetadataHistoryEvents] = fmt.Sprint(len(result.Events))
		if latest, ok := latestHistoryEvent(result.Events); ok {
			metadata[capability.BriefMetadataLatestHistory] = latest.DisplayText
			refs = append(refs, capability.SourceRef{ID: "brief-history", Kind: "history", Excerpt: latestHistoryExcerpt(latest)})
		} else {
			refs = append(refs, capability.SourceRef{ID: "brief-history", Kind: "history", Excerpt: string(result.State) + " events=0"})
		}
		if result.State == state.FakeHistoryRecoveryNeeded {
			metadata[capability.BriefMetadataSuggestedNextAction] = "Inspect history recovery before relying on prior activity."
		}
	}
	if metadata[capability.BriefMetadataSuggestedNextAction] == "" {
		metadata[capability.BriefMetadataSuggestedNextAction] = statusSuggestedNextAction(controller.view, controller.runner.model)
	}
	return capability.Request{
		ID:         "status-brief",
		Capability: capability.NameBrief,
		Input:      "status orientation",
		Phase:      workflowPhaseFromView(controller.view),
		SourceRefs: refs,
		Metadata:   metadata,
	}
}

func (runner *inputRunner) proposeCapability(request capability.Request) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.CapabilityProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	turn.Brief = briefView(runner.model.LastCapability, runner.model.Status)
	if turn.Brief != nil {
		turn.StatusDetail = "brief capability status"
	}
	return turn
}

func briefView(payload capability.ExitPayload, runtimeStatus runtime.Status) *tui.BriefView {
	if payload.Capability == "" {
		return nil
	}
	return &tui.BriefView{
		Source:              "app.brief",
		Capability:          string(payload.Capability),
		Signal:              string(payload.Signal),
		Summary:             payload.Summary,
		CurrentPhase:        phaseFromBriefSourceRefs(payload.SourceRefs),
		RuntimeStatus:       string(runtimeStatus),
		KnownGaps:           append([]string(nil), payload.Concerns...),
		SuggestedNextAction: payload.NextAction,
		TransitionClaimed:   payload.RecommendedSuccessor != "",
		DisplayOnly:         true,
		SourceRefs:          briefSourceRefViews(payload.SourceRefs),
		BoundaryRequests:    briefBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func statusRuntimeLabel(view tui.ViewState, model runtime.Model) string {
	if model.Status != "" {
		return string(model.Status)
	}
	return valueOr(view.RuntimeStatus, "unknown")
}

func contextStatusLabel(view tui.ViewState) string {
	if strings.TrimSpace(view.FooterContext) == "" || view.FooterContext == "placeholder" {
		return "unavailable"
	}
	return "available"
}

func healthStatusLabel(view tui.ViewState) string {
	if len(view.Diagnostics) > 0 {
		return "flagged"
	}
	return "available"
}

func statusSuggestedNextAction(view tui.ViewState, model runtime.Model) string {
	if len(model.Queued) > 0 || view.QueuedCount > 0 {
		return "Resolve queued input before starting another capability."
	}
	if len(view.Diagnostics) > 0 {
		return "Inspect visible diagnostics before continuing."
	}
	if statusRuntimeLabel(view, model) != string(runtime.StatusIdle) {
		return "Let runtime work settle before starting another capability."
	}
	return "Continue with the current roadmap task or choose the next capability."
}

func workflowPhaseFromView(view tui.ViewState) workflow.Phase {
	switch workflow.Phase(strings.ToLower(strings.TrimSpace(view.PhaseSource))) {
	case workflow.PhaseEnvision:
		return workflow.PhaseEnvision
	case workflow.PhaseDeliberate:
		return workflow.PhaseDeliberate
	case workflow.PhasePlan:
		return workflow.PhasePlan
	case workflow.PhaseBuild:
		return workflow.PhaseBuild
	case workflow.PhaseAudit:
		return workflow.PhaseAudit
	default:
		return workflow.PhaseIdle
	}
}

func latestHistoryExcerpt(event history.FakeEvent) string {
	parts := nonEmptyParts(string(event.Kind), event.Source, event.DisplayText)
	if len(parts) == 0 {
		return "history event"
	}
	return strings.Join(parts, " ")
}

func phaseFromBriefSourceRefs(refs []capability.SourceRef) string {
	for _, ref := range refs {
		if ref.ID != "brief-runtime" {
			continue
		}
		for _, field := range strings.Fields(ref.Excerpt) {
			if strings.HasPrefix(field, "phase=") {
				return strings.TrimPrefix(field, "phase=")
			}
		}
	}
	return workflow.PhaseIdle.String()
}

func briefSourceRefViews(refs []capability.SourceRef) []tui.BriefSourceRefView {
	views := make([]tui.BriefSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.BriefSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func briefBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.BriefBoundaryRequestView {
	views := make([]tui.BriefBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.BriefBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}

func briefStatusLines(brief *tui.BriefView) []string {
	if brief == nil {
		return nil
	}
	lines := []string{
		"brief capability: " + valueOr(brief.Capability, "brief") + " " + valueOr(brief.Signal, "complete"),
		"brief source: " + valueOr(brief.Source, "app.brief"),
		"brief current phase: " + valueOr(brief.CurrentPhase, "unknown"),
		"brief runtime status: " + valueOr(brief.RuntimeStatus, "unknown"),
		"brief transition claimed: " + boolText(brief.TransitionClaimed),
		"brief display-only: " + boolText(brief.DisplayOnly),
	}
	if brief.Summary != "" {
		lines = append(lines, "brief summary: "+brief.Summary)
	}
	for _, gap := range brief.KnownGaps {
		lines = append(lines, "brief known gap: "+gap)
	}
	if brief.SuggestedNextAction != "" {
		lines = append(lines, "brief suggested next action: "+brief.SuggestedNextAction)
	}
	for _, request := range brief.BoundaryRequests {
		line := "brief requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		lines = append(lines, line)
	}
	for _, ref := range brief.SourceRefs {
		line := "brief source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	return lines
}
